package server

import (
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/model"
	"bridge-core/internal/store"
	"bridge-core/internal/version"
	"bridge-core/webui"
)

type App struct {
	config             config.ServerConfig
	store              *store.SQLiteStore
	realtime           *realtimeHub
	supportPresence    *supportPresenceHub
	alerts             *alertService
	demoDataSource     http.Handler
	exchangeRatesMu    sync.Mutex
	exchangeRatesCache model.ExchangeRatesResponse
	dashboardCacheMu   sync.Mutex
	dashboardCache     map[string]dashboardCacheEntry
	topologyCache      map[string]dashboardCacheEntry
	customerViewCache  map[string]customerOverviewCacheEntry
	topologyBuilds     map[string]chan struct{}
	areaTrafficMu      sync.Mutex
	areaTrafficSamples map[string]areaManagerTrafficSample
	areaRealtimeMu     sync.Mutex
	areaRealtimeCache  areaManagerRealtimeContextCache
	lookupCacheMu      sync.Mutex
	updateLatestMu     sync.Mutex
	updateLatestCache  map[string]updateLatestCacheEntry
	trustedProxies     []netip.Prefix
	loginLimiter       *loginRateLimiter
}

const (
	adminSessionCookieName    = "bridge_core_session"
	adminSessionTTL           = 24 * time.Hour
	customerSessionCookieName = "bridge_core_customer_session"
	customerSessionTTL        = 7 * 24 * time.Hour
	dashboardCacheTTL         = 10 * time.Second
	topologyCacheTTL          = 45 * time.Second
	customerOverviewCacheTTL  = 10 * time.Second
	updateLatestCacheTTL      = 10 * time.Minute
	maxJSONRequestBodyBytes   = 16 << 20
)

const (
	loginRateLimitWindow = 15 * time.Minute
	loginRateLimitMax    = 10
	loginRateLimitBlock  = time.Minute
)

type dashboardCacheEntry struct {
	expiresAt time.Time
	view      model.GlobalDashboardView
}

type customerOverviewCacheEntry struct {
	expiresAt time.Time
	view      model.GlobalDashboardView
	agents    []model.AgentRecord
	snapshots []model.AgentSnapshot
}

type updateLatestCacheEntry struct {
	expiresAt time.Time
	info      *model.UpdateLatestInfo
}

type loginRateLimitEntry struct {
	windowStart  time.Time
	failures     int
	blockedUntil time.Time
}

type loginRateLimiter struct {
	mu      sync.Mutex
	entries map[string]loginRateLimitEntry
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{entries: make(map[string]loginRateLimitEntry)}
}

func (l *loginRateLimiter) allowed(keys ...string) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		entry, ok := l.entries[key]
		if !ok || now.After(entry.blockedUntil) {
			continue
		}
		return false
	}
	return true
}

func (l *loginRateLimiter) failure(keys ...string) {
	if l == nil {
		return
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) >= 10000 {
		for key, entry := range l.entries {
			if now.Sub(entry.windowStart) >= loginRateLimitWindow && now.After(entry.blockedUntil) {
				delete(l.entries, key)
			}
		}
	}
	for _, key := range keys {
		entry := l.entries[key]
		if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= loginRateLimitWindow {
			entry = loginRateLimitEntry{windowStart: now}
		}
		entry.failures++
		if entry.failures >= loginRateLimitMax {
			entry.blockedUntil = now.Add(loginRateLimitBlock)
		}
		l.entries[key] = entry
	}
}

func (l *loginRateLimiter) success(accountKey string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.entries, accountKey)
	l.mu.Unlock()
}

func New(cfg config.ServerConfig) (*App, error) {
	trustedProxies, err := parseTrustedProxyPrefixes(cfg.TrustedProxyCIDRs)
	if err != nil {
		return nil, err
	}
	cipher, err := store.LoadOrCreateCredentialCipher(cfg.CredentialKeyPath)
	if err != nil {
		return nil, err
	}
	fs, err := store.NewSQLiteStore(
		cfg.DatabasePath,
		store.WithCredentialCipher(cipher),
		store.WithSnapshotRetention(store.SnapshotRetentionPolicy{
			MaxAge:      time.Duration(cfg.SnapshotRetentionDays) * 24 * time.Hour,
			MaxPerAgent: cfg.SnapshotRetentionCount,
		}),
	)
	if err != nil {
		return nil, err
	}
	if err := fs.SeedAgents(cfg.Agents); err != nil {
		return nil, err
	}
	adminPassword := cfg.AdminPassword
	if adminPassword == "" {
		adminPassword = cfg.AdminToken
	}
	if err := fs.EnsureAdminAccount(cfg.AdminUsername, adminPassword); err != nil {
		return nil, err
	}
	var demoDataSource http.Handler
	if cfg.DemoDataSourceURL != "" {
		demoDataSource, err = newDemoDataSource(cfg.DemoDataSourceURL)
		if err != nil {
			return nil, err
		}
		log.Printf("demo data source enabled: %s", cfg.DemoDataSourceURL)
	}
	app := &App{
		config:            cfg,
		store:             fs,
		realtime:          newRealtimeHub(),
		supportPresence:   newSupportPresenceHub(),
		alerts:            newAlertService(fs),
		demoDataSource:    demoDataSource,
		dashboardCache:    make(map[string]dashboardCacheEntry),
		topologyCache:     make(map[string]dashboardCacheEntry),
		customerViewCache: make(map[string]customerOverviewCacheEntry),
		topologyBuilds:    make(map[string]chan struct{}),
		updateLatestCache: make(map[string]updateLatestCacheEntry),
		trustedProxies:    trustedProxies,
		loginLimiter:      newLoginRateLimiter(),
	}
	app.alerts.Start()
	return app, nil
}

func parseTrustedProxyPrefixes(values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			addr, addrErr := netip.ParseAddr(value)
			if addrErr != nil {
				return nil, fmt.Errorf("parse trusted proxy %q: %w", value, err)
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealth)
	if a.demoDataSource != nil {
		mux.Handle("/api/", a.demoDataSource)
		mux.Handle("/", webui.NewHandler())
		return secureTransportHeaders(a, limitJSONRequestBodies(mux))
	}
	mux.HandleFunc("/api/v1/admin/", a.handleAdmin)
	mux.HandleFunc("/api/v1/customer/", a.handleCustomer)
	mux.HandleFunc("/api/v1/frontend-settings", a.handlePublicFrontendSettings)
	mux.HandleFunc("/api/v1/image-proxy", a.handleImageProxy)
	mux.HandleFunc("/api/v1/public/topology", a.handlePublicTopology)
	mux.HandleFunc("/api/v1/dashboard/realtime", a.handleDashboardRealtime)
	mux.HandleFunc("/api/v1/dashboard/topology", a.handleDashboardTopology)
	mux.HandleFunc("/api/v1/exchange-rates", a.handleAdminExchangeRates)
	mux.HandleFunc("/api/v1/dashboard", a.handleDashboard)
	mux.HandleFunc("/api/v1/agents", a.handleAgents)
	mux.HandleFunc("/api/v1/agents/register", a.handleRegister)
	mux.HandleFunc("/api/v1/agents/", a.handleAgentByID)
	mux.Handle("/", webui.NewHandler())
	return secureTransportHeaders(a, limitJSONRequestBodies(mux))
}

func secureTransportHeaders(app *App, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if app != nil && app.isSecureRequest(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func limitJSONRequestBodies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	info := serverSystemInfo()
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": info.Version,
	})
}

func serverSystemInfo() model.SystemInfo {
	info := version.Get("server")
	return model.SystemInfo{
		Role:      info.Role,
		Version:   info.Version,
		BuildTime: info.BuildTime,
		GitCommit: info.GitCommit,
		GoVersion: info.GoVersion,
		Platform:  info.Platform,
	}
}
