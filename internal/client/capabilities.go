package client

import (
	"context"
	"runtime"
	"time"

	"bridge-core/internal/model"
)

func detectAgentCapabilities(runner commandRunner) model.AgentCapabilities {
	return detectAgentCapabilitiesForOS(runtime.GOOS, runner)
}

func detectAgentCapabilitiesForOS(osName string, runner commandRunner) model.AgentCapabilities {
	return model.AgentCapabilities{
		Realm: realmCapabilityAvailable(osName, runner),
	}
}

func realmCapabilityAvailable(osName string, runner commandRunner) bool {
	if osName != "linux" {
		return false
	}
	path, err := resolveRealmBinary("", runner)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := runner.Run(ctx, path, "-v"); err == nil {
		return true
	}
	_, err = runner.Run(ctx, path, "--version")
	return err == nil
}
