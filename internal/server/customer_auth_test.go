package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func customerAuthFixture(t *testing.T) (*App, model.CustomerAdminView) {
	t.Helper()
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	customer, err := s.CreateCustomer(model.CustomerAccountRequest{Username: "alice", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	return &App{store: s, loginLimiter: newLoginRateLimiter()}, customer
}

func customerAuthRequest(app *App, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://monitor.example"+path, strings.NewReader(body))
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	return w
}

func authCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

func TestCustomerLoginRefreshAndLogoutHTTP(t *testing.T) {
	app, customer := customerAuthFixture(t)
	login := customerAuthRequest(app, http.MethodPost, "/api/v1/customer/login", `{"username":"alice","password":"password123"}`)
	if login.Code != http.StatusOK || login.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("login status=%d", login.Code)
	}
	access := authCookie(t, login, customerSessionCookieName)
	refresh := authCookie(t, login, customerRefreshCookieName)
	for _, cookie := range []*http.Cookie{access, refresh} {
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatal("credentials must use secure HttpOnly same-site cookies")
		}
		if strings.Contains(login.Body.String(), cookie.Value) {
			t.Fatal("credentials must not be exposed in JSON")
		}
	}
	if access.Value == refresh.Value || access.Path != "/" || refresh.Path != "/api/v1/customer/" || access.MaxAge < int(customerSessionTTL.Seconds())-2 || access.MaxAge > int(customerSessionTTL.Seconds()) || refresh.MaxAge < int(customerRefreshTTL.Seconds())-2 {
		t.Fatal("unexpected credential scope or lifetime")
	}
	if w := customerAuthRequest(app, http.MethodGet, "/api/v1/customer/session", "", access); w.Code != http.StatusOK {
		t.Fatalf("access session status=%d", w.Code)
	}
	if w := customerAuthRequest(app, http.MethodGet, "/api/v1/customer/session", "", refresh); w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh alone must not access protected routes: status=%d", w.Code)
	}
	if w := customerAuthRequest(app, http.MethodPost, "/api/v1/customer/refresh", "", access); w.Code != http.StatusUnauthorized {
		t.Fatalf("access alone must not refresh: status=%d", w.Code)
	}
	if w := customerAuthRequest(app, http.MethodPost, "/api/v1/customer/login", `{"username":"alice","password":"wrong-password"}`); w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Fatalf("invalid login status=%d", w.Code)
	}

	// Expire only access, as if the browser returned after two hours.
	short, err := app.store.CreateCustomerSessionPair(customer.ID, time.Nanosecond, customerRefreshTTL)
	if err != nil {
		t.Fatal(err)
	}
	expiredAccess := &http.Cookie{Name: customerSessionCookieName, Value: short.AccessToken}
	validRefresh := &http.Cookie{Name: customerRefreshCookieName, Value: short.RefreshToken}
	if w := customerAuthRequest(app, http.MethodGet, "/api/v1/customer/session", "", expiredAccess); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired access status=%d", w.Code)
	}
	renewal := customerAuthRequest(app, http.MethodPost, "/api/v1/customer/refresh", "", validRefresh)
	if renewal.Code != http.StatusOK || renewal.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("refresh status=%d body=%s", renewal.Code, renewal.Body.String())
	}
	renewedAccess := authCookie(t, renewal, customerSessionCookieName)
	if renewedAccess.Value == short.AccessToken || len(renewal.Result().Cookies()) != 1 {
		t.Fatal("refresh must issue a new access cookie without extending refresh cookie")
	}
	if w := customerAuthRequest(app, http.MethodGet, "/api/v1/customer/session", "", renewedAccess); w.Code != http.StatusOK {
		t.Fatalf("renewed access status=%d", w.Code)
	}
	logout := customerAuthRequest(app, http.MethodPost, "/api/v1/customer/logout", "", validRefresh)
	if logout.Code != http.StatusOK || authCookie(t, logout, customerSessionCookieName).MaxAge != -1 || authCookie(t, logout, customerRefreshCookieName).MaxAge != -1 {
		t.Fatal("logout must clear both cookies even without access")
	}
	if w := customerAuthRequest(app, http.MethodGet, "/api/v1/customer/session", "", renewedAccess); w.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out access status=%d", w.Code)
	}
	if w := customerAuthRequest(app, http.MethodPost, "/api/v1/customer/refresh", "", validRefresh); w.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out refresh status=%d", w.Code)
	}
	// Logging out one device leaves another device's session untouched.
	if w := customerAuthRequest(app, http.MethodPost, "/api/v1/customer/refresh", "", refresh); w.Code != http.StatusOK {
		t.Fatalf("other device refresh status=%d", w.Code)
	}
}

func TestCustomerRefreshInvalidExpiryOriginAndMethods(t *testing.T) {
	app, customer := customerAuthFixture(t)
	expired, err := app.store.CreateCustomerSessionPair(customer.ID, time.Nanosecond, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "unknown-refresh", expired.RefreshToken} {
		w := customerAuthRequest(app, http.MethodPost, "/api/v1/customer/refresh", "", &http.Cookie{Name: customerRefreshCookieName, Value: token})
		if w.Code != http.StatusUnauthorized || authCookie(t, w, customerRefreshCookieName).MaxAge != -1 {
			t.Fatalf("invalid refresh status=%d", w.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		if w := customerAuthRequest(app, method, "/api/v1/customer/refresh", ""); w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("refresh method %s status=%d", method, w.Code)
		}
	}
	valid, err := app.store.CreateCustomerSessionPair(customer.ID, customerSessionTTL, customerRefreshTTL)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/customer/refresh", nil)
	r.Header.Set("Origin", "https://other.example")
	r.AddCookie(&http.Cookie{Name: customerRefreshCookieName, Value: valid.RefreshToken})
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
		t.Fatal("cross-origin renewal must be rejected without altering cookies")
	}
	r.Header.Set("Origin", "https://monitor.example")
	w = httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("same-origin refresh status=%d", w.Code)
	}
}

func TestCustomerCredentialCookiePublicPrefix(t *testing.T) {
	app, _ := customerAuthFixture(t)
	app.config = config.ServerConfig{PublicPathPrefixes: map[string]string{"monitor.example": "/zanelin"}}
	app.trustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	for _, action := range []string{"login", "logout"} {
		r := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/customer/"+action, strings.NewReader(`{"username":"alice","password":"password123"}`))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Forwarded-Prefix", "/zanelin")
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d", action, w.Code)
		}
		if authCookie(t, w, customerSessionCookieName).Path != "/zanelin/" || authCookie(t, w, customerRefreshCookieName).Path != "/zanelin/api/v1/customer/" {
			t.Fatalf("%s did not preserve cookie prefix", action)
		}
	}
}
