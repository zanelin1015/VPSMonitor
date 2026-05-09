//go:build windows

package main

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "VPSMonitorClient"

type windowsClientService struct {
	configPath string
}

func runWindowsServiceIfNeeded(configPath string) bool {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false
	}
	if err := svc.Run(windowsServiceName, windowsClientService{configPath: configPath}); err != nil {
		log.Printf("windows service stopped with error: %v", err)
	}
	return true
}

func (s windowsClientService) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	closeLog := setupWindowsServiceLogging(s.configPath)
	defer closeLog()
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runClient(ctx, s.configPath, false)
	}()

	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				changes <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
				}
				return false, 0
			default:
				// Pause/continue are not supported by this client service.
			}
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("client service exited: %v", err)
				return false, 1
			}
			return false, 0
		}
	}
}

func setupWindowsServiceLogging(configPath string) func() {
	logPath := filepath.Join(filepath.Dir(filepath.Dir(configPath)), "vpsmonitor-client.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return func() {}
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return func() {}
	}
	previous := log.Writer()
	log.SetOutput(io.MultiWriter(previous, file))
	log.Printf("windows service logging to %s", logPath)
	return func() {
		log.SetOutput(previous)
		_ = file.Close()
	}
}
