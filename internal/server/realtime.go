package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"bridge-core/internal/model"
)

const (
	realtimeSnapshotMessage = "snapshot"
	realtimeMetricsMessage  = "metrics"
	realtimeMetricTTL       = 2 * time.Minute
)

type realtimeHub struct {
	mu            sync.RWMutex
	metrics       map[string]model.AgentRealtimeMetrics
	subscribers   map[chan model.AgentRealtimeMetrics]struct{}
	agentControls map[string]*agentControlSession
	terminals     map[string]*terminalRelaySession
}

type agentControlSession struct {
	ch        chan model.AgentControlMessage
	done      chan struct{}
	closeOnce sync.Once
}

func (s *agentControlSession) close() {
	s.closeOnce.Do(func() {
		close(s.done)
	})
}

type terminalRelaySession struct {
	agentID   string
	sessionID string
	ch        chan model.TerminalMessage
	done      chan struct{}
	closeOnce sync.Once
}

func (s *terminalRelaySession) close() {
	s.closeOnce.Do(func() {
		close(s.done)
	})
}

func newRealtimeHub() *realtimeHub {
	return &realtimeHub{
		metrics:       make(map[string]model.AgentRealtimeMetrics),
		subscribers:   make(map[chan model.AgentRealtimeMetrics]struct{}),
		agentControls: make(map[string]*agentControlSession),
		terminals:     make(map[string]*terminalRelaySession),
	}
}

func (h *realtimeHub) update(metric model.AgentRealtimeMetrics) {
	if metric.AgentID == "" {
		return
	}
	if metric.ReportedAt.IsZero() {
		metric.ReportedAt = time.Now().UTC()
	} else {
		metric.ReportedAt = metric.ReportedAt.UTC()
	}

	h.mu.Lock()
	h.metrics[metric.AgentID] = metric
	for ch := range h.subscribers {
		select {
		case ch <- metric:
		default:
		}
	}
	h.mu.Unlock()
}

func (h *realtimeHub) snapshot() []model.AgentRealtimeMetrics {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]model.AgentRealtimeMetrics, 0, len(h.metrics))
	for _, metric := range h.metrics {
		if realtimeMetricFresh(metric) {
			result = append(result, metric)
		}
	}
	return result
}

func (h *realtimeHub) subscribe() (chan model.AgentRealtimeMetrics, []model.AgentRealtimeMetrics, func()) {
	ch := make(chan model.AgentRealtimeMetrics, 32)
	h.mu.Lock()
	snapshot := make([]model.AgentRealtimeMetrics, 0, len(h.metrics))
	for _, metric := range h.metrics {
		if realtimeMetricFresh(metric) {
			snapshot = append(snapshot, metric)
		}
	}
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subscribers[ch]; ok {
			delete(h.subscribers, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, snapshot, cancel
}

func (h *realtimeHub) applyToDashboard(view *model.GlobalDashboardView) {
	if view == nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for i := range view.Agents {
		if metric, ok := h.metrics[view.Agents[i].AgentID]; ok && realtimeMetricFresh(metric) {
			mergeRealtimeSummary(&view.Agents[i].Summary, metric.Summary)
			realtimeAt := metric.ReportedAt
			view.Agents[i].RealtimeAt = &realtimeAt
			if view.Agents[i].AgentName == "" && metric.AgentName != "" {
				view.Agents[i].AgentName = metric.AgentName
			}
		}
	}
}

func (h *realtimeHub) applyToAgentItems(items []model.AgentListItem) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for i := range items {
		if metric, ok := h.metrics[items[i].AgentID]; ok && realtimeMetricFresh(metric) {
			mergeRealtimeSummary(&items[i].Summary, metric.Summary)
			realtimeAt := metric.ReportedAt
			items[i].RealtimeAt = &realtimeAt
			if items[i].AgentName == "" && metric.AgentName != "" {
				items[i].AgentName = metric.AgentName
			}
		}
	}
}

func (h *realtimeHub) registerAgentControl(agentID string) *agentControlSession {
	session := &agentControlSession{
		ch:   make(chan model.AgentControlMessage, 512),
		done: make(chan struct{}),
	}
	h.mu.Lock()
	if previous := h.agentControls[agentID]; previous != nil {
		previous.close()
	}
	h.agentControls[agentID] = session
	h.mu.Unlock()
	return session
}

func (h *realtimeHub) unregisterAgentControl(agentID string, session *agentControlSession) {
	h.mu.Lock()
	if current := h.agentControls[agentID]; current == session {
		delete(h.agentControls, agentID)
	}
	h.mu.Unlock()
	session.close()
}

func (h *realtimeHub) sendAgentControl(agentID string, message model.AgentControlMessage) bool {
	h.mu.RLock()
	session := h.agentControls[agentID]
	h.mu.RUnlock()
	if session == nil {
		return false
	}
	select {
	case <-session.done:
		return false
	default:
	}
	select {
	case session.ch <- message:
		return true
	case <-session.done:
		return false
	default:
		return false
	}
}

func (h *realtimeHub) removeAgent(agentID string) {
	h.mu.Lock()
	delete(h.metrics, agentID)
	if session := h.agentControls[agentID]; session != nil {
		session.close()
		delete(h.agentControls, agentID)
	}
	for sessionID, terminal := range h.terminals {
		if terminal.agentID == agentID {
			terminal.close()
			delete(h.terminals, sessionID)
		}
	}
	h.mu.Unlock()
}

func (h *realtimeHub) registerTerminal(agentID, sessionID string) *terminalRelaySession {
	session := &terminalRelaySession{
		agentID:   agentID,
		sessionID: sessionID,
		ch:        make(chan model.TerminalMessage, 128),
		done:      make(chan struct{}),
	}
	h.mu.Lock()
	if previous := h.terminals[sessionID]; previous != nil {
		previous.close()
	}
	h.terminals[sessionID] = session
	h.mu.Unlock()
	return session
}

func (h *realtimeHub) unregisterTerminal(sessionID string, session *terminalRelaySession) {
	h.mu.Lock()
	if current := h.terminals[sessionID]; current == session {
		delete(h.terminals, sessionID)
	}
	h.mu.Unlock()
	session.close()
}

func (h *realtimeHub) relayTerminalMessage(message model.TerminalMessage) bool {
	if message.SessionID == "" {
		return false
	}
	h.mu.RLock()
	session := h.terminals[message.SessionID]
	h.mu.RUnlock()
	if session == nil || (message.AgentID != "" && message.AgentID != session.agentID) {
		return false
	}
	select {
	case session.ch <- message:
		return true
	case <-session.done:
		return false
	default:
		return false
	}
}

func realtimeMetricFresh(metric model.AgentRealtimeMetrics) bool {
	if metric.ReportedAt.IsZero() {
		return false
	}
	return time.Since(metric.ReportedAt) <= realtimeMetricTTL
}

func mergeRealtimeSummary(target *model.VPSSummary, source model.VPSSummary) {
	if target == nil {
		return
	}
	if source.Hostname != "" {
		target.Hostname = source.Hostname
	}
	if source.ObservedIP != "" {
		target.ObservedIP = source.ObservedIP
	}
	if source.ServerSeenIP != "" {
		target.ServerSeenIP = source.ServerSeenIP
	}
	if source.PublicIPv4 != "" {
		target.PublicIPv4 = source.PublicIPv4
	}
	if source.PublicIPv6 != "" {
		target.PublicIPv6 = source.PublicIPv6
	}
	target.CPU = source.CPU
	if source.MemTotal > 0 {
		target.MemUsed = source.MemUsed
		target.MemTotal = source.MemTotal
	}
	target.NetIOUp = source.NetIOUp
	target.NetIODown = source.NetIODown
	if source.NetTrafficSent > 0 || source.NetTrafficRecv > 0 || source.NetTrafficTotal > 0 {
		target.NetTrafficSent = source.NetTrafficSent
		target.NetTrafficRecv = source.NetTrafficRecv
		target.NetTrafficTotal = source.NetTrafficTotal
	}
	if source.XrayState != "" {
		target.XrayState = source.XrayState
	}
}

func (a *App) handleDashboardRealtime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}

	conn, err := dashboardWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	updates, snapshot, unsubscribe := a.realtime.subscribe()
	defer unsubscribe()

	if err := conn.WriteJSON(model.DashboardRealtimeMessage{Type: realtimeSnapshotMessage, Metrics: a.filterRealtimeMetricsForAdmin(user, snapshot)}); err != nil {
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	pingTicker := time.NewTicker(25 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case metric, ok := <-updates:
			if !ok {
				return
			}
			if !a.adminCanAccessAgent(user, metric.AgentID) {
				continue
			}
			metric = a.sanitizeRealtimeMetricForAdmin(user, metric)
			if err := conn.WriteJSON(model.DashboardRealtimeMessage{Type: realtimeMetricsMessage, Metric: &metric}); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case <-done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (a *App) handleAgentTerminalWS(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if !isRootAdmin(user) {
		writeError(w, http.StatusForbidden, "only root admin can open remote terminal")
		return
	}
	if !a.adminCanAccessAgent(user, agentID) {
		writeError(w, http.StatusForbidden, "agent is not assigned to this account")
		return
	}
	if _, found, err := a.store.GetAgent(agentID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	conn, err := dashboardWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(64 * 1024)

	sessionID := randomTerminalSessionID()
	relay := a.realtime.registerTerminal(agentID, sessionID)
	defer a.realtime.unregisterTerminal(sessionID, relay)

	shell := strings.TrimSpace(r.URL.Query().Get("shell"))
	cols := queryInt(r, "cols", 120)
	rows := queryInt(r, "rows", 36)
	if !a.realtime.sendAgentControl(agentID, model.AgentControlMessage{
		Type: model.AgentControlTerminalOpen,
		Payload: map[string]any{
			"session_id": sessionID,
			"shell":      shell,
			"cols":       cols,
			"rows":       rows,
		},
	}) {
		_ = conn.WriteJSON(model.TerminalMessage{Type: model.TerminalMessageError, SessionID: sessionID, Error: "Client 实时连接不在线，请确认 Client 已更新并保持在线"})
		return
	}

	done := make(chan struct{})
	adminErrors := make(chan model.TerminalMessage, 1)
	go func() {
		defer close(done)
		for {
			var message model.TerminalMessage
			if err := conn.ReadJSON(&message); err != nil {
				return
			}
			if message.SessionID == "" {
				message.SessionID = sessionID
			}
			if message.SessionID != sessionID {
				continue
			}
			control := terminalControlFromAdminMessage(message)
			if control.Type == "" {
				continue
			}
			if !a.realtime.sendAgentControl(agentID, control) {
				select {
				case adminErrors <- model.TerminalMessage{Type: model.TerminalMessageError, SessionID: sessionID, Error: "Client 实时连接已断开"}:
				default:
				}
				return
			}
		}
	}()

	pingTicker := time.NewTicker(25 * time.Second)
	defer pingTicker.Stop()
	for {
		select {
		case message := <-relay.ch:
			if err := conn.WriteJSON(message); err != nil {
				return
			}
		case message := <-adminErrors:
			if err := conn.WriteJSON(message); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case <-done:
			a.realtime.sendAgentControl(agentID, model.AgentControlMessage{
				Type:    model.AgentControlTerminalClose,
				Payload: map[string]any{"session_id": sessionID},
			})
			return
		case <-r.Context().Done():
			return
		case <-relay.done:
			return
		}
	}
}

func terminalControlFromAdminMessage(message model.TerminalMessage) model.AgentControlMessage {
	payload := map[string]any{"session_id": message.SessionID}
	switch message.Type {
	case "input", model.AgentControlTerminalInput:
		payload["data"] = message.Data
		return model.AgentControlMessage{Type: model.AgentControlTerminalInput, Payload: payload}
	case "resize", model.AgentControlTerminalResize:
		payload["cols"] = message.Cols
		payload["rows"] = message.Rows
		return model.AgentControlMessage{Type: model.AgentControlTerminalResize, Payload: payload}
	case "close", model.AgentControlTerminalClose:
		return model.AgentControlMessage{Type: model.AgentControlTerminalClose, Payload: payload}
	default:
		return model.AgentControlMessage{}
	}
}

func (a *App) handleAgentMetricsWS(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	token := r.Header.Get("X-Agent-Token")
	if token == "" {
		token = r.URL.Query().Get("agent_token")
	}
	if !a.isAuthorized(agentID, token) {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}

	conn, err := agentWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(32 * 1024)
	serverSeenIP := requestObservedIP(r)
	controlSession := a.realtime.registerAgentControl(agentID)
	defer a.realtime.unregisterAgentControl(agentID, controlSession)

	go func() {
		for {
			select {
			case message := <-controlSession.ch:
				if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
					_ = conn.Close()
					return
				}
				if err := conn.WriteJSON(message); err != nil {
					_ = conn.Close()
					return
				}
			case <-controlSession.done:
				return
			case <-r.Context().Done():
				return
			}
		}
	}()

	agent, found, err := a.store.GetAgent(agentID)
	if err != nil {
		return
	}

	for {
		var raw json.RawMessage
		if err := conn.ReadJSON(&raw); err != nil {
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err == nil && strings.HasPrefix(envelope.Type, "terminal_") {
			var message model.TerminalMessage
			if err := json.Unmarshal(raw, &message); err == nil {
				if message.AgentID == "" {
					message.AgentID = agentID
				}
				a.realtime.relayTerminalMessage(message)
			}
			continue
		}
		var metric model.AgentRealtimeMetrics
		if err := json.Unmarshal(raw, &metric); err != nil {
			continue
		}
		if metric.AgentID == "" {
			metric.AgentID = agentID
		}
		if metric.AgentID != agentID {
			continue
		}
		if metric.AgentName == "" && found {
			metric.AgentName = agent.AgentName
		}
		if isUsableObservedIP(serverSeenIP) {
			metric.Summary.ServerSeenIP = serverSeenIP
		}
		a.realtime.update(metric)
	}
}

func randomTerminalSessionID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func requestObservedIP(r *http.Request) string {
	for _, header := range []string{"CF-Connecting-IP", "X-Real-IP", "X-Forwarded-For"} {
		value := r.Header.Get(header)
		if value == "" {
			continue
		}
		for _, part := range strings.Split(value, ",") {
			candidate := strings.TrimSpace(part)
			if ip := net.ParseIP(candidate); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}
	if ip := net.ParseIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return r.RemoteAddr
}

func isUsableObservedIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified()
}

var dashboardWSUpgrader = websocket.Upgrader{
	CheckOrigin: sameOriginWebSocket,
}

var agentWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

func sameOriginWebSocket(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return originURL.Host == r.Host
}
