package server

import (
	"fmt"
	"net/http"

	"bridge-core/internal/model"
)

// Ownership is explicit and root-only. Checking the recipient's assignment
// permissions before moving the customer avoids deriving authority from the
// customer's own grants in areaManagerClientScope.
func (a *App) customerOwnerForRequest(actor model.AdminUser, customerID int64, req model.CustomerAccountRequest) (string, int64, int, error) {
	ownerType, ownerID := model.AdminRoleRoot, int64(1)
	if isAreaManager(actor) {
		ownerType, ownerID = model.AdminRoleAreaManager, actor.ID
	}
	if req.OwnerAreaManagerID == nil {
		return ownerType, ownerID, 0, nil
	}
	if !isRootAdmin(actor) {
		return "", 0, http.StatusForbidden, fmt.Errorf("只有总管理员可以修改用户所属区域")
	}
	managerID := *req.OwnerAreaManagerID
	if managerID < 0 {
		return "", 0, http.StatusBadRequest, fmt.Errorf("所属区域账号无效")
	}
	if managerID == 0 {
		return model.AdminRoleRoot, 1, 0, nil
	}
	var current model.CustomerAdminView
	if customerID > 0 {
		var found bool
		var err error
		current, found, err = a.store.GetCustomer(customerID)
		if err != nil {
			return "", 0, http.StatusInternalServerError, err
		}
		if !found {
			return "", 0, http.StatusNotFound, fmt.Errorf("customer not found")
		}
		// An unchanged owner remains valid even if that area account is disabled.
		if current.OwnerType == model.AdminRoleAreaManager && current.OwnerID == managerID {
			return current.OwnerType, current.OwnerID, 0, nil
		}
	}
	manager, found, err := a.store.GetAreaManager(managerID)
	if err != nil {
		return "", 0, http.StatusInternalServerError, err
	}
	if !found || !manager.Enabled {
		return "", 0, http.StatusBadRequest, fmt.Errorf("所属区域账号不存在或已停用，请选择启用的区域账号")
	}
	recipient := model.AdminUser{ID: manager.ID, Role: model.AdminRoleAreaManager, AgentIDs: manager.AgentIDs}
	frontProxyIDs := req.FrontProxyNodeIDs
	if frontProxyIDs == nil {
		frontProxyIDs, err = a.store.ListFrontProxyGrantNodeIDs(model.FrontProxyGranteeCustomer, customerID)
		if err != nil {
			return "", 0, http.StatusInternalServerError, err
		}
	}
	assignmentProxyIDs, err := a.store.ListCustomerAssignmentFrontProxyNodeIDs(customerID)
	if err != nil {
		return "", 0, http.StatusInternalServerError, err
	}
	allowedProxyIDs, err := a.store.ListFrontProxyGrantNodeIDs(model.FrontProxyGranteeAreaManager, managerID)
	if err != nil {
		return "", 0, http.StatusInternalServerError, err
	}
	allowedProxies := make(map[int64]bool, len(allowedProxyIDs))
	for _, id := range allowedProxyIDs {
		allowedProxies[id] = true
	}
	for _, id := range append(assignmentProxyIDs, frontProxyIDs...) {
		if !allowedProxies[id] {
			return "", 0, http.StatusBadRequest, fmt.Errorf("用户或链路的前置代理 #%d 超出接收区域的授权范围，请先调整授权", id)
		}
	}
	for _, assignment := range current.Assignments {
		if !a.areaManagerCustomerAssignmentAllowed(recipient, model.CustomerAssignmentRequest{
			AgentID: assignment.AgentID, InboundID: assignment.InboundID,
			InboundTag: assignment.InboundTag, ClientEmail: assignment.ClientEmail,
		}) {
			return "", 0, http.StatusBadRequest, fmt.Errorf("链路 %q（%s / %d）的节点或客户端超出接收区域的授权范围，请先为区域账号授权或调整用户链路", firstNonEmptyString(assignment.PublicClientName, assignment.ClientEmail, assignment.InboundTag), assignment.AgentID, assignment.InboundID)
		}
	}
	return model.AdminRoleAreaManager, managerID, 0, nil
}
