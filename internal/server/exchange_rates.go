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
	exchangeRatesSourceURL = "https://api.frankfurter.dev/v1/latest?from=EUR"
	exchangeRatesCacheTTL  = 12 * time.Hour
)

func (a *App) handleAdminExchangeRates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, _, ok := a.requireAdmin(w, r); !ok {
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
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rates)
}

func (a *App) loadExchangeRates() (model.ExchangeRatesResponse, error) {
	if cached, ok := a.cachedExchangeRates(); ok && time.Since(cached.FetchedAt) < exchangeRatesCacheTTL {
		return cached, nil
	}

	client := &http.Client{Timeout: 12 * time.Second}
	request, err := http.NewRequest(http.MethodGet, exchangeRatesSourceURL, nil)
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
		Source:    "Frankfurter / ECB latest reference rates",
		FetchedAt: time.Now().UTC(),
	}
	if result.Date == "" || len(result.Rates) <= 1 {
		return model.ExchangeRatesResponse{}, fmt.Errorf("汇率响应缺少有效数据")
	}

	a.exchangeRatesMu.Lock()
	a.exchangeRatesCache = result
	a.exchangeRatesMu.Unlock()
	return result, nil
}

func (a *App) cachedExchangeRates() (model.ExchangeRatesResponse, bool) {
	a.exchangeRatesMu.Lock()
	defer a.exchangeRatesMu.Unlock()
	if a.exchangeRatesCache.FetchedAt.IsZero() || len(a.exchangeRatesCache.Rates) == 0 {
		return model.ExchangeRatesResponse{}, false
	}
	return a.exchangeRatesCache, true
}
