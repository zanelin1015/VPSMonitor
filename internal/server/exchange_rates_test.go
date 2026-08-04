package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestFetchExchangeRatesParsesFrankfurterResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("base") != "EUR" {
			t.Fatalf("expected base=EUR query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base":"EUR","date":"2026-07-30","rates":{"USD":1.08,"CNY":7.85,"BAD":0}}`))
	}))
	defer upstream.Close()

	rates, err := fetchExchangeRates(upstream.Client(), exchangeRatesSource{
		name: "test-source",
		url:  upstream.URL + "?base=EUR",
	})
	if err != nil {
		t.Fatalf("fetchExchangeRates: %v", err)
	}
	if rates.Base != "EUR" || rates.Date != "2026-07-30" || rates.Source != "test-source" {
		t.Fatalf("unexpected response metadata: %#v", rates)
	}
	if rates.Rates["EUR"] != 1 || rates.Rates["USD"] != 1.08 || rates.Rates["CNY"] != 7.85 {
		t.Fatalf("unexpected parsed rates: %#v", rates.Rates)
	}
	if _, ok := rates.Rates["BAD"]; ok {
		t.Fatalf("expected invalid zero rate to be dropped: %#v", rates.Rates)
	}
}

func TestCachedExchangeRatesLoadsPersistedCache(t *testing.T) {
	sqliteStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sqliteStore.Close()

	fetchedAt := time.Now().UTC().Add(-time.Hour)
	if err := sqliteStore.SaveExchangeRatesCache(model.ExchangeRatesResponse{
		Base:      "EUR",
		Date:      "2026-07-30",
		Rates:     map[string]float64{"EUR": 1, "CNY": 7.85},
		Source:    "stored",
		FetchedAt: fetchedAt,
	}); err != nil {
		t.Fatalf("SaveExchangeRatesCache: %v", err)
	}

	app := &App{store: sqliteStore}
	cached, ok := app.cachedExchangeRates()
	if !ok {
		t.Fatal("expected cached rates")
	}
	if cached.Source != "stored" || cached.Rates["CNY"] != 7.85 {
		t.Fatalf("unexpected cached rates: %#v", cached)
	}
}

func TestFallbackExchangeRatesCompactsTimeoutError(t *testing.T) {
	rates := fallbackExchangeRates(assertTimeoutError{})
	if !rates.Stale || rates.Rates["CNY"] == 0 || rates.Rates["USD"] == 0 {
		t.Fatalf("unexpected fallback rates: %#v", rates)
	}
	if !strings.Contains(rates.Error, "超时") {
		t.Fatalf("expected compact timeout error, got %q", rates.Error)
	}
}

type assertTimeoutError struct{}

func (assertTimeoutError) Error() string {
	return `请求汇率接口失败: Get "https://api.frankfurter.dev/v1/latest?base=EUR": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
}
