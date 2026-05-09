package main

import (
	"context"
	"flag"
	"log"
	"time"

	"bridge-core/internal/client"
	"bridge-core/internal/config"
)

func main() {
	configPath := flag.String("config", "./config/client.json", "path to client config")
	runOnce := flag.Bool("once", false, "collect and push one snapshot, then exit")
	flag.Parse()

	cfg, interval, err := config.LoadClientConfig(*configPath)
	if err != nil {
		log.Fatalf("load client config: %v", err)
	}

	app, err := client.New(cfg)
	if err != nil {
		log.Fatalf("init client app: %v", err)
	}

	if *runOnce {
		if err := app.RunOnce(context.Background()); err != nil {
			log.Fatalf("run client once: %v", err)
		}
		log.Printf("snapshot pushed for agent %s", cfg.AgentID)
		return
	}

	go app.RunRealtimeMetrics(context.Background(), 2*time.Second)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := app.RunOnce(context.Background()); err != nil {
			log.Printf("push snapshot failed: %v", err)
		} else {
			log.Printf("snapshot pushed for agent %s", cfg.AgentID)
		}
		<-ticker.C
	}
}
