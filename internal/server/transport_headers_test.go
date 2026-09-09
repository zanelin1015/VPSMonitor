package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecureTransportHeadersAddsHSTSOnlyForSecureRequests(t *testing.T) {
	handler := secureTransportHeaders(&App{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	secureRequest := httptest.NewRequest(http.MethodGet, "https://panel.example/", nil)
	secureRequest.TLS = &tls.ConnectionState{}
	secureResponse := httptest.NewRecorder()
	handler.ServeHTTP(secureResponse, secureRequest)
	if got := secureResponse.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("expected HSTS on HTTPS response, got %q", got)
	}

	insecureRequest := httptest.NewRequest(http.MethodGet, "http://panel.example/", nil)
	insecureResponse := httptest.NewRecorder()
	handler.ServeHTTP(insecureResponse, insecureRequest)
	if got := insecureResponse.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS must not be emitted over HTTP, got %q", got)
	}
}
