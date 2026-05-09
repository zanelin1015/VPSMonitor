package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"sync"
	"time"

	"bridge-core/internal/devdata"
)

var mockXUI = struct {
	sync.Mutex
	inbounds  []map[string]any
	outbounds []map[string]any
	rules     []map[string]any
}{
	inbounds:  devdata.DemoSnapshot("local-dev-01", "Local Dev VPS", time.Now().UTC()).XUI.Inbounds,
	outbounds: devdata.DemoOutbounds(),
	rules:     devdata.DemoRoutingRules(),
}

func main() {
	listenAddr := flag.String("listen", "127.0.0.1:19090", "mock x-ui listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/login", handleXUILogin)
	mux.HandleFunc("/panel/api/server/status", handleXUIStatus)
	mux.HandleFunc("/panel/api/inbounds/list", handleXUIInbounds)
	mux.HandleFunc("/panel/api/inbounds/add", handleXUIAddInbound)
	mux.HandleFunc("/panel/api/xray/", handleXUISetting)
	mux.HandleFunc("/panel/api/xray/update", handleXUIUpdateSetting)
	mux.HandleFunc("/panel/api/server/restartXrayService", handleXUIRestart)
	mux.HandleFunc("/panel/api/server/getConfigJson", handleXUIConfig)
	mux.HandleFunc("/panel/xray/getOutboundsTraffic", handleXUIOutboundTraffic)
	mux.HandleFunc("/api/v1/login", handleNezhaLogin)
	mux.HandleFunc("/api/v1/server", handleNezhaServer)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})

	log.Printf("mock x-ui panels listening on %s", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatalf("mock panels stopped: %v", err)
	}
}

func handleXUILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"success": true, "msg": "ok"})
}

func handleXUIStatus(w http.ResponseWriter, _ *http.Request) {
	snapshot := devdata.DemoSnapshot("local-dev-01", "Local Dev VPS", time.Now().UTC())
	writeJSON(w, map[string]any{"success": true, "obj": snapshot.XUI.ServerStatus})
}

func handleXUIInbounds(w http.ResponseWriter, _ *http.Request) {
	mockXUI.Lock()
	defer mockXUI.Unlock()
	writeJSON(w, map[string]any{"success": true, "obj": mockXUI.inbounds})
}

func handleXUIAddInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var inbound map[string]any
	if err := json.NewDecoder(r.Body).Decode(&inbound); err != nil {
		writeJSON(w, map[string]any{"success": false, "msg": err.Error()})
		return
	}
	mockXUI.Lock()
	if inbound["id"] == nil {
		inbound["id"] = len(mockXUI.inbounds) + 100
	}
	if inbound["tag"] == nil {
		inbound["tag"] = "inbound-dev-added"
	}
	mockXUI.inbounds = append(mockXUI.inbounds, inbound)
	mockXUI.Unlock()
	writeJSON(w, map[string]any{"success": true, "msg": "inbound added", "obj": inbound})
}

func handleXUISetting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mockXUI.Lock()
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"inbounds":  []map[string]any{},
		"outbounds": mockXUI.outbounds,
		"routing":   map[string]any{"rules": mockXUI.rules},
	}
	mockXUI.Unlock()
	wrapper, _ := json.Marshal(map[string]any{
		"xraySetting":     cfg,
		"inboundTags":     []string{},
		"outboundTestUrl": "https://www.google.com/generate_204",
	})
	writeJSON(w, map[string]any{"success": true, "obj": string(wrapper)})
}

func handleXUIUpdateSetting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, map[string]any{"success": false, "msg": err.Error()})
		return
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(r.Form.Get("xraySetting")), &cfg); err != nil {
		writeJSON(w, map[string]any{"success": false, "msg": err.Error()})
		return
	}
	mockXUI.Lock()
	mockXUI.outbounds = objectList(cfg["outbounds"])
	if routing, ok := cfg["routing"].(map[string]any); ok {
		mockXUI.rules = objectList(routing["rules"])
	}
	mockXUI.Unlock()
	writeJSON(w, map[string]any{"success": true, "msg": "xray setting updated"})
}

func handleXUIRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"success": true, "msg": "xray restarted"})
}

func handleXUIConfig(w http.ResponseWriter, _ *http.Request) {
	mockXUI.Lock()
	defer mockXUI.Unlock()
	writeJSON(w, map[string]any{
		"success": true,
		"obj": map[string]any{
			"outbounds": mockXUI.outbounds,
			"routing": map[string]any{
				"rules": mockXUI.rules,
			},
		},
	})
}

func handleXUIOutboundTraffic(w http.ResponseWriter, _ *http.Request) {
	snapshot := devdata.DemoSnapshot("local-dev-01", "Local Dev VPS", time.Now().UTC())
	writeJSON(w, map[string]any{"success": true, "obj": snapshot.XUI.OutboundTraffic})
}

func handleNezhaLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"success": true,
		"data": map[string]any{
			"token": "mock-nezha-token",
		},
	})
}

func handleNezhaServer(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		http.Error(w, "missing authorization", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{
		"success": true,
		"data": []map[string]any{
			{
				"id":   7,
				"name": "local-dev-vps",
				"uuid": "mock-nezha-uuid",
				"geoip": map[string]any{
					"ip": map[string]any{
						"ipv4_addr": "203.0.113.24",
						"ipv6_addr": "2001:db8::24",
					},
				},
				"host": map[string]any{
					"mem_total": 1024 * 1024 * 1024,
				},
				"state": map[string]any{
					"cpu":      18.7,
					"mem_used": 420 * 1024 * 1024,
				},
			},
		},
	})
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func objectList(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			result = append(result, obj)
		}
	}
	return result
}
