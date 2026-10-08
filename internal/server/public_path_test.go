package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/config"
)

func TestPublicPathTrustAndHostScope(t *testing.T) {
	app := &App{
		config:         config.ServerConfig{PublicPathPrefixes: map[string]string{"monitor.appleaccount.top": "/zanelin"}},
		trustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
	}
	for _, tc := range []struct{ name, host, remote, header, want string }{
		{"new trusted host", "monitor.appleaccount.top", "127.0.0.1:1234", "/zanelin", "/zanelin"},
		{"old host unchanged", "monitor.zanelin.top", "127.0.0.1:1234", "/zanelin", ""},
		{"untrusted caller", "monitor.appleaccount.top", "192.0.2.1:1234", "/zanelin", ""},
		{"no header", "monitor.appleaccount.top", "127.0.0.1:1234", "", ""},
		{"unsafe header", "monitor.appleaccount.top", "127.0.0.1:1234", "/zanelin/\"><script>", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/customer", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-Prefix", tc.header)
			handler := app.withPublicPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := requestPublicPrefix(r); got != tc.want {
					t.Fatalf("prefix=%q want=%q", got, tc.want)
				}
				if got := requestPublicBaseURL(r); got != "https://"+tc.host+tc.want {
					t.Fatalf("base URL=%q", got)
				}
				if got := customerSubscriptionURL(r, "test-token", "clash.yaml"); !strings.HasPrefix(got, "https://"+tc.host+tc.want+"/api/v1/customer/subscription/") {
					t.Fatalf("subscription=%q", got)
				}
				app.setAdminSessionCookie(w, r, "test-token", time.Now().Add(time.Hour))
				app.setCustomerSessionCookie(w, r, "test-token", time.Now().Add(time.Hour))
				app.clearAdminSessionCookie(w, r)
				app.clearCustomerSessionCookie(w, r)
			}))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			for _, cookie := range rr.Result().Cookies() {
				if cookie.Path != tc.want+"/" || !cookie.Secure || !cookie.HttpOnly {
					t.Fatalf("invalid cookie scope/security: %#v", cookie)
				}
			}
		})
	}
}

func TestPublicPathHTMLBases(t *testing.T) {
	app := &App{config: config.ServerConfig{PublicPathPrefixes: map[string]string{"monitor.appleaccount.top": "/zanelin"}}, trustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}
	for _, tc := range []struct{ host, prefix, want string }{
		{"monitor.appleaccount.top", "/zanelin", "/zanelin/"},
		{"monitor.zanelin.top", "/zanelin", "/"},
	} {
		req := httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/customer", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-Forwarded-Prefix", tc.prefix)
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `<base href="`+tc.want+`">`) {
			t.Fatalf("HTML base missing: status=%d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("entry HTML must not be cached")
		}
	}
}
