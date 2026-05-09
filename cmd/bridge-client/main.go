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

	if !*runOnce && runWindowsServiceIfNeeded(*configPath) {
		return
	}

	if err := runClient(context.Background(), *configPath, *runOnce); err != nil {
		log.Fatal(err)
	}
}

func runClient(ctx context.Context, configPath string, runOnce bool) error {
	if runOnce {
		cfg, _, app, err := loadClientApp(configPath)
		if err != nil {
			return err
		}
		if err := app.RunOnce(ctx); err != nil {
			return err
		}
		log.Printf("snapshot pushed for agent %s", cfg.AgentID)
		return nil
	}

	for ctx.Err() == nil {
		cfg, interval, app, err := loadClientApp(configPath)
		if err != nil {
			log.Printf("init client failed: %v; retry in 10s", err)
			if !sleepContext(ctx, 10*time.Second) {
				return ctx.Err()
			}
			continue
		}

		go app.RunRealtimeMetrics(ctx, 2*time.Second)
		ticker := time.NewTicker(interval)
		for ctx.Err() == nil {
			if err := app.RunOnce(ctx); err != nil {
				log.Printf("push snapshot failed: %v", err)
			} else {
				log.Printf("snapshot pushed for agent %s", cfg.AgentID)
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				ticker.Stop()
				return ctx.Err()
			}
		}
		ticker.Stop()
	}
	return ctx.Err()
}

func loadClientApp(configPath string) (config.ClientConfig, time.Duration, *client.App, error) {
	cfg, interval, err := config.LoadClientConfig(configPath)
	if err != nil {
		return cfg, 0, nil, err
	}
	app, err := client.New(cfg)
	if err != nil {
		return cfg, 0, nil, err
	}
	return cfg, interval, app, nil
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
