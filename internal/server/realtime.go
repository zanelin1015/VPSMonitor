package server

import (
	"net"
	"net/http"
	"net/url"
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

func newRealtimeHub() *realtimeHub {
	return &realtimeHub{
		metrics:       make(map[string]model.AgentRealtimeMetrics),
		subscribers:   make(map[chan model.AgentRealtimeMetrics]struct{}),
		agentControls: make(map[string]*agentControlSession),
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
		ch:   make(chan model.AgentControlMessage, 8),
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
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	conn, err := dashboardWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	updates, snapshot, unsubscribe := a.realtime.subscribe()
	defer unsubscribe()

	if err := conn.WriteJSON(model.DashboardRealtimeMessage{Type: realtimeSnapshotMessage, Metrics: snapshot}); err != nil {
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
	observedIP := requestObservedIP(r)
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
		var metric model.AgentRealtimeMetrics
		if err := conn.ReadJSON(&metric); err != nil {
			return
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
		if metric.Summary.ObservedIP == "" {
			metric.Summary.ObservedIP = observedIP
		}
		a.realtime.update(metric)
	}
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
