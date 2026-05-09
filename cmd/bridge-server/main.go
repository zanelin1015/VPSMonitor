package main

import (
	"flag"
	"log"
	"net/http"

	"bridge-core/internal/config"
	"bridge-core/internal/server"
)

func main() {
	configPath := flag.String("config", "./config/server.json", "path to server config")
	flag.Parse()

	cfg, err := config.LoadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("load server config: %v", err)
	}

	app, err := server.New(cfg)
	if err != nil {
		log.Fatalf("init server app: %v", err)
	}

	log.Printf("bridge server listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, app.Handler()); err != nil {
		log.Fatalf("bridge server stopped: %v", err)
	}
}
