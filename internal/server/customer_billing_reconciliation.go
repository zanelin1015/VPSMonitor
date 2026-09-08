package server

import (
	"fmt"
	"strings"

	"bridge-core/internal/dashboard"
	"bridge-core/internal/model"
)

// customerBillingReconciliationForAdmin compares persisted customer bindings
// with the latest x-ui snapshot. It deliberately keeps legacy email matching
// as a diagnostic state so existing installations can be repaired gradually.
func (a *App) customerBillingReconciliationForAdmin(user model.AdminUser) (model.CustomerBillingReconciliationResponse, error) {
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
		return model.CustomerBillingReconciliationResponse{}, err
	}
	agents, err := a.store.ListAgents()
	if err != nil {
		return model.CustomerBillingReconciliationResponse{}, err
	}
	snapshots := a.store.ListLatest()
	entryByAgent := make(map[string]model.AgentEntryConfig, len(agents))
	for _, agent := range agents {
		entryByAgent[agent.AgentID] = agent.Config.Entry
	}
	clientsByAgent := make(map[string][]model.XUIClientView, len(snapshots))
	for _, snapshot := range snapshots {
		overview := dashboard.BuildXUIOverviewWithOptions(snapshot, dashboard.XUIOverviewOptions{Entry: entryByAgent[snapshot.AgentID]})
		if overview != nil {
			clientsByAgent[snapshot.AgentID] = overview.Clients
		}
	}

	response := model.CustomerBillingReconciliationResponse{Items: make([]model.CustomerBillingDiagnostic, 0)}
	for _, customer := range customers {
		for _, assignment := range customer.Assignments {
			item := model.CustomerBillingDiagnostic{
				AssignmentID:    assignment.ID,
				CustomerID:      customer.ID,
				CustomerName:    firstNonEmptyString(customer.DisplayName, customer.Username),
				AgentID:         assignment.AgentID,
				InboundID:       assignment.InboundID,
				InboundTag:      assignment.InboundTag,
				ClientID:        assignment.ClientID,
				ClientEmail:     assignment.ClientEmail,
				PriceMode:       assignment.PriceMode,
				RevenueAmount:   assignment.RevenueAmount,
				RevenueCurrency: assignment.RevenueCurrency,
				RevenueCycle:    assignment.RevenueCycle,
			}
			client, status, message := reconcileCustomerAssignment(assignment, clientsByAgent[assignment.AgentID])
			item.Status = status
			item.Message = message
			if client != nil {
				item.CurrentClientID = client.ClientID
				item.CurrentClientEmail = client.Email
				item.CurrentClientName = firstNonEmptyString(client.Comment, client.Email, client.ClientID)
				item.CanRebind = client.ClientID != "" && status != "matched"
			}
			response.Items = append(response.Items, item)
			switch status {
			case "matched":
				response.Matched++
			case "legacy":
				response.Legacy++
			case "mismatch":
				response.Mismatch++
			default:
				response.Missing++
			}
		}
	}
	return response, nil
}

func reconcileCustomerAssignment(assignment model.CustomerAssignment, clients []model.XUIClientView) (*model.XUIClientView, string, string) {
	if assignment.ClientID != "" {
		for index := range clients {
			if clients[index].ClientID == assignment.ClientID {
				return &clients[index], "matched", "稳定 ID 已匹配"
			}
		}
	}
	for index := range clients {
		client := &clients[index]
		if client.InboundID != assignment.InboundID {
			continue
		}
		if assignment.ClientEmail != "" && strings.EqualFold(strings.TrimSpace(client.Email), strings.TrimSpace(assignment.ClientEmail)) {
			if assignment.ClientID == "" {
				return client, "legacy", "按旧版邮箱匹配，可升级为稳定 ID"
			}
			return client, "mismatch", "邮箱仍匹配，但稳定 ID 已变化"
		}
	}
	for index := range clients {
		client := &clients[index]
		if client.InboundID != assignment.InboundID {
			continue
		}
		if sameBindingLabel(client.Comment, assignment.ClientEmail, assignment.PublicClientName) || sameBindingLabel(client.Email, assignment.ClientEmail, assignment.PublicClientName) {
			return client, "mismatch", "疑似把客户端备注当成了客户端标识"
		}
	}
	if assignment.ClientEmail == "" {
		for index := range clients {
			if clients[index].InboundID == assignment.InboundID {
				return &clients[index], "legacy", "节点级旧数据，可绑定到具体客户端"
			}
		}
	}
	return nil, "missing", "最新 x-ui 上报中未找到该客户端"
}

func sameBindingLabel(value string, labels ...string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, label := range labels {
		if label != "" && strings.EqualFold(value, strings.TrimSpace(label)) {
			return true
		}
	}
	return false
}

func (a *App) currentXUIClient(agentID, clientID string) (model.XUIClientView, bool, error) {
	clientID = strings.TrimSpace(clientID)
	if agentID == "" || clientID == "" {
		return model.XUIClientView{}, false, fmt.Errorf("agent_id and client_id are required")
	}
	agent, found, err := a.store.GetAgent(agentID)
	if err != nil {
		return model.XUIClientView{}, false, err
	}
	if !found {
		return model.XUIClientView{}, false, fmt.Errorf("agent not found")
	}
	snapshot, ok := a.store.GetLatest(agentID)
	if !ok {
		return model.XUIClientView{}, false, nil
	}
	overview := dashboard.BuildXUIOverviewWithOptions(snapshot, dashboard.XUIOverviewOptions{Entry: agent.Config.Entry})
	if overview == nil {
		return model.XUIClientView{}, false, nil
	}
	for _, client := range overview.Clients {
		if client.ClientID == clientID {
			return client, true, nil
		}
	}
	return model.XUIClientView{}, false, nil
}
