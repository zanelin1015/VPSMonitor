package server

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestLoginRateLimiterBlocksRepeatedFailures(t *testing.T) {
	limiter := newLoginRateLimiter()
	for i := 0; i < loginRateLimitMax; i++ {
		if !limiter.allowed("admin:ip:203.0.113.10", "admin:account:root") {
			t.Fatalf("attempt %d was blocked before the limit", i+1)
		}
		limiter.failure("admin:ip:203.0.113.10", "admin:account:root")
	}
	if limiter.allowed("admin:ip:203.0.113.10", "admin:account:root") {
		t.Fatal("expected repeated failed logins to be blocked")
	}
	limiter.success("admin:account:root")
	if limiter.allowed("admin:ip:203.0.113.10", "admin:account:root") {
		t.Fatal("successful login must not clear the per-IP protection")
	}
}

func TestSecureRequestTrustsForwardedProtoOnlyFromConfiguredProxy(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.test/", nil)
	req.RemoteAddr = "198.51.100.20:443"
	req.Header.Set("X-Forwarded-Proto", "https")
	if (&App{}).isSecureRequest(req) {
		t.Fatal("untrusted forwarded proto must not mark a request secure")
	}
	app := &App{trustedProxies: []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}}
	if !app.isSecureRequest(req) {
		t.Fatal("configured proxy forwarded proto should mark a request secure")
	}
}
