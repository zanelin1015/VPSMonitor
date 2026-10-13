package panels

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestXUILoginWithCSRFProtectsSessionWrites(t *testing.T) {
	for _, meta := range []string{
		`<meta name="csrf-token" content="test-csrf-token">`,
		`<META content='test-csrf-token' name='csrf-token'>`,
	} {
		t.Run(meta, func(t *testing.T) {
			logins, pages, writes := 0, 0, 0
			client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("Authorization") != "" {
					t.Fatal("session authentication must not send a bearer token")
				}
				switch req.URL.Path {
				case "/login":
					logins++
					if logins == 1 {
						response := jsonResponse(t, req, map[string]any{"success": false})
						response.StatusCode = http.StatusForbidden
						return response, nil
					}
					if req.Header.Get("X-CSRF-Token") != "test-csrf-token" {
						t.Fatal("login retry did not send the page's CSRF token")
					}
					if cookie, err := req.Cookie("csrf-session"); err != nil || cookie.Value != "page-cookie" {
						t.Fatal("login retry did not preserve the page's cookie")
					}
					if err := req.ParseForm(); err != nil || req.Form.Get("username") != "test-user" || req.Form.Get("password") != "test-password" {
						t.Fatal("login retry lost credentials")
					}
					return jsonResponse(t, req, map[string]any{"success": true}), nil
				case "/":
					pages++
					return &http.Response{
						StatusCode: http.StatusOK, Request: req,
						Header: http.Header{"Content-Type": {"text/html"}, "Set-Cookie": {"csrf-session=page-cookie; Path=/"}},
						Body:   io.NopCloser(strings.NewReader(meta)),
					}, nil
				case "/panel/api/clients/update/alice":
					writes++
					if req.Header.Get("X-CSRF-Token") != "test-csrf-token" {
						t.Fatal("authenticated API write lost CSRF protection")
					}
					return jsonResponse(t, req, map[string]any{"success": true}), nil
				default:
					t.Fatalf("unexpected request: %s", req.URL.Path)
					return nil, nil
				}
			})
			client.config.APIToken = ""
			client.config.Username = "test-user"
			client.config.Password = "test-password"
			client.csrfToken = "stale-token"
			if err := client.login(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := client.postJSON(context.Background(), "/panel/api/clients/update/alice", map[string]any{"email": "alice"}); err != nil {
				t.Fatal(err)
			}
			if logins != 2 || pages != 1 || writes != 1 || !client.authenticated {
				t.Fatalf("unexpected authentication flow: logins=%d pages=%d writes=%d", logins, pages, writes)
			}
			client.invalidateSession()
			if client.csrfToken != "" || client.authenticated {
				t.Fatal("invalidating authentication must clear its CSRF token")
			}
		})
	}
}

func TestXUILoginCSRFRejectionDoesNotLoop(t *testing.T) {
	for _, tokenPresent := range []bool{true, false} {
		requests := 0
		client := xuiClientUpdateTestClient(t, func(req *http.Request) (*http.Response, error) {
			requests++
			if req.Method == http.MethodGet {
				page := `<html>Access denied</html>`
				if tokenPresent {
					page = `<meta name="csrf-token" content="test-token">`
				}
				return &http.Response{StatusCode: http.StatusOK, Request: req, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(page))}, nil
			}
			response := jsonResponse(t, req, map[string]any{"success": false})
			response.StatusCode = http.StatusForbidden
			return response, nil
		})
		client.config.APIToken = ""
		if err := client.login(context.Background()); !isXUIHTTPStatus(err, http.StatusForbidden) {
			t.Fatalf("expected the original HTTP rejection, got %v", err)
		}
		want := 2
		if tokenPresent {
			want = 3
		}
		if requests != want || client.authenticated {
			t.Fatalf("authentication must stop after one CSRF attempt: requests=%d", requests)
		}
	}
}
