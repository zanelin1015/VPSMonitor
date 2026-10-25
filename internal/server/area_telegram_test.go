package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

type areaTelegramDelivery struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}
type areaTelegramFixture struct {
	db                       *store.SQLiteStore
	app                      *App
	one, two                 model.AreaManagerAdminView
	adminBot, oneBot, twoBot model.TelegramBot
	deliveries               chan areaTelegramDelivery
}

func newAreaTelegramFixture(t *testing.T) areaTelegramFixture {
	t.Helper()
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.EnsureAdminAccount("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	enabled := true
	one, err := db.CreateAreaManager(model.AreaManagerAccountRequest{Username: "one", Password: "password123", Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	two, err := db.CreateAreaManager(model.AreaManagerAccountRequest{Username: "two", Password: "password123", Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	bots := make([]model.TelegramBot, 0, 3)
	for _, ownerID := range []int64{0, one.ID, two.ID} {
		bot, err := db.CreateTelegramBot(model.TelegramBotRequest{Name: fmt.Sprint(ownerID), BotToken: "fake-test-token", ChatID: fmt.Sprint(ownerID), Enabled: true}, ownerID)
		if err != nil {
			t.Fatal(err)
		}
		bots = append(bots, bot)
	}
	deliveries := make(chan areaTelegramDelivery, 100)
	alerts := newAlertService(db)
	alerts.http = &http.Client{Transport: supportRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var delivery areaTelegramDelivery
		if err := json.NewDecoder(r.Body).Decode(&delivery); err != nil {
			return nil, err
		}
		deliveries <- delivery
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})}
	app := &App{store: db, alerts: alerts, supportPresence: newSupportPresenceHub()}
	return areaTelegramFixture{db: db, app: app, one: one, two: two, adminBot: bots[0], oneBot: bots[1], twoBot: bots[2], deliveries: deliveries}
}

func TestAreaTelegramSettingsAreScoped(t *testing.T) {
	f := newAreaTelegramFixture(t)
	handler := f.app.Handler()
	call := func(method, path, body, user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://monitor.example/api/v1/admin/telegram-bots"+path, bytes.NewBufferString(body))
		if user != "" {
			req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: supportAdminToken(t, f.db, user)})
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	for user, botID := range map[string]int64{"admin": f.adminBot.ID, "one": f.oneBot.ID, "two": f.twoBot.ID} {
		rec := call(http.MethodGet, "", "", user)
		var bots []model.TelegramBot
		if err := json.Unmarshal(rec.Body.Bytes(), &bots); err != nil || rec.Code != 200 || len(bots) != 1 || bots[0].ID != botID {
			t.Fatalf("%s sees wrong bots: %d %s", user, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "fake-test-token") {
			t.Fatal("bot token disclosed")
		}
	}
	for _, user := range []string{"admin", "two"} {
		for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPost} {
			path := fmt.Sprintf("/%d", f.oneBot.ID)
			if method == http.MethodPost {
				path += "/test"
			}
			rec := call(method, path, `{"name":"hijack","bot_token":"steal","chat_id":"other","enabled":true}`, user)
			if rec.Code != 404 {
				t.Fatalf("cross-owner %s %s: %d %s", user, method, rec.Code, rec.Body.String())
			}
		}
	}
	rec := call(http.MethodPost, "", `{"name":"area only","bot_token":"new-area-token","chat_id":"new-area-chat","enabled":true,"area_manager_id":0,"notify_expiry":false,"notify_support":true}`, "one")
	if rec.Code != 200 {
		t.Fatalf("area bot create: %d %s", rec.Code, rec.Body.String())
	}
	if bots, _ := f.db.ListTelegramBots(0); len(bots) != 1 {
		t.Fatal("request body changed bot ownership")
	}
	bots, _ := f.db.ListTelegramBots(f.one.ID)
	if len(bots) != 2 || bots[0].NotifyExpiry || !bots[0].NotifySupport {
		t.Fatal("area notification settings not saved")
	}
	rec = call(http.MethodPost, fmt.Sprintf("/%d/test", f.oneBot.ID), "", "one")
	if rec.Code != 200 {
		t.Fatalf("own bot test: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case d := <-f.deliveries:
		if d.ChatID != fmt.Sprint(f.one.ID) {
			t.Fatal("wrong test recipient")
		}
	default:
		t.Fatal("own bot test did not send")
	}
	if _, err := f.db.CreateOperator(model.OperatorAccountRequest{Username: "operator", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	if rec := call(http.MethodGet, "", "", "operator"); rec.Code != 403 {
		t.Fatalf("operator gained bot access: %d", rec.Code)
	}
	if rec := call(http.MethodGet, "", "", ""); rec.Code != 401 {
		t.Fatalf("anonymous bot access: %d", rec.Code)
	}
}

func TestAreaTelegramExpiryUsesCurrentClientPermissions(t *testing.T) {
	f := newAreaTelegramFixture(t)
	if _, err := testRegisterAgent(f.db, model.AgentRegisterRequest{AgentID: "shared", AgentName: "internal-name"}); err != nil {
		t.Fatal(err)
	}
	agent, _, _ := f.db.GetAgent("shared")
	cfg := agent.Config
	cfg.CustomerDisplayName = "Public Client"
	cfg.Tags = []string{"root-private-tag"}
	if _, err := testUpdateAgentConfig(f.db, "shared", cfg); err != nil {
		t.Fatal(err)
	}
	grant, err := testCreateAreaManagerAssignment(f.db, f.one.ID, model.AreaManagerAssignmentRequest{AgentID: "shared", InboundID: 1, InboundTag: "node-1", ClientEmail: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testCreateAreaManagerAssignment(f.db, f.two.ID, model.AreaManagerAssignmentRequest{AgentID: "shared", InboundID: 1, InboundTag: "node-1", ClientEmail: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := testCreateAreaManagerAssignment(f.db, f.one.ID, model.AreaManagerAssignmentRequest{AgentID: "shared", InboundID: 2, InboundTag: "node-2", NodeOnly: true}); err != nil {
		t.Fatal(err)
	}
	created, err := f.db.CreateXUIActionWithActor("shared", model.XUIActionRequest{Kind: model.XUIActionAddClient, Payload: map[string]any{"inbound_id": 2, "inbound_tag": "node-2", "client": map[string]any{"email": "self-created"}}}, model.XUIActionActor{Role: model.AdminRoleAreaManager, AccountID: f.one.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.CompleteXUIAction("shared", created.ID, model.XUIActionResultRequest{Status: model.XUIActionStatusSucceeded}); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().AddDate(0, 0, 2).UnixMilli()
	save := func(aliceExpiry int64) {
		t.Helper()
		inbounds := []map[string]any{}
		for index, emails := range [][]string{{"alice", "bob", "private"}, {"self-created", "sibling"}} {
			clients := []map[string]any{}
			for _, email := range emails {
				when := expiry
				if email == "alice" {
					when = aliceExpiry
				}
				clients = append(clients, map[string]any{"id": email + "-uuid", "email": email, "enable": true, "expiryTime": when})
			}
			settings, _ := json.Marshal(map[string]any{"clients": clients})
			inbounds = append(inbounds, map[string]any{"id": index + 1, "tag": fmt.Sprintf("node-%d", index+1), "protocol": "vless", "enable": true, "port": 20001 + index, "settings": string(settings)})
		}
		if err := f.db.SaveSnapshot(model.AgentSnapshot{AgentID: "shared", ReportedAt: time.Now(), XUI: &model.XUISnapshot{Inbounds: inbounds}}); err != nil {
			t.Fatal(err)
		}
	}
	save(expiry)
	f.app.alerts.EvaluateAll()
	if len(f.deliveries) != 0 {
		t.Fatal("expiry sent outside the daily window")
	}
	f.app.alerts.EvaluateAllDaily()
	counts := map[string]int{}
	for len(f.deliveries) > 0 {
		d := <-f.deliveries
		counts[d.ChatID]++
		if d.ChatID != "0" {
			if strings.Contains(d.Text, "root-private-tag") || strings.Contains(d.Text, "internal-name") || strings.Contains(d.Text, "Client：private") || strings.Contains(d.Text, "Client：sibling") {
				t.Fatalf("area received private data: %s", d.Text)
			}
			if d.ChatID == fmt.Sprint(f.one.ID) && strings.Contains(d.Text, "Client：bob") {
				t.Fatal("one received two's client")
			}
			if d.ChatID == fmt.Sprint(f.two.ID) && (strings.Contains(d.Text, "Client：alice") || strings.Contains(d.Text, "Client：self-created")) {
				t.Fatal("two received one's client")
			}
		}
	}
	if counts["0"] != 5 || counts[fmt.Sprint(f.one.ID)] != 2 || counts[fmt.Sprint(f.two.ID)] != 1 {
		t.Fatalf("wrong expiry recipients: %v", counts)
	}
	f.app.alerts.EvaluateAllDaily()
	if len(f.deliveries) != 0 {
		t.Fatal("duplicate daily evaluation bypassed scoped cooldowns")
	}
	if err := f.db.DeleteAreaManagerAssignment(f.one.ID, grant.ID); err != nil {
		t.Fatal(err)
	}
	save(expiry + int64(time.Hour/time.Millisecond))
	f.app.alerts.EvaluateAllDaily()
	for len(f.deliveries) > 0 {
		if d := <-f.deliveries; d.ChatID != "0" {
			t.Fatalf("revoked/unchanged client sent again: %#v", d)
		}
	}
}

func TestAreaTelegramVisitorMessagesAreOwnedAndImmediate(t *testing.T) {
	f := newAreaTelegramFixture(t)
	disconnectRoot := f.app.supportPresence.connect(model.AdminUser{ID: 1, Role: model.AdminRoleRoot})
	defer disconnectRoot()
	disconnectOne := f.app.supportPresence.connect(model.AdminUser{ID: f.one.ID, Role: model.AdminRoleAreaManager})
	defer disconnectOne()
	handler := f.app.Handler()
	post := func(token, ip, body string) int {
		req := httptest.NewRequest(http.MethodPost, "https://monitor.example/api/v1/public/support/messages?area="+token, bytes.NewBufferString(`{"body":"`+body+`"}`))
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, tc := range []struct {
		area model.AreaManagerAdminView
		body string
	}{{f.one, "one guest"}, {f.two, "two guest"}} {
		if code := post(tc.area.RecommendationToken, "203.0.113.9", tc.body); code != 201 {
			t.Fatalf("visitor post: %d", code)
		}
		select {
		case d := <-f.deliveries:
			if d.ChatID != fmt.Sprint(tc.area.ID) || !strings.Contains(d.Text, tc.body) || !strings.Contains(d.Text, "访客 IP：203.0.113.9") {
				t.Fatalf("wrong guest recipient/body: %#v", d)
			}
		case <-time.After(time.Second):
			t.Fatal("online area did not receive guest notification")
		}
	}
	if code := post(f.one.RecommendationToken, "203.0.113.9", "same conversation"); code != 201 {
		t.Fatal(code)
	}
	no := false
	if _, err := f.db.UpdateTelegramBot(f.oneBot.ID, model.TelegramBotRequest{Enabled: true, NotifySupport: &no}, f.one.ID); err != nil {
		t.Fatal(err)
	}
	if code := post(f.one.RecommendationToken, "203.0.113.10", "muted guest"); code != 201 {
		t.Fatal(code)
	}
	if code := post("unknown-area", "203.0.113.11", "unknown guest"); code != 404 {
		t.Fatal(code)
	}
	select {
	case d := <-f.deliveries:
		t.Fatalf("unexpected duplicate/cross-owner/muted notification: %#v", d)
	case <-time.After(100 * time.Millisecond):
	}
}
