package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const (
	exchangeRatesCacheTTL = 12 * time.Hour
)

var exchangeRatesSources = []exchangeRatesSource{
	{
		name: "Frankfurter / ECB latest reference rates",
		url:  "https://api.frankfurter.dev/v1/latest?base=EUR",
	},
}

type exchangeRatesSource struct {
	name string
	url  string
}

func (a *App) handleAdminExchangeRates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, _, ok := a.requireRootAdmin(w, r); !ok {
		return
	}

	rates, err := a.loadExchangeRates()
	if err != nil {
		if cached, ok := a.cachedExchangeRates(); ok {
			cached.Stale = true
			cached.Error = err.Error()
			writeJSON(w, http.StatusOK, cached)
			return
		}
		fallback := fallbackExchangeRates(err)
		writeJSON(w, http.StatusOK, fallback)
		return
	}
	writeJSON(w, http.StatusOK, rates)
}

func (a *App) loadExchangeRates() (model.ExchangeRatesResponse, error) {
	if cached, ok := a.cachedExchangeRates(); ok && time.Since(cached.FetchedAt) < exchangeRatesCacheTTL {
		return cached, nil
	}

	client := &http.Client{Timeout: 12 * time.Second}
	var lastErr error
	for _, source := range exchangeRatesSources {
		result, err := fetchExchangeRates(client, source)
		if err != nil {
			lastErr = err
			continue
		}
		a.exchangeRatesMu.Lock()
		a.exchangeRatesCache = result
		a.exchangeRatesMu.Unlock()
		if a.store != nil {
			_ = a.store.SaveExchangeRatesCache(result)
		}
		return result, nil
	}
	if lastErr != nil {
		return model.ExchangeRatesResponse{}, lastErr
	}
	return model.ExchangeRatesResponse{}, fmt.Errorf("没有可用汇率源")
}

func fetchExchangeRates(client *http.Client, source exchangeRatesSource) (model.ExchangeRatesResponse, error) {
	request, err := http.NewRequest(http.MethodGet, source.url, nil)
	if err != nil {
		return model.ExchangeRatesResponse{}, fmt.Errorf("创建汇率请求失败: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "VPSMonitor/1.0")

	response, err := client.Do(request)
	if err != nil {
		return model.ExchangeRatesResponse{}, fmt.Errorf("请求汇率接口失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return model.ExchangeRatesResponse{}, fmt.Errorf("汇率接口返回 %s", response.Status)
	}

	var payload struct {
		Base  string             `json:"base"`
		Date  string             `json:"date"`
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return model.ExchangeRatesResponse{}, fmt.Errorf("解析汇率响应失败: %w", err)
	}

	rates := map[string]float64{"EUR": 1}
	for currency, rate := range payload.Rates {
		currency = strings.ToUpper(strings.TrimSpace(currency))
		if len(currency) == 3 && rate > 0 {
			rates[currency] = rate
		}
	}
	result := model.ExchangeRatesResponse{
		Base:      firstNonEmptyString(strings.ToUpper(strings.TrimSpace(payload.Base)), "EUR"),
		Date:      strings.TrimSpace(payload.Date),
		Rates:     rates,
		Source:    source.name,
		FetchedAt: time.Now().UTC(),
	}
	if result.Date == "" || len(result.Rates) <= 1 {
		return model.ExchangeRatesResponse{}, fmt.Errorf("汇率响应缺少有效数据")
	}
	return result, nil
}

func (a *App) cachedExchangeRates() (model.ExchangeRatesResponse, bool) {
	a.exchangeRatesMu.Lock()
	cached := a.exchangeRatesCache
	a.exchangeRatesMu.Unlock()
	if !cached.FetchedAt.IsZero() && len(cached.Rates) > 0 {
		return cached, true
	}
	if a.store == nil {
		return model.ExchangeRatesResponse{}, false
	}
	stored, ok, err := a.store.GetExchangeRatesCache()
	if err != nil || !ok {
		return model.ExchangeRatesResponse{}, false
	}
	a.exchangeRatesMu.Lock()
	a.exchangeRatesCache = stored
	a.exchangeRatesMu.Unlock()
	return stored, true
}

func fallbackExchangeRates(err error) model.ExchangeRatesResponse {
	return model.ExchangeRatesResponse{
		Base: "EUR",
		Date: time.Now().UTC().Format("2006-01-02"),
		Rates: map[string]float64{
			"EUR": 1,
			"USD": 1.08,
			"CNY": 7.85,
			"HKD": 8.45,
			"JPY": 169,
			"GBP": 0.86,
			"SGD": 1.45,
			"AUD": 1.66,
			"CAD": 1.49,
		},
		Source:    "Fallback approximate exchange rates",
		FetchedAt: time.Now().UTC(),
		Stale:     true,
		Error:     compactExchangeRateError(err),
	}
}

func compactExchangeRateError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if strings.Contains(message, "Client.Timeout") || strings.Contains(message, "context deadline exceeded") {
		return "汇率接口超时"
	}
	if strings.Contains(message, "no such host") || strings.Contains(message, "lookup ") {
		return "汇率接口 DNS 解析失败"
	}
	return message
}
