package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"bridge-core/internal/model"
)

func (a *App) handleAdminOutboundLinks(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			if _, _, ok := a.requireAdmin(w, r); !ok {
				return
			}
			items, err := a.store.ListOutboundLinkLibraryItems()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			if _, _, ok := a.requireRootAdmin(w, r); !ok {
				return
			}
			var req model.OutboundLinkLibraryItem
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("decode outbound link: %v", err))
				return
			}
			if err := validateOutboundLinkLibraryItem(req); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			item, err := a.store.SaveOutboundLinkLibraryItem(req)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, item)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	id := strings.TrimSpace(parts[0])
	switch r.Method {
	case http.MethodPut:
		if _, _, ok := a.requireRootAdmin(w, r); !ok {
			return
		}
		var req model.OutboundLinkLibraryItem
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode outbound link: %v", err))
			return
		}
		req.ID = id
		if err := validateOutboundLinkLibraryItem(req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		item, err := a.store.SaveOutboundLinkLibraryItem(req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if _, _, ok := a.requireRootAdmin(w, r); !ok {
			return
		}
		if err := a.store.DeleteOutboundLinkLibraryItem(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func validateOutboundLinkLibraryItem(item model.OutboundLinkLibraryItem) error {
	if item.Outbound == nil {
		return fmt.Errorf("outbound config is required")
	}
	tag := strings.TrimSpace(item.Tag)
	if tag == "" {
		if outboundTag, _ := item.Outbound["tag"].(string); outboundTag != "" {
			tag = strings.TrimSpace(outboundTag)
		}
	}
	if tag == "" {
		return fmt.Errorf("outbound tag is required")
	}
	protocol := strings.ToLower(strings.TrimSpace(item.Protocol))
	if protocol == "" {
		if outboundProtocol, _ := item.Outbound["protocol"].(string); outboundProtocol != "" {
			protocol = strings.ToLower(strings.TrimSpace(outboundProtocol))
		}
	}
	switch protocol {
	case "vless", "vmess", "socks", "http", "shadowsocks", "freedom", "blackhole":
	default:
		return fmt.Errorf("unsupported outbound protocol: %s", protocol)
	}
	return nil
}
