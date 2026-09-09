package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"bridge-core/internal/config"
	"bridge-core/internal/server"
	"bridge-core/internal/version"
)

func main() {
	configPath := flag.String("config", "./config/server.json", "path to server config")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String("server"))
		return
	}

	cfg, err := config.LoadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("load server config: %v", err)
	}

	app, err := server.New(cfg)
	if err != nil {
		log.Fatalf("init server app: %v", err)
	}

	log.Printf("bridge server listening on %s", cfg.ListenAddr)
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	var serveErr error
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		log.Printf("bridge server TLS enabled with certificate %s", cfg.TLSCertFile)
		serveErr = httpServer.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	} else {
		if cfg.AllowInsecureHTTP {
			log.Printf("WARNING: bridge server is serving HTTP only because allow_insecure_http is enabled; use this only for local development")
		} else {
			log.Printf("bridge server expects TLS to be terminated by a configured trusted reverse proxy")
		}
		serveErr = httpServer.ListenAndServe()
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		log.Fatalf("bridge server stopped: %v", serveErr)
	}
}
