package server

import (
	"testing"

	"bridge-core/internal/model"
)

func TestSupportPresenceScopesAreaManagersAndRootAdmin(t *testing.T) {
	hub := newSupportPresenceHub()
	rootCustomer := model.CustomerUser{OwnerType: model.AdminRoleRoot, OwnerID: 1}
	areaOneCustomer := model.CustomerUser{OwnerType: model.AdminRoleAreaManager, OwnerID: 11}
	areaTwoCustomer := model.CustomerUser{OwnerType: model.AdminRoleAreaManager, OwnerID: 22}

	disconnectArea := hub.connect(model.AdminUser{ID: 11, Role: model.AdminRoleAreaManager})
	if hub.onlineForCustomer(rootCustomer) {
		t.Fatal("area manager must not cover a root-owned customer")
	}
	if !hub.onlineForCustomer(areaOneCustomer) || hub.onlineForCustomer(areaTwoCustomer) {
		t.Fatal("area manager presence did not stay within its owner scope")
	}

	disconnectRoot := hub.connect(model.AdminUser{ID: 1, Role: model.AdminRoleRoot})
	if !hub.onlineForCustomer(rootCustomer) || !hub.onlineForCustomer(areaTwoCustomer) {
		t.Fatal("root admin should cover every customer")
	}
	disconnectRoot()
	if hub.onlineForCustomer(rootCustomer) || hub.onlineForCustomer(areaTwoCustomer) {
		t.Fatal("root admin disconnect should clear root coverage")
	}
	disconnectArea()
	if hub.onlineForCustomer(areaOneCustomer) {
		t.Fatal("area manager disconnect should clear area coverage")
	}
}
