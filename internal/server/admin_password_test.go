package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func TestAreaManagerChangesOwnPassword(t *testing.T) {
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "password.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	for _, username := range []string{"area", "other-area"} {
		if _, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: username, Password: "password123"}); err != nil {
			t.Fatal(err)
		}
	}
	login := func(username, password string) (model.AdminUser, string) {
		t.Helper()
		user, valid, err := s.AuthenticateAdmin(username, password)
		if err != nil || !valid {
			t.Fatalf("login %s: valid=%v err=%v", username, valid, err)
		}
		token, _, err := s.CreateAdminSession(user, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return user, token
	}
	area, currentToken := login("area", "password123")
	_, oldToken := login("area", "password123")
	_, rootToken := login("admin", "password123")
	_, otherToken := login("other-area", "password123")
	app := (&App{store: s}).Handler()
	request := func(token, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
		req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
		}
		return w
	}
	validBody := `{"current_password":"password123","new_password":"new-password123"}`
	request("", "/api/v1/admin/password", validBody, 401)
	request(rootToken, "/api/v1/admin/password", validBody, 403)
	request(currentToken, "/api/v1/admin/account", validBody, 403)
	for _, body := range []string{
		`{"current_password":"wrong","new_password":"new-password123"}`,
		`{"current_password":"","new_password":"new-password123"}`,
		`{"current_password":"password123","new_password":"short"}`,
		`{"current_password":"password123","new_password":""}`,
		`{"current_password":"password123","new_password":"new-password123","account_id":2}`,
	} {
		request(currentToken, "/api/v1/admin/password", body, 400)
	}
	if _, _, valid, err := s.ValidateAdminSession(oldToken); err != nil || !valid {
		t.Fatal("rejected updates revoked a session")
	}
	response := request(currentToken, "/api/v1/admin/password", validBody, 200)
	var updated model.AdminLoginResponse
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil || updated.User.ID != area.ID || updated.User.Role != model.AdminRoleAreaManager || updated.User.Username != "area" {
		t.Fatalf("account identity changed: %s", response.Body.String())
	}
	if _, valid, err := s.AuthenticateAdmin("area", "password123"); err != nil || valid {
		t.Fatal("old password still authenticates")
	}
	login("area", "new-password123")
	login("admin", "password123")
	login("other-area", "password123")
	for token, want := range map[string]bool{currentToken: true, oldToken: false, rootToken: true, otherToken: true} {
		if _, _, valid, err := s.ValidateAdminSession(token); err != nil || valid != want {
			t.Fatalf("session validity=%v want=%v err=%v", valid, want, err)
		}
	}
}
