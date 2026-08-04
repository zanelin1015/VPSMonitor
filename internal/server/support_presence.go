package server

import (
	"fmt"
	"sync"

	"bridge-core/internal/model"
)

type supportPresenceHub struct {
	mu          sync.RWMutex
	connections map[string]int
}

func newSupportPresenceHub() *supportPresenceHub {
	return &supportPresenceHub{connections: make(map[string]int)}
}

func (h *supportPresenceHub) connect(user model.AdminUser) func() {
	if h == nil {
		return func() {}
	}
	key := supportPresenceKey(user)
	h.mu.Lock()
	h.connections[key]++
	h.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			if h.connections[key] <= 1 {
				delete(h.connections, key)
			} else {
				h.connections[key]--
			}
			h.mu.Unlock()
		})
	}
}

func (h *supportPresenceHub) onlineForCustomer(customer model.CustomerUser) bool {
	if h == nil {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.connections[supportPresenceKey(model.AdminUser{ID: 1, Role: model.AdminRoleRoot})] > 0 {
		return true
	}
	if customer.OwnerType == model.AdminRoleAreaManager && customer.OwnerID > 0 {
		return h.connections[supportPresenceKey(model.AdminUser{ID: customer.OwnerID, Role: model.AdminRoleAreaManager})] > 0
	}
	return false
}

func supportPresenceKey(user model.AdminUser) string {
	role := user.Role
	if role == "" {
		role = model.AdminRoleRoot
	}
	id := user.ID
	if role == model.AdminRoleRoot {
		id = 1
	}
	return fmt.Sprintf("%s:%d", role, id)
}
