package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
)

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.isRegistrationAuthorized(r.Header.Get("X-Registration-Token")) {
		writeError(w, http.StatusUnauthorized, "invalid registration token")
		return
	}

	var req model.AgentRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode register request: %v", err))
		return
	}
	if !config.IsValidAgentID(req.AgentID) {
		writeError(w, http.StatusBadRequest, "agent_id must be 1-80 characters using only letters, digits, dots, underscores, or hyphens")
		return
	}
	if req.AgentName == "" {
		req.AgentName = req.AgentID
	}
	req.SeedConfig.AgentID = req.AgentID
	if req.SeedConfig.AgentName == "" {
		req.SeedConfig.AgentName = req.AgentName
	}
	// Network identity is server-observed; never trust an address supplied by the agent.
	req.PublicIPv4 = ""
	req.PublicIPv6 = ""
	observedIP := a.requestObservedIP(r)
	if isUsableObservedIP(observedIP) {
		if net.ParseIP(observedIP).To4() != nil {
			req.PublicIPv4 = observedIP
		} else {
			req.PublicIPv6 = observedIP
		}
	}
	a.applyDefaultXUIBootstrap(&req)

	presentedAgentToken := strings.TrimSpace(r.Header.Get("X-Agent-Token"))
	// Pre-0.3.25 clients do not know how to send the issued per-agent token.
	// The shared registration token was already authenticated above; keep this
	// compatibility path version-gated until those clients are upgraded. New
	// clients without an agent token may still create a new identity, but can
	// never refresh an existing one through the bootstrap token.
	result, err := a.store.RegisterAgentWithTokenOrLegacyBootstrap(req, presentedAgentToken, !requiresPerAgentRegistrationToken(req.Version))
	if err != nil {
		if errors.Is(err, store.ErrAgentRegistrationUnauthorized) {
			writeError(w, http.StatusUnauthorized, "existing agent registration requires a valid agent token")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hydrated, hydrateErr := a.hydrateHAProxyTargets(result.Config); hydrateErr == nil {
		result.Config = hydrated
	} else {
		log.Printf("hydrate HAProxy targets for %s during registration failed: %v", req.AgentID, hydrateErr)
	}
	a.clearCustomerOverviewCache()
	go a.refreshTopologyLookupCacheFromRegister(req, observedIP)
	writeJSON(w, http.StatusOK, result)
}

// requiresPerAgentRegistrationToken identifies clients that understand the
// per-agent registration token protocol introduced in 0.3.25. Unknown
// versions remain on the legacy path so older development builds can recover.
func requiresPerAgentRegistrationToken(version string) bool {
	normalized := normalizeVersion(version)
	if _, ok := parseSemver(normalized); !ok {
		return false
	}
	return !isVersionNewer("0.3.25", normalized)
}

func (a *App) applyDefaultXUIBootstrap(req *model.AgentRegisterRequest) {
	// x-ui auto-install is currently disabled globally. Do not seed new clients
	// with auto_install even if old install settings still have it enabled.
	_ = req
}

func (a *App) isRegistrationAuthorized(token string) bool {
	if a.config.RegistrationToken == "" {
		return true
	}
	return token != "" && token == a.config.RegistrationToken
}
