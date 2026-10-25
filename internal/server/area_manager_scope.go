package server

import (
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

type areaManagerClientScope struct {
	exactClients     map[string]struct{}
	inbounds         map[string]struct{}
	nodeOnlyInbounds map[string]struct{}
	realmPorts       map[string]struct{}
	haProxyPorts     map[string]struct{}
	agents           map[string]struct{}
}

type areaManagerOutboundScope struct {
	tags map[string]struct{}
}

func (a *App) areaManagerOutboundScope(user model.AdminUser) areaManagerOutboundScope {
	scope := areaManagerOutboundScope{tags: make(map[string]struct{})}
	if !isAreaManager(user) || user.ID <= 0 || a == nil || a.store == nil {
		return scope
	}
	grants, err := a.store.ListAreaManagerOutboundGrants(user.ID)
	if err != nil {
		return scope
	}
	for _, grant := range grants {
		scope.tags[outboundGrantKey(grant.AgentID, grant.OutboundTag)] = struct{}{}
	}
	return scope
}

func (s areaManagerOutboundScope) allows(agentID, outboundTag string) bool {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(outboundTag) == "" {
		return false
	}
	_, ok := s.tags[outboundGrantKey(agentID, outboundTag)]
	return ok
}

func outboundGrantKey(agentID, outboundTag string) string {
	return strings.TrimSpace(agentID) + "\x00" + strings.TrimSpace(outboundTag)
}

func (a *App) areaManagerClientScope(user model.AdminUser) areaManagerClientScope {
	scope := a.areaManagerGrantedClientScope(user)
	if !isAreaManager(user) || user.ID <= 0 || a == nil || a.store == nil {
		return scope
	}
	for agentID := range scope.agents {
		a.addAreaManagerCreatedClients(user, agentID, &scope)
	}
	customers, err := a.store.ListCustomersForOwner(model.AdminRoleAreaManager, user.ID)
	if err != nil {
		return scope
	}
	for _, customer := range customers {
		for _, assignment := range customer.Assignments {
			addAreaManagerScopeAssignment(&scope, assignment.AgentID, assignment.InboundID, assignment.InboundTag, assignment.ClientEmail, assignment.Enabled)
		}
	}
	return scope
}

func (a *App) areaManagerGrantedClientScope(user model.AdminUser) areaManagerClientScope {
	scope := areaManagerClientScope{
		exactClients: make(map[string]struct{}),
		inbounds:     make(map[string]struct{}),
		realmPorts:   make(map[string]struct{}),
		haProxyPorts: make(map[string]struct{}),
		agents:       adminAgentSet(user),
	}
	if !isAreaManager(user) || user.ID <= 0 || a == nil || a.store == nil {
		return scope
	}
	assignments, err := a.store.ListAreaManagerAssignments(user.ID)
	if err == nil {
		configCache := make(map[string]*model.ManagedAgentConfig)
		for _, assignment := range assignments {
			inboundTag := assignment.InboundTag
			if assignment.ClientEmail == "" {
				inboundTag = a.normalizeAreaManagerForwardingAssignmentTag(assignment.AgentID, assignment.InboundID, inboundTag, configCache)
			}
			addAreaManagerScopeAssignment(&scope, assignment.AgentID, assignment.InboundID, inboundTag, assignment.ClientEmail, assignment.Enabled, assignment.NodeOnly)
		}
	}
	return scope
}

func (a *App) normalizeAreaManagerForwardingAssignmentTag(agentID string, listenPort int, inboundTag string, cache map[string]*model.ManagedAgentConfig) string {
	if a == nil || a.store == nil || listenPort <= 0 || (!isRealmAssignmentTag(inboundTag) && !isHAProxyAssignmentTag(inboundTag)) {
		return inboundTag
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return inboundTag
	}
	cfg, cached := cache[agentID]
	if !cached {
		loaded, found, err := a.store.GetAgentConfig(agentID)
		if err != nil || !found {
			cache[agentID] = nil
			return inboundTag
		}
		cfg = &loaded
		cache[agentID] = cfg
	}
	if cfg == nil {
		return inboundTag
	}
	if cfg.Entry.HAProxy.Enabled {
		for _, rule := range cfg.Entry.HAProxy.Rules {
			if rule.Enabled && rule.ListenPort == listenPort {
				return "haproxy:" + strconv.Itoa(listenPort)
			}
		}
	}
	if realmForwardingActive(cfg.Entry.PortForwarding) {
		for _, rule := range cfg.Entry.PortForwarding.Rules {
			if rule.Enabled && rule.ListenPort == listenPort {
				return "realm:" + strconv.Itoa(listenPort)
			}
		}
	}
	return inboundTag
}

func (a *App) areaManagerAssignmentClientScope(user model.AdminUser, agentID string) areaManagerClientScope {
	scope := a.areaManagerGrantedClientScope(user)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || !isAreaManager(user) || user.ID <= 0 || a == nil || a.store == nil {
		return scope
	}
	a.addAreaManagerCreatedClients(user, agentID, &scope)
	return scope
}

func (a *App) addAreaManagerCreatedClients(user model.AdminUser, agentID string, scope *areaManagerClientScope) {
	actions, err := a.store.ListSucceededXUIActionsByActorKind(agentID, user.Role, user.ID, model.XUIActionAddClient)
	if err != nil {
		return
	}
	for _, action := range actions {
		inboundID, inboundTag, email, ok := xuiAddClientTarget(action.Payload)
		if !ok || !scope.allowsManageInbound(agentID, inboundID, inboundTag) {
			continue
		}
		addAreaManagerExactClient(scope, agentID, inboundID, inboundTag, email)
	}
}

func xuiAddClientTarget(payload map[string]any) (int, string, string, bool) {
	if payload == nil {
		return 0, "", "", false
	}
	inboundID := intFromAny(payload["inbound_id"])
	inboundTag := strings.TrimSpace(stringFromAny(payload["inbound_tag"]))
	client, ok := payload["client"].(map[string]any)
	if !ok || (inboundID <= 0 && inboundTag == "") {
		return 0, "", "", false
	}
	email := strings.TrimSpace(stringFromAny(client["email"]))
	return inboundID, inboundTag, email, email != ""
}

func addAreaManagerExactClient(scope *areaManagerClientScope, agentID string, inboundID int, inboundTag, email string) {
	if scope == nil || strings.TrimSpace(agentID) == "" || strings.TrimSpace(email) == "" {
		return
	}
	scope.agents[strings.TrimSpace(agentID)] = struct{}{}
	scope.exactClients[areaClientExactKey(agentID, inboundID, inboundTag, email)] = struct{}{}
	if strings.TrimSpace(inboundTag) != "" {
		scope.exactClients[areaClientExactKey(agentID, inboundID, "", email)] = struct{}{}
	}
}

func addAreaManagerScopeAssignment(scope *areaManagerClientScope, agentID string, inboundID int, inboundTag, clientEmail string, enabled bool, nodeOnly ...bool) {
	if scope == nil || !enabled || agentID == "" {
		return
	}
	scope.agents[agentID] = struct{}{}
	if clientEmail != "" {
		scope.exactClients[areaClientExactKey(agentID, inboundID, inboundTag, clientEmail)] = struct{}{}
		if inboundTag != "" {
			scope.exactClients[areaClientExactKey(agentID, inboundID, "", clientEmail)] = struct{}{}
		}
		removeAreaManagerInboundGrant(scope, agentID, inboundID, inboundTag)
		return
	}
	if isRealmAssignmentTag(inboundTag) {
		scope.realmPorts[areaRealmPortKey(agentID, inboundID)] = struct{}{}
		return
	}
	if isHAProxyAssignmentTag(inboundTag) {
		scope.haProxyPorts[areaForwardingPortKey(agentID, inboundID)] = struct{}{}
		return
	}
	if len(nodeOnly) > 0 && nodeOnly[0] {
		if scope.nodeOnlyInbounds == nil {
			scope.nodeOnlyInbounds = make(map[string]struct{})
		}
		scope.nodeOnlyInbounds[areaClientInboundKey(agentID, inboundID, inboundTag)] = struct{}{}
		scope.nodeOnlyInbounds[areaClientInboundKey(agentID, inboundID, "")] = struct{}{}
		return
	}
	if scope.hasExactClientOnInbound(agentID, inboundID, inboundTag) {
		return
	}
	scope.inbounds[areaClientInboundKey(agentID, inboundID, inboundTag)] = struct{}{}
	if inboundTag != "" {
		scope.inbounds[areaClientInboundKey(agentID, inboundID, "")] = struct{}{}
	}
}

func removeAreaManagerInboundGrant(scope *areaManagerClientScope, agentID string, inboundID int, inboundTag string) {
	if scope == nil {
		return
	}
	delete(scope.inbounds, areaClientInboundKey(agentID, inboundID, inboundTag))
	if inboundTag != "" {
		delete(scope.inbounds, areaClientInboundKey(agentID, inboundID, ""))
		return
	}
	normalizedAgentID := strings.TrimSpace(agentID)
	normalizedInboundID := strconv.Itoa(inboundID)
	for key := range scope.inbounds {
		parts := strings.Split(key, "\x00")
		if len(parts) == 3 && strings.EqualFold(parts[0], normalizedAgentID) && parts[1] == normalizedInboundID {
			delete(scope.inbounds, key)
		}
	}
}

func (s areaManagerClientScope) allowsClient(agentID string, inboundID int, inboundTag, email string) bool {
	if agentID == "" {
		return false
	}
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	if email == "" {
		return s.allowsInbound(agentID, inboundID, inboundTag)
	}
	if _, ok := s.exactClients[areaClientExactKey(agentID, inboundID, inboundTag, email)]; ok {
		return true
	}
	if inboundTag != "" {
		if _, ok := s.exactClients[areaClientExactKey(agentID, inboundID, "", email)]; ok {
			return true
		}
	}
	return s.allowsWholeInbound(agentID, inboundID, inboundTag)
}

func (s areaManagerClientScope) allowsInbound(agentID string, inboundID int, inboundTag string) bool {
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	if _, ok := s.nodeOnlyInbounds[areaClientInboundKey(agentID, inboundID, inboundTag)]; ok {
		return true
	}
	if _, ok := s.nodeOnlyInbounds[areaClientInboundKey(agentID, inboundID, "")]; ok {
		return true
	}
	return s.allowsWholeInbound(agentID, inboundID, inboundTag)
}

func (s areaManagerClientScope) allowsWholeInbound(agentID string, inboundID int, inboundTag string) bool {
	if agentID == "" {
		return false
	}
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	if _, ok := s.inbounds[areaClientInboundKey(agentID, inboundID, inboundTag)]; ok {
		return true
	}
	if inboundTag != "" {
		if _, ok := s.inbounds[areaClientInboundKey(agentID, inboundID, "")]; ok {
			return true
		}
	}
	return false
}

func (s areaManagerClientScope) allowsForwardingPort(agentID string, listenPort int, forwardType string) bool {
	if agentID == "" || listenPort <= 0 {
		return false
	}
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	portKey := areaForwardingPortKey(agentID, listenPort)
	ports := s.realmPorts
	assignmentTagMatch := isRealmAssignmentTag
	if strings.EqualFold(strings.TrimSpace(forwardType), "haproxy") {
		ports = s.haProxyPorts
		assignmentTagMatch = isHAProxyAssignmentTag
	}
	if _, ok := ports[portKey]; ok {
		return true
	}
	for key := range s.inbounds {
		parts := strings.Split(key, "\x00")
		if len(parts) != 3 || !strings.EqualFold(parts[0], strings.TrimSpace(agentID)) || parts[1] != strconv.Itoa(listenPort) {
			continue
		}
		if assignmentTagMatch(parts[2]) {
			return true
		}
	}
	return false
}

func areaClientExactKey(agentID string, inboundID int, inboundTag, email string) string {
	return strings.Join([]string{
		strings.TrimSpace(agentID),
		strconv.Itoa(inboundID),
		strings.ToLower(strings.TrimSpace(inboundTag)),
		strings.ToLower(strings.TrimSpace(email)),
	}, "\x00")
}

func areaClientInboundKey(agentID string, inboundID int, inboundTag string) string {
	return strings.Join([]string{
		strings.TrimSpace(agentID),
		strconv.Itoa(inboundID),
		strings.ToLower(strings.TrimSpace(inboundTag)),
	}, "\x00")
}

func areaRealmPortKey(agentID string, listenPort int) string {
	return areaForwardingPortKey(agentID, listenPort)
}

func areaForwardingPortKey(agentID string, listenPort int) string {
	return strings.Join([]string{strings.TrimSpace(agentID), strconv.Itoa(listenPort)}, "\x00")
}

func isRealmAssignmentTag(tag string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(tag)), "realm:")
}

func isHAProxyAssignmentTag(tag string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(tag)), "haproxy:")
}

func (s areaManagerClientScope) hasExactClientOnInbound(agentID string, inboundID int, inboundTag string) bool {
	if agentID == "" || inboundID <= 0 {
		return false
	}
	if _, ok := s.agents[agentID]; !ok {
		return false
	}
	normalizedAgentID := strings.TrimSpace(agentID)
	normalizedInboundID := strconv.Itoa(inboundID)
	normalizedTag := strings.ToLower(strings.TrimSpace(inboundTag))
	for key := range s.exactClients {
		parts := strings.Split(key, "\x00")
		if len(parts) != 4 || !strings.EqualFold(parts[0], normalizedAgentID) || parts[1] != normalizedInboundID {
			continue
		}
		if parts[2] == normalizedTag || parts[2] == "" || normalizedTag == "" {
			return true
		}
	}
	return false
}

func (s areaManagerClientScope) allowsManageInbound(agentID string, inboundID int, inboundTag string) bool {
	return s.allowsInbound(agentID, inboundID, inboundTag) || s.hasExactClientOnInbound(agentID, inboundID, inboundTag)
}
