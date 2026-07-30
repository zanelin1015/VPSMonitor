package panels

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bridge-core/internal/config"
)

func TestCollectClientTrafficUsesLightweightAPI(t *testing.T) {
	const token = "current-api-token"
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/panel/api/inbounds/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"obj": []map[string]any{{
					"id": 7, "tag": "vless-20001",
					"clientStats": []map[string]any{{"email": "allowed@example.com", "up": 120, "down": 340}},
				}},
			})
		case "/panel/api/clients/list":
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected x-ui request: %s", r.URL.Path)
		}
	}))
	defer panel.Close()

	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  panel.URL,
		DBPath:   filepath.Join(t.TempDir(), "missing.db"),
		APIToken: token,
	}, time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	traffic, err := client.CollectClientTraffic(context.Background())
	if err != nil {
		t.Fatalf("CollectClientTraffic: %v", err)
	}
	if len(traffic) != 1 || traffic[0].InboundID != 7 || traffic[0].InboundTag != "vless-20001" || traffic[0].Email != "allowed@example.com" || traffic[0].Up != 120 || traffic[0].Down != 340 {
		t.Fatalf("unexpected API traffic: %#v", traffic)
	}
}

func TestCollectClientTrafficUsesLocalSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE inbounds (id INTEGER PRIMARY KEY, tag TEXT)`,
		`CREATE TABLE client_traffics (
			id INTEGER PRIMARY KEY, inbound_id INTEGER, enable BOOLEAN, email TEXT,
			up INTEGER, down INTEGER, all_time INTEGER, expiry_time INTEGER,
			total INTEGER, reset INTEGER, last_online INTEGER
		)`,
		`INSERT INTO inbounds (id, tag) VALUES (9, 'ss-20009')`,
		`INSERT INTO client_traffics
			(id, inbound_id, enable, email, up, down, all_time, expiry_time, total, reset, last_online)
			VALUES (1, 9, 1, 'local@example.com', 555, 777, 1332, 0, 0, 0, 0)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	client, err := NewXUIClient(config.XUIConfig{Enabled: true, DBPath: dbPath}, time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	traffic, err := client.CollectClientTraffic(context.Background())
	if err != nil {
		t.Fatalf("CollectClientTraffic: %v", err)
	}
	if len(traffic) != 1 || traffic[0].InboundID != 9 || traffic[0].InboundTag != "ss-20009" || traffic[0].Email != "local@example.com" || traffic[0].Up != 555 || traffic[0].Down != 777 {
		t.Fatalf("unexpected local traffic: %#v", traffic)
	}
}

func TestCollectClientTrafficFailureIsReturnedWithoutSnapshotCollection(t *testing.T) {
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	}))
	defer panel.Close()

	client, err := NewXUIClient(config.XUIConfig{
		Enabled:  true,
		BaseURL:  panel.URL,
		DBPath:   filepath.Join(t.TempDir(), "missing.db"),
		APIToken: "token",
	}, time.Second)
	if err != nil {
		t.Fatalf("NewXUIClient: %v", err)
	}
	if _, err := client.CollectClientTraffic(context.Background()); err == nil {
		t.Fatal("expected lightweight traffic collection error")
	}
}
