package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

type supportRoundTripFunc func(*http.Request) (*http.Response, error)

func (f supportRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSupportHandlersEnforceAreaOwnershipAndSendOfflineTelegram(t *testing.T) {
	tempDir := t.TempDir()
	cipher, err := store.LoadOrCreateCredentialCipher(filepath.Join(tempDir, "credential.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateCredentialCipher: %v", err)
	}
	s, err := store.NewSQLiteStore(filepath.Join(tempDir, "bridge.db"), store.WithCredentialCipher(cipher))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer s.Close()
	if err := s.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatalf("EnsureAdminAccount: %v", err)
	}
	enabled := true
	areaOne, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area-one", Password: "password123", Enabled: &enabled})
	if err != nil {
		t.Fatalf("CreateAreaManager area one: %v", err)
	}
	if _, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "area-two", Password: "password123", Enabled: &enabled}); err != nil {
		t.Fatalf("CreateAreaManager area two: %v", err)
	}
	customer, err := s.CreateCustomerForOwner(model.CustomerAccountRequest{Username: "alice", Password: "password123", DisplayName: "Alice"}, model.AdminRoleAreaManager, areaOne.ID)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner: %v", err)
	}
	if _, err := s.CreateTelegramBot(model.TelegramBotRequest{Name: "Support", BotToken: "token", ChatID: "123", Enabled: true}); err != nil {
		t.Fatalf("CreateTelegramBot: %v", err)
	}

	telegramBodies := make(chan string, 1)
	alerts := newAlertService(s)
	alerts.http = &http.Client{Transport: supportRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(request.Body)
		telegramBodies <- string(payload)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})}
	app := &App{store: s, alerts: alerts, supportPresence: newSupportPresenceHub()}
	handler := app.Handler()

	customerToken, _, err := s.CreateCustomerSession(customer.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateCustomerSession: %v", err)
	}
	post := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/customer/support/messages", bytes.NewBufferString(`{"body":"Need help"}`))
	post.Header.Set("Content-Type", "application/json")
	post.AddCookie(&http.Cookie{Name: customerSessionCookieName, Value: customerToken})
	postRecorder := httptest.NewRecorder()
	handler.ServeHTTP(postRecorder, post)
	if postRecorder.Code != http.StatusCreated {
		t.Fatalf("customer support post status=%d body=%s", postRecorder.Code, postRecorder.Body.String())
	}
	select {
	case payload := <-telegramBodies:
		if !strings.Contains(payload, "Need help") || !strings.Contains(payload, "Alice") {
			t.Fatalf("unexpected Telegram payload: %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("offline customer message did not trigger Telegram notification")
	}

	conversation, found, err := s.GetSupportConversationByCustomer(customer.ID, model.SupportSenderAdmin)
	if err != nil || !found {
		t.Fatalf("GetSupportConversationByCustomer: %#v found=%v err=%v", conversation, found, err)
	}
	areaOneToken := supportAdminToken(t, s, "area-one")
	areaTwoToken := supportAdminToken(t, s, "area-two")

	list := httptest.NewRequest(http.MethodGet, "/api/v1/admin/support", nil)
	list.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: areaOneToken})
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("area support list status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listResponse model.SupportConversationListResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listResponse); err != nil || len(listResponse.Conversations) != 1 || listResponse.Conversations[0].CustomerID != customer.ID {
		t.Fatalf("unexpected area support list: %#v err=%v", listResponse, err)
	}

	forbidden := httptest.NewRequest(http.MethodGet, "/api/v1/admin/support/conversations/"+strconvFormat(conversation.ID), nil)
	forbidden.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: areaTwoToken})
	forbiddenRecorder := httptest.NewRecorder()
	handler.ServeHTTP(forbiddenRecorder, forbidden)
	if forbiddenRecorder.Code != http.StatusNotFound {
		t.Fatalf("other area manager status=%d body=%s", forbiddenRecorder.Code, forbiddenRecorder.Body.String())
	}

	areaTwoUser, ok, err := s.AuthenticateAdmin("area-two", "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin area two: %#v ok=%v err=%v", areaTwoUser, ok, err)
	}
	disconnect := app.supportPresence.connect(areaTwoUser)
	defer disconnect()
	get := httptest.NewRequest(http.MethodGet, "/api/v1/customer/support?mark_read=0", nil)
	get.AddCookie(&http.Cookie{Name: customerSessionCookieName, Value: customerToken})
	getRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getRecorder, get)
	var thread model.SupportThreadResponse
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &thread); err != nil {
		t.Fatalf("decode customer thread: %v body=%s", err, getRecorder.Body.String())
	}
	if thread.SupportOnline {
		t.Fatal("unrelated area manager must not make customer support online")
	}
	disconnectOne := app.supportPresence.connect(model.AdminUser{ID: areaOne.ID, Role: model.AdminRoleAreaManager})
	defer disconnectOne()
	getOnline := httptest.NewRequest(http.MethodGet, "/api/v1/customer/support?mark_read=0", nil)
	getOnline.AddCookie(&http.Cookie{Name: customerSessionCookieName, Value: customerToken})
	getOnlineRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getOnlineRecorder, getOnline)
	if err := json.Unmarshal(getOnlineRecorder.Body.Bytes(), &thread); err != nil || !thread.SupportOnline {
		t.Fatalf("expected matching area manager online: %#v err=%v", thread, err)
	}

	onlineCustomer, err := s.CreateCustomerForOwner(model.CustomerAccountRequest{Username: "online-customer", Password: "password123"}, model.AdminRoleAreaManager, areaOne.ID)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner online customer: %v", err)
	}
	onlineCustomerToken, _, err := s.CreateCustomerSession(onlineCustomer.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateCustomerSession online customer: %v", err)
	}
	onlinePost := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/customer/support/messages", bytes.NewBufferString(`{"body":"Online help"}`))
	onlinePost.Header.Set("Content-Type", "application/json")
	onlinePost.AddCookie(&http.Cookie{Name: customerSessionCookieName, Value: onlineCustomerToken})
	onlinePostRecorder := httptest.NewRecorder()
	handler.ServeHTTP(onlinePostRecorder, onlinePost)
	if onlinePostRecorder.Code != http.StatusCreated {
		t.Fatalf("online customer support post status=%d body=%s", onlinePostRecorder.Code, onlinePostRecorder.Body.String())
	}
	select {
	case payload := <-telegramBodies:
		t.Fatalf("online customer message unexpectedly triggered Telegram: %s", payload)
	case <-time.After(100 * time.Millisecond):
	}
}

func supportAdminToken(t *testing.T, s *store.SQLiteStore, username string) string {
	t.Helper()
	user, ok, err := s.AuthenticateAdmin(username, "password123")
	if err != nil || !ok {
		t.Fatalf("AuthenticateAdmin %s: %#v ok=%v err=%v", username, user, ok, err)
	}
	token, _, err := s.CreateAdminSession(user, time.Hour)
	if err != nil {
		t.Fatalf("CreateAdminSession %s: %v", username, err)
	}
	return token
}

func strconvFormat(value int64) string {
	return strconv.FormatInt(value, 10)
}
