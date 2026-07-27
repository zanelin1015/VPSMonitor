package server

import (
	"log"
	"net/http"
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
	lookupCacheMu      sync.Mutex
	updateLatestMu     sync.Mutex
	updateLatestCache  map[string]updateLatestCacheEntry
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

func New(cfg config.ServerConfig) (*App, error) {
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
		alerts:            newAlertService(fs),
		demoDataSource:    demoDataSource,
		dashboardCache:    make(map[string]dashboardCacheEntry),
		topologyCache:     make(map[string]dashboardCacheEntry),
		customerViewCache: make(map[string]customerOverviewCacheEntry),
		topologyBuilds:    make(map[string]chan struct{}),
		updateLatestCache: make(map[string]updateLatestCacheEntry),
	}
	app.alerts.Start()
	return app, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealth)
	if a.demoDataSource != nil {
		mux.Handle("/api/", a.demoDataSource)
		mux.Handle("/", webui.NewHandler())
		return mux
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
	return mux
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
