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

func (a *App) syncCustomerAssignmentRevenue(req model.CustomerAssignmentRequest, actor string) error {
	if req.RevenueAmount == nil {
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
	amount := *req.RevenueAmount
	if amount < 0 {
		amount = 0
	}
	currency := strings.ToUpper(strings.TrimSpace(req.RevenueCurrency))
	if currency != "USDT" {
		currency = "CNY"
	}
	cycle := strings.ToLower(strings.TrimSpace(req.RevenueCycle))
	switch cycle {
	case "quarter", "year":
	default:
		cycle = "month"
	}
	billing := model.XUIClientBillingConfig{
		InboundID:       req.InboundID,
		InboundTag:      strings.TrimSpace(req.InboundTag),
		Email:           strings.TrimSpace(req.ClientEmail),
		RevenueAmount:   amount,
		RevenueCurrency: currency,
		RevenueCycle:    cycle,
		ExpireCycle:     "month",
	}
	key := customerBillingKey(billing.InboundID, billing.InboundTag, billing.Email)
	emailKey := customerBillingEmailKey(billing.InboundID, billing.Email)
	replaced := false
	for index, existing := range cfg.Renewal.ClientBillings {
		if customerBillingKey(existing.InboundID, existing.InboundTag, existing.Email) != key &&
			(emailKey == "" || customerBillingEmailKey(existing.InboundID, existing.Email) != emailKey) {
			continue
		}
		billing.ExpireTime = existing.ExpireTime
		if existing.ExpireCycle != "" {
			billing.ExpireCycle = existing.ExpireCycle
		}
		billing.ExpireAutoRenew = existing.ExpireAutoRenew
		cfg.Renewal.ClientBillings[index] = billing
		replaced = true
		break
	}
	if !replaced {
		cfg.Renewal.ClientBillings = append(cfg.Renewal.ClientBillings, billing)
	}
	_, err = a.store.UpdateAgentConfigWithActor(req.AgentID, cfg, actor)
	return err
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
