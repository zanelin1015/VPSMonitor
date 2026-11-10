package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFrontendCachePolicy(t *testing.T) {
	files := fstest.MapFS{
		"index.html":                {Data: []byte("<html><head></head><body>portal</body></html>")},
		"assets/index-Abcd123_.js":  {Data: []byte("window.portal = true;")},
		"assets/index-Abcd123_.css": {Data: []byte("body { color: black; }")},
		"assets/settings.js":        {Data: []byte("window.settings = {};")},
	}
	handler := newHandler(files, func(*http.Request) string { return "/panel" })
	for _, tc := range []struct {
		path      string
		status    int
		immutable bool
		noStore   bool
	}{
		{"/assets/index-Abcd123_.js", 200, true, false},
		{"/assets/index-Abcd123_.css", 200, true, false},
		{"/assets/settings.js", 200, false, false},
		{"/assets/missing-Abcd123_.js", 404, false, true},
		{"/assets", 404, false, true},
		{"/customer", 200, false, true},
		{"/", 200, false, true},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+tc.path, func(t *testing.T) {
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, httptest.NewRequest(method, tc.path, nil))
				if rr.Code != tc.status {
					t.Fatalf("status=%d, want %d", rr.Code, tc.status)
				}
				cache := rr.Header().Get("Cache-Control")
				if strings.Contains(cache, "immutable") != tc.immutable || (cache == "no-store") != tc.noStore {
					t.Fatalf("unexpected Cache-Control %q", cache)
				}
				if method == http.MethodGet && (tc.path == "/customer" || tc.path == "/") && !strings.Contains(rr.Body.String(), `<base href="/panel/">`) {
					t.Fatal("entry HTML must retain the deployment base path")
				}
				if tc.status == http.StatusNotFound && strings.Contains(rr.Body.String(), "<html>") {
					t.Fatal("missing asset returned the SPA entry document")
				}
			})
		}
	}
}
