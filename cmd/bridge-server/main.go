package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

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
	if err := http.ListenAndServe(cfg.ListenAddr, app.Handler()); err != nil {
		log.Fatalf("bridge server stopped: %v", err)
	}
}
