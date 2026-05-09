package server

import (
	"net/http"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/store"
	"bridge-core/webui"
)

type App struct {
	config   config.ServerConfig
	store    *store.SQLiteStore
	realtime *realtimeHub
	alerts   *alertService
}

const (
	adminSessionCookieName = "bridge_core_session"
	adminSessionTTL        = 24 * time.Hour
)

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
	app := &App{
		config:   cfg,
		store:    fs,
		realtime: newRealtimeHub(),
		alerts:   newAlertService(fs),
	}
	app.alerts.Start()
	return app, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealth)
	mux.HandleFunc("/api/v1/admin/", a.handleAdmin)
	mux.HandleFunc("/api/v1/dashboard/realtime", a.handleDashboardRealtime)
	mux.HandleFunc("/api/v1/dashboard", a.handleDashboard)
	mux.HandleFunc("/api/v1/agents", a.handleAgents)
	mux.HandleFunc("/api/v1/agents/register", a.handleRegister)
	mux.HandleFunc("/api/v1/agents/", a.handleAgentByID)
	mux.Handle("/", webui.NewHandler())
	return mux
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
