package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

type customerEntranceSelection struct {
	XrayAgentID string   `json:"agent_id"`
	InboundID   int      `json:"inbound_id"`
	ClientID    string   `json:"client_id"`
	ClientEmail string   `json:"client_email"`
	Entries     []string `json:"entries"`
	Preview     bool     `json:"preview"`
}

type customerEntranceOption struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	RouteType string `json:"route_type,omitempty"`
	Granted   bool   `json:"granted"`
	request   model.CustomerAssignmentRequest
}

func (a *App) handleCustomerEntrances(w http.ResponseWriter, r *http.Request, customerID int64) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req customerEntranceSelection
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || (!req.Preview && len(req.Entries) == 0) {
		writeError(w, http.StatusBadRequest, "invalid entrance selection")
		return
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	assignments, err := a.store.ListCustomerAssignments(customerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	options, err := customerEntranceOptions(req, agents, buildForwardedOverviewContext(agents, a.store.ListLatest()), assignments)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Preview {
		writeJSON(w, http.StatusOK, options)
		return
	}
	byKey := make(map[string]customerEntranceOption, len(options))
	for _, option := range options {
		byKey[option.Key] = option
	}
	grants := make([]model.CustomerAssignmentRequest, 0, len(req.Entries))
	for _, key := range req.Entries {
		option, ok := byKey[key]
		if !ok {
			writeError(w, http.StatusConflict, "an entrance changed or no longer reaches the selected Xray client")
			return
		}
		if !option.Granted {
			grants = append(grants, option.request)
		}
		byKey[key] = customerEntranceOption{Granted: true}
	}
	created, err := a.store.CreateCustomerEntranceAssignments(customerID, grants)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"created": created})
}

func customerEntranceOptions(req customerEntranceSelection, records []model.AgentRecord, context forwardedOverviewContext, assignments []model.CustomerAssignment) ([]customerEntranceOption, error) {
	xray := context.targetOverviewByAgent[req.XrayAgentID]
	if xray == nil || req.InboundID <= 0 || (req.ClientID == "" && req.ClientEmail == "") {
		return nil, fmt.Errorf("请选择已上报的 Xray 入站和客户端")
	}
	var target model.XUIClientView
	for _, client := range xray.Clients {
		if client.InboundID != req.InboundID {
			continue
		}
		if (req.ClientID != "" && client.ClientID == req.ClientID) || (req.ClientID == "" && strings.EqualFold(req.ClientEmail, client.Email)) {
			if target.InboundID > 0 {
				return nil, fmt.Errorf("Xray 客户端无法唯一识别")
			}
			target = client
		}
	}
	if target.InboundID == 0 {
		return nil, fmt.Errorf("所选 Xray 客户端不存在")
	}
	sourceURL := firstNonEmptyString(target.ImportURL, context.templateURLs[req.XrayAgentID][customerAssignmentKey(req.XrayAgentID, target.InboundID, target.Email)])
	options := make([]customerEntranceOption, 0)
	for _, record := range records {
		agent := context.agentMap[record.AgentID]
		ports := make(map[int]bool)
		for _, rule := range agent.Entry.PortForwarding.Rules {
			ports[rule.ListenPort] = true
		}
		for _, rule := range agent.Entry.HAProxy.Rules {
			ports[rule.ListenPort] = true
		}
		for port := range ports {
			rule := findCustomerEntranceRule(agent.Entry, port)
			configured := findCustomerEntranceRule(record.Config.Entry, port)
			if !rule.enabled || (configured.kind != "" && !configured.enabled) || internalCustomerEntrance(agent, rule.publication) {
				continue
			}
			resolution := rule.resolve(agent, context)
			if !resolution.Resolved || resolution.FinalAgentID != req.XrayAgentID || resolution.FinalNode.ID != req.InboundID {
				continue
			}
			endpoint := rule.publicEntry(agent)
			if endpoint.Error != "" || rewriteCustomerImportURL(sourceURL, endpoint.Host, endpoint.Port) == "" {
				continue
			}
			tag := realmForwardedInboundTag(rule.realm, target)
			if rule.kind == "haproxy" {
				tag = haProxyForwardedInboundTag(rule.haproxy, target)
			}
			name := firstNonEmptyString(rule.publication.Name, agent.CustomerDisplayName, agent.AgentName, agent.AgentID)
			request := model.CustomerAssignmentRequest{AgentID: agent.AgentID, InboundID: port, InboundTag: tag, ClientID: target.ClientID,
				ClientEmail: target.Email, PublicClientName: name, PriceMode: "inherit"}
			option := customerEntranceOption{Key: fmt.Sprintf("%s:%s:%s:%d", agent.AgentID, rule.kind, rule.id, port), Name: name,
				Endpoint: net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port)), RouteType: resolution.routeType(rule.publication.Type), request: request}
			for _, grant := range assignments {
				if grant.AgentID == agent.AgentID && grant.InboundID == port &&
					((target.ClientID != "" && grant.ClientID == target.ClientID) || (target.Email != "" && strings.EqualFold(target.Email, grant.ClientEmail))) {
					option.Granted = true
					break
				}
			}
			options = append(options, option)
		}
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Name == options[j].Name {
			return options[i].Key < options[j].Key
		}
		return options[i].Name < options[j].Name
	})
	return options, nil
}
