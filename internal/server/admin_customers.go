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
	if _, _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			customers, err := a.store.ListCustomers()
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
			customer, err := a.store.CreateCustomer(req)
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
			assignment, err := a.store.CreateCustomerAssignment(customerID, req)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
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
		assignment, err := a.store.UpdateCustomerAssignment(customerID, assignmentID, req)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
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
