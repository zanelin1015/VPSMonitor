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

func TestPublicGuestSupportBindsTemporaryConversationToIP(t *testing.T) {
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
	app := &App{store: s, supportPresence: newSupportPresenceHub()}
	handler := app.Handler()

	get := httptest.NewRequest(http.MethodGet, "https://monitor.example/api/v1/public/support?mark_read=0", nil)
	get.RemoteAddr = "198.51.100.24:443"
	getRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("guest support get status=%d body=%s", getRecorder.Code, getRecorder.Body.String())
	}
	guestCookie := authCookie(t, getRecorder, guestSupportCookieName)
	if guestCookie == nil || guestCookie.Value == "" || guestCookie.Path != "/api/v1/" {
		t.Fatalf("expected API-scoped guest support cookie, got %#v", guestCookie)
	}

	post := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/public/support/messages", bytes.NewBufferString(`{"body":"Guest question"}`))
	post.RemoteAddr = "198.51.100.24:443"
	post.Header.Set("Content-Type", "application/json")
	post.AddCookie(guestCookie)
	postRecorder := httptest.NewRecorder()
	handler.ServeHTTP(postRecorder, post)
	if postRecorder.Code != http.StatusCreated {
		t.Fatalf("guest support post status=%d body=%s", postRecorder.Code, postRecorder.Body.String())
	}

	adminToken := supportAdminToken(t, s, "admin")
	list := httptest.NewRequest(http.MethodGet, "/api/v1/admin/support", nil)
	list.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: adminToken})
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("admin support list status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var response model.SupportConversationListResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode guest support list: %v", err)
	}
	if len(response.Conversations) != 1 || !response.Conversations[0].IsGuest || response.Conversations[0].GuestIP != "198.51.100.24" {
		t.Fatalf("expected one IP-labelled guest conversation, got %#v", response.Conversations)
	}
	if customers, err := s.ListCustomers(); err != nil {
		t.Fatalf("list customers: %v", err)
	} else if len(customers) != 0 {
		t.Fatalf("guest account should stay out of customer management, got %#v", customers)
	}
}

func TestCustomerLoginClaimsGuestSupportConversation(t *testing.T) {
	tempDir := t.TempDir()
	s, err := store.NewSQLiteStore(filepath.Join(tempDir, "bridge.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer s.Close()
	customer, err := s.CreateCustomerForOwner(model.CustomerAccountRequest{
		Username: "alice", Password: "password123", DisplayName: "Alice",
	}, model.AdminRoleRoot, 1)
	if err != nil {
		t.Fatalf("CreateCustomerForOwner: %v", err)
	}
	app := &App{store: s, loginLimiter: newLoginRateLimiter(), supportPresence: newSupportPresenceHub()}
	handler := app.Handler()

	guestGet := httptest.NewRequest(http.MethodGet, "https://monitor.example/api/v1/public/support", nil)
	guestGet.RemoteAddr = "198.51.100.25:443"
	guestGetRecorder := httptest.NewRecorder()
	handler.ServeHTTP(guestGetRecorder, guestGet)
	if guestGetRecorder.Code != http.StatusOK {
		t.Fatalf("guest support get status=%d body=%s", guestGetRecorder.Code, guestGetRecorder.Body.String())
	}
	guestCookie := authCookie(t, guestGetRecorder, guestSupportCookieName)
	guestPost := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/public/support/messages", bytes.NewBufferString(`{"body":"Before login"}`))
	guestPost.RemoteAddr = "198.51.100.25:443"
	guestPost.Header.Set("Content-Type", "application/json")
	guestPost.AddCookie(guestCookie)
	guestPostRecorder := httptest.NewRecorder()
	handler.ServeHTTP(guestPostRecorder, guestPost)
	if guestPostRecorder.Code != http.StatusCreated {
		t.Fatalf("guest support post status=%d body=%s", guestPostRecorder.Code, guestPostRecorder.Body.String())
	}
	guestConversations, err := s.ListSupportConversations(model.AdminRoleRoot, 1)
	if err != nil || len(guestConversations) != 1 || !guestConversations[0].IsGuest {
		t.Fatalf("expected guest conversation before login: %#v err=%v", guestConversations, err)
	}
	guestID := guestConversations[0].CustomerID

	login := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/customer/login", strings.NewReader(`{"username":"alice","password":"password123"}`))
	login.AddCookie(guestCookie)
	loginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("customer login status=%d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var linked model.CustomerLoginResponse
	if err := json.Unmarshal(loginRecorder.Body.Bytes(), &linked); err != nil {
		t.Fatalf("decode customer login: %v", err)
	}
	if linked.User.ID != customer.ID {
		t.Fatalf("login returned customer %d, want %d", linked.User.ID, customer.ID)
	}
	linkedConversation, found, err := s.GetSupportConversationByCustomer(customer.ID, model.SupportSenderAdmin)
	if err != nil || !found || linkedConversation.IsGuest {
		t.Fatalf("guest conversation was not linked to customer: %#v found=%v err=%v", linkedConversation, found, err)
	}
	messages, err := s.ListSupportMessages(linkedConversation.ID)
	if err != nil || len(messages) != 1 || messages[0].Body != "Before login" || messages[0].SenderAccountID != customer.ID {
		t.Fatalf("linked conversation did not preserve guest message: %#v err=%v", messages, err)
	}
	if conversations, err := s.ListSupportConversations(model.AdminRoleRoot, 1); err != nil || len(conversations) != 1 || conversations[0].CustomerID != customer.ID {
		t.Fatalf("expected one linked conversation after login: %#v err=%v", conversations, err)
	}
	if _, found, err := s.GetSupportConversationByCustomer(guestID, model.SupportSenderAdmin); err != nil || found {
		t.Fatalf("guest conversation should be removed after linking: found=%v err=%v", found, err)
	}
	guestCleared := false
	for _, cookie := range loginRecorder.Result().Cookies() {
		if cookie.Name == guestSupportCookieName && cookie.Path == "/api/v1/" && cookie.MaxAge < 0 {
			guestCleared = true
		}
	}
	if !guestCleared {
		t.Fatal("login should clear the root-scoped guest support cookie")
	}
}

func TestPublicGuestSupportScopesRecommendationURLToAreaManager(t *testing.T) {
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
	areaOne, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "scoped-one", Password: "password123", Enabled: &enabled})
	if err != nil {
		t.Fatalf("CreateAreaManager one: %v", err)
	}
	areaTwo, err := s.CreateAreaManager(model.AreaManagerAccountRequest{Username: "scoped-two", Password: "password123", Enabled: &enabled})
	if err != nil {
		t.Fatalf("CreateAreaManager two: %v", err)
	}
	if areaOne.RecommendationToken == "" || areaOne.RecommendationToken == areaTwo.RecommendationToken {
		t.Fatalf("area recommendation tokens should be distinct: %#v %#v", areaOne, areaTwo)
	}
	app := &App{store: s, supportPresence: newSupportPresenceHub()}
	handler := app.Handler()

	getScoped := func(token string) *http.Cookie {
		request := httptest.NewRequest(http.MethodGet, "https://monitor.example/api/v1/public/support?area="+token+"&mark_read=0", nil)
		request.RemoteAddr = "198.51.100.77:443"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("scoped guest support get status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		return authCookie(t, recorder, guestSupportCookieName)
	}

	areaOneCookie := getScoped(areaOne.RecommendationToken)
	if areaOneCookie == nil || areaOneCookie.Value == "" {
		t.Fatal("expected area one guest support cookie")
	}
	post := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/public/support/messages?area="+areaOne.RecommendationToken, bytes.NewBufferString(`{"body":"Area one question"}`))
	post.RemoteAddr = "198.51.100.77:443"
	post.Header.Set("Content-Type", "application/json")
	post.AddCookie(areaOneCookie)
	postRecorder := httptest.NewRecorder()
	handler.ServeHTTP(postRecorder, post)
	if postRecorder.Code != http.StatusCreated {
		t.Fatalf("area one guest support post status=%d body=%s", postRecorder.Code, postRecorder.Body.String())
	}

	areaTwoCookie := getScoped(areaTwo.RecommendationToken)
	postTwo := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/public/support/messages?area="+areaTwo.RecommendationToken, bytes.NewBufferString(`{"body":"Area two question"}`))
	postTwo.RemoteAddr = "198.51.100.77:443"
	postTwo.Header.Set("Content-Type", "application/json")
	postTwo.AddCookie(areaTwoCookie)
	postTwoRecorder := httptest.NewRecorder()
	handler.ServeHTTP(postTwoRecorder, postTwo)
	if postTwoRecorder.Code != http.StatusCreated {
		t.Fatalf("area two guest support post status=%d body=%s", postTwoRecorder.Code, postTwoRecorder.Body.String())
	}
	areaOneAdminToken := supportAdminToken(t, s, "scoped-one")
	areaTwoAdminToken := supportAdminToken(t, s, "scoped-two")
	listFor := func(token string) model.SupportConversationListResponse {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/support", nil)
		request.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: token})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("area support list status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var response model.SupportConversationListResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode area support list: %v", err)
		}
		return response
	}
	areaOneConversations := listFor(areaOneAdminToken).Conversations
	areaTwoConversations := listFor(areaTwoAdminToken).Conversations
	if len(areaOneConversations) != 1 || !areaOneConversations[0].IsGuest || areaOneConversations[0].OwnerID != areaOne.ID {
		t.Fatalf("area one should see only its guest conversation: %#v", areaOneConversations)
	}
	if len(areaTwoConversations) != 1 || !areaTwoConversations[0].IsGuest || areaTwoConversations[0].OwnerID != areaTwo.ID {
		t.Fatalf("area two should see only its guest conversation: %#v", areaTwoConversations)
	}
	if areaOneConversations[0].CustomerID == areaTwoConversations[0].CustomerID {
		t.Fatal("same IP on different recommendation URLs must use separate guest accounts")
	}
	if customers, err := s.ListCustomersForOwner(model.AdminRoleAreaManager, areaOne.ID); err != nil {
		t.Fatalf("list area one customers: %v", err)
	} else if len(customers) != 0 {
		t.Fatalf("scoped guest account should stay out of customer management: %#v", customers)
	}
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
	if _, err := s.CreateTelegramBot(model.TelegramBotRequest{Name: "Support", BotToken: "token", ChatID: "123", Enabled: true}, 0); err != nil {
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

func TestFormatOfflineSupportNotificationUsesBeijingTime(t *testing.T) {
	text := formatOfflineSupportNotification(
		model.CustomerUser{Username: "alice", DisplayName: "Alice"},
		model.SupportConversation{ID: 9},
		model.SupportMessage{Body: "Need help", CreatedAt: time.Date(2026, 8, 5, 2, 51, 52, 0, time.UTC)},
		"https://monitor.example",
	)
	if !strings.Contains(text, "时间：2026-08-05 10:51:52") {
		t.Fatalf("expected Beijing notification time, got %q", text)
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
