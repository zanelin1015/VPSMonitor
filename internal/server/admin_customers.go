package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) handleAdminCustomers(w http.ResponseWriter, r *http.Request, parts []string) {
	user, _, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			var (
				customers []model.CustomerAdminView
				err       error
			)
			if isRootAdmin(user) {
				customers, err = a.store.ListCustomers()
			} else {
				customers, err = a.store.ListCustomersForOwner(model.AdminRoleAreaManager, user.ID)
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, customers)
		case http.MethodPost:
			var req model.CustomerAccountRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode customer request: %v", err))
				return
			}
			ownerType := model.AdminRoleRoot
			ownerID := int64(1)
			if isAreaManager(user) {
				ownerType = model.AdminRoleAreaManager
				ownerID = user.ID
				if !a.adminCanUseFrontProxyNodes(user, req.FrontProxyNodeIDs) {
					writeError(w, http.StatusForbidden, "front proxy is outside the area manager authorization scope")
					return
				}
			}
			customer, err := a.store.CreateCustomerForOwner(req, ownerType, ownerID)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, customer)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) == 1 && parts[0] == "assignment-sources" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		sources, err := a.customerAssignmentSourcesForAdmin(user)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sources)
		return
	}

	customerID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || customerID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid customer id")
		return
	}
	visible, err := a.customerVisibleToAdmin(user, customerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !visible {
		writeError(w, http.StatusNotFound, "customer not found")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			var req model.CustomerAccountRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode customer request: %v", err))
				return
			}
			if isAreaManager(user) && !a.adminCanUseFrontProxyNodes(user, req.FrontProxyNodeIDs) {
				writeError(w, http.StatusForbidden, "front proxy is outside the area manager authorization scope")
				return
			}
			customer, err := a.store.UpdateCustomer(customerID, req)
			if err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "not found") {
					status = http.StatusNotFound
				}
				writeError(w, status, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, customer)
		case http.MethodDelete:
			if err := a.store.DeleteCustomer(customerID); err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "not found") {
					status = http.StatusNotFound
				}
				writeError(w, status, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "reset-password" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		customer, err := a.store.UpdateCustomer(customerID, model.CustomerAccountRequest{
			Password: model.DefaultAccountPassword,
		})
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, customer)
		return
	}
	if len(parts) == 2 && parts[1] == "subscription" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		assignmentIDs, selectionProvided, err := parseCustomerSubscriptionAssignmentIDs(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if selectionProvided && len(assignmentIDs) == 0 {
			writeError(w, http.StatusBadRequest, "at least one customer subscription assignment is required")
			return
		}
		if selectionProvided {
			assignments, listErr := a.store.ListCustomerAssignments(customerID)
			if listErr != nil {
				writeError(w, http.StatusInternalServerError, listErr.Error())
				return
			}
			allowed := make(map[int64]struct{}, len(assignments))
			for _, assignment := range assignments {
				allowed[assignment.ID] = struct{}{}
			}
			for _, assignmentID := range assignmentIDs {
				if _, ok := allowed[assignmentID]; !ok {
					writeError(w, http.StatusBadRequest, "customer subscription assignment is outside the customer scope")
					return
				}
			}
		}
		token, err := a.store.EnsureCustomerSubscriptionToken(customerID)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, model.CustomerSubscriptionURLResponse{
			ClashSubscriptionURL:  customerSubscriptionURLForAssignments(r, token, "clash.yaml", assignmentIDs),
			MihomoSubscriptionURL: customerSubscriptionURLForAssignments(r, token, "mihomo.yaml", assignmentIDs),
		})
		return
	}

	if parts[1] != "assignments" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			assignments, err := a.store.ListCustomerAssignments(customerID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, assignments)
		case http.MethodPost:
			var req model.CustomerAssignmentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode assignment request: %v", err))
				return
			}
			if !a.adminCanAccessAgent(user, req.AgentID) {
				writeError(w, http.StatusForbidden, "agent is not assigned to this account")
				return
			}
			if isAreaManager(user) && !a.areaManagerCustomerAssignmentAllowed(user, req) {
				writeError(w, http.StatusForbidden, "node or client is outside the area manager authorization scope")
				return
			}
			if !a.adminCanUseFrontProxyNodes(user, req.FrontProxyNodeIDs) {
				writeError(w, http.StatusForbidden, "front proxy is outside this account authorization scope")
				return
			}
			if isAreaManager(user) {
				req.RevenueAmount = nil
				req.RevenueCurrency = ""
				req.RevenueCycle = ""
			}
			assignment, err := a.store.CreateCustomerAssignment(customerID, req)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if isRootAdmin(user) {
				if err := a.syncCustomerAssignmentRevenue(req, user.Username); err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
			}
			writeJSON(w, http.StatusOK, assignment)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	assignmentID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || assignmentID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid assignment id")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req model.CustomerAssignmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode assignment request: %v", err))
			return
		}
		if !a.adminCanAccessAgent(user, req.AgentID) {
			writeError(w, http.StatusForbidden, "agent is not assigned to this account")
			return
		}
		if isAreaManager(user) && !a.areaManagerCustomerAssignmentAllowed(user, req) {
			writeError(w, http.StatusForbidden, "node or client is outside the area manager authorization scope")
			return
		}
		if !a.adminCanUseFrontProxyNodes(user, req.FrontProxyNodeIDs) {
			writeError(w, http.StatusForbidden, "front proxy is outside this account authorization scope")
			return
		}
		if isAreaManager(user) {
			req.RevenueAmount = nil
			req.RevenueCurrency = ""
			req.RevenueCycle = ""
		}
		assignment, err := a.store.UpdateCustomerAssignment(customerID, assignmentID, req)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		if isRootAdmin(user) {
			if err := a.syncCustomerAssignmentRevenue(req, user.Username); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, assignment)
	case http.MethodDelete:
		if err := a.store.DeleteCustomerAssignment(customerID, assignmentID); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) customerAssignmentSourcesForAdmin(user model.AdminUser) ([]model.CustomerAssignmentSourceView, error) {
	view, err := a.dashboardTopologyViewForAdmin(user)
	if err != nil {
		return nil, err
	}
	eligible := make(map[string]struct{}, len(view.Agents))
	if isRootAdmin(user) {
		for _, agent := range view.Agents {
			eligible[agent.AgentID] = struct{}{}
		}
	} else {
		forwardingTargets := customerAssignmentForwardingTargetAgents(view.Links)
		visibleAgents := make(map[string]struct{}, len(view.Agents))
		for _, agent := range view.Agents {
			visibleAgents[agent.AgentID] = struct{}{}
		}
		for _, link := range view.Links {
			if !isForwardingProtocol(link.Source.Protocol) {
				continue
			}
			if customerAssignmentAgentTargetsForwarding(link.Source.AgentID, forwardingTargets) {
				continue
			}
			if _, visible := visibleAgents[link.Source.AgentID]; visible {
				eligible[link.Source.AgentID] = struct{}{}
			}
		}
		assignments, listErr := a.store.ListAreaManagerAssignments(user.ID)
		if listErr != nil {
			return nil, listErr
		}
		configCache := make(map[string]*model.ManagedAgentConfig)
		for _, assignment := range assignments {
			if !assignment.Enabled {
				continue
			}
			inboundTag := a.normalizeAreaManagerForwardingAssignmentTag(assignment.AgentID, assignment.InboundID, assignment.InboundTag, configCache)
			if isRealmAssignmentTag(inboundTag) || isHAProxyAssignmentTag(inboundTag) {
				continue
			}
			if customerAssignmentAgentTargetsForwarding(assignment.AgentID, forwardingTargets) {
				continue
			}
			eligible[assignment.AgentID] = struct{}{}
		}
	}

	result := make([]model.CustomerAssignmentSourceView, 0, len(eligible))
	for _, agent := range view.Agents {
		if _, ok := eligible[agent.AgentID]; !ok {
			continue
		}
		result = append(result, model.CustomerAssignmentSourceView{
			AgentID:   agent.AgentID,
			AgentName: firstNonEmptyString(agent.AgentName, agent.AgentID),
		})
	}
	return result, nil
}

func (a *App) areaManagerCustomerAssignmentAgentTargetsForwarding(user model.AdminUser, agentID string) bool {
	if !isAreaManager(user) {
		return false
	}
	view, err := a.dashboardTopologyViewForAdmin(user)
	return err == nil && customerAssignmentAgentTargetsForwarding(agentID, customerAssignmentForwardingTargetAgents(view.Links))
}

func (a *App) adminCanUseFrontProxyNodes(user model.AdminUser, nodeIDs []int64) bool {
	if len(nodeIDs) == 0 || isRootAdmin(user) {
		return true
	}
	if !isAreaManager(user) {
		return false
	}
	allowed, err := a.store.ListFrontProxyNodesForGrantee(model.FrontProxyGranteeAreaManager, user.ID)
	if err != nil {
		return false
	}
	allowedIDs := make(map[int64]struct{}, len(allowed))
	for _, item := range allowed {
		allowedIDs[item.ID] = struct{}{}
	}
	for _, nodeID := range nodeIDs {
		if _, ok := allowedIDs[nodeID]; !ok {
			return false
		}
	}
	return true
}

func customerAssignmentForwardingTargetAgents(links []model.TopologyLinkView) map[string]struct{} {
	targets := make(map[string]struct{})
	add := func(agentID string) {
		if key := strings.ToLower(strings.TrimSpace(agentID)); key != "" {
			targets[key] = struct{}{}
		}
	}
	for _, link := range links {
		if !isForwardingProtocol(link.Source.Protocol) {
			continue
		}
		add(link.Target.AgentID)
		if link.FinalTarget != nil {
			add(link.FinalTarget.AgentID)
		}
		for _, hop := range link.RealmHops {
			add(hop.AgentID)
		}
	}
	return targets
}

func customerAssignmentAgentTargetsForwarding(agentID string, targets map[string]struct{}) bool {
	_, ok := targets[strings.ToLower(strings.TrimSpace(agentID))]
	return ok
}

func (a *App) syncCustomerAssignmentRevenue(req model.CustomerAssignmentRequest, actor string) error {
	if req.RevenueAmount == nil && req.TrafficMultiplier == nil {
		return nil
	}
	if req.AgentID == "" || req.InboundID <= 0 {
		return nil
	}
	cfg, found, err := a.store.GetAgentConfig(req.AgentID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("agent not found")
	}
	amount := 0.0
	currency := strings.ToUpper(strings.TrimSpace(req.RevenueCurrency))
	if currency != "USDT" {
		currency = "CNY"
	}
	cycle := strings.ToLower(strings.TrimSpace(req.RevenueCycle))
	switch cycle {
	case "quarter", "semiannual", "year":
	case "halfyear", "half-year", "half_year", "half_yearly":
		cycle = "semiannual"
	default:
		cycle = "month"
	}
	if req.RevenueAmount != nil {
		amount = max(*req.RevenueAmount, 0)
	}
	trafficMultiplier := 1.0
	if req.TrafficMultiplier != nil {
		trafficMultiplier = normalizeClientTrafficMultiplier(*req.TrafficMultiplier)
	}
	billing := model.XUIClientBillingConfig{
		InboundID:         req.InboundID,
		InboundTag:        strings.TrimSpace(req.InboundTag),
		Email:             strings.TrimSpace(req.ClientEmail),
		TrafficMultiplier: trafficMultiplier,
		RevenueAmount:     amount,
		RevenueCurrency:   currency,
		RevenueCycle:      cycle,
		ExpireCycle:       cycle,
	}
	key := customerBillingKey(billing.InboundID, billing.InboundTag, billing.Email)
	emailKey := customerBillingEmailKey(billing.InboundID, billing.Email)
	replaced := false
	for index, existing := range cfg.Renewal.ClientBillings {
		if customerBillingKey(existing.InboundID, existing.InboundTag, existing.Email) != key &&
			(emailKey == "" || customerBillingEmailKey(existing.InboundID, existing.Email) != emailKey) {
			continue
		}
		billing.StartTime = existing.StartTime
		billing.ExpireTime = existing.ExpireTime
		billing.ExpireAutoRenew = existing.ExpireAutoRenew
		if req.RevenueAmount == nil {
			billing.RevenueAmount = existing.RevenueAmount
			billing.RevenueCurrency = existing.RevenueCurrency
			billing.RevenueCycle = existing.RevenueCycle
			billing.ExpireCycle = existing.ExpireCycle
		}
		if req.TrafficMultiplier == nil {
			billing.TrafficMultiplier = normalizeClientTrafficMultiplier(existing.TrafficMultiplier)
		}
		cfg.Renewal.ClientBillings[index] = billing
		replaced = true
		break
	}
	if !replaced {
		cfg.Renewal.ClientBillings = append(cfg.Renewal.ClientBillings, billing)
	}
	_, err = a.store.UpdateAgentConfigWithActor(req.AgentID, cfg, actor)
	if err == nil {
		a.clearCustomerOverviewCache()
	}
	return err
}

func normalizeClientTrafficMultiplier(value float64) float64 {
	if value <= 0 {
		return 1
	}
	if value < 0.1 {
		return 0.1
	}
	if value > 100 {
		return 100
	}
	return value
}

func customerBillingKey(inboundID int, inboundTag, email string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", inboundID, strings.ToLower(strings.TrimSpace(inboundTag)), strings.ToLower(strings.TrimSpace(email)))
}

func customerBillingEmailKey(inboundID int, email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ""
	}
	return fmt.Sprintf("%d\x00%s", inboundID, email)
}
