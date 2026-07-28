package client

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type capabilityCommandRunner struct {
	realmPath   string
	haProxyPath string
	runError    error
	commands    []string
}

func (r *capabilityCommandRunner) LookPath(name string) (string, error) {
	path := r.realmPath
	if name == "haproxy" {
		path = r.haProxyPath
	}
	if path == "" {
		return "", errors.New("not found")
	}
	return path, nil
}

func (r *capabilityCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if r.runError != nil {
		return nil, r.runError
	}
	return []byte("realm 2.9.4"), nil
}

func TestDetectAgentCapabilitiesFindsWorkingRealm(t *testing.T) {
	runner := &capabilityCommandRunner{realmPath: "/usr/local/bin/realm"}
	capabilities := detectAgentCapabilitiesForOS("linux", runner)
	if !capabilities.Realm {
		t.Fatal("expected a working Realm binary to be reported")
	}
	if len(runner.commands) != 1 || runner.commands[0] != "/usr/local/bin/realm -v" {
		t.Fatalf("unexpected Realm validation commands: %#v", runner.commands)
	}
}

func TestDetectAgentCapabilitiesRejectsUnsupportedOSAndBrokenRealm(t *testing.T) {
	windowsRunner := &capabilityCommandRunner{realmPath: "C:/realm.exe", haProxyPath: "C:/haproxy.exe"}
	if detectAgentCapabilitiesForOS("windows", windowsRunner).Realm {
		t.Fatal("expected Realm discovery to stay disabled on Windows")
	}
	if len(windowsRunner.commands) != 0 {
		t.Fatalf("expected no Realm command on Windows, got %#v", windowsRunner.commands)
	}

	brokenRunner := &capabilityCommandRunner{realmPath: "/usr/local/bin/realm", runError: errors.New("cannot execute")}
	if detectAgentCapabilitiesForOS("linux", brokenRunner).Realm {
		t.Fatal("expected an invalid Realm binary to be rejected")
	}
	if len(brokenRunner.commands) != 2 {
		t.Fatalf("expected both version flags to be attempted, got %#v", brokenRunner.commands)
	}
}

func TestDetectAgentCapabilitiesFindsWorkingHAProxy(t *testing.T) {
	runner := &capabilityCommandRunner{haProxyPath: "/usr/sbin/haproxy"}
	capabilities := detectAgentCapabilitiesForOS("linux", runner)
	if !capabilities.HAProxy {
		t.Fatal("expected a working HAProxy binary to be reported")
	}
	if len(runner.commands) != 1 || runner.commands[0] != "/usr/sbin/haproxy -v" {
		t.Fatalf("unexpected HAProxy validation commands: %#v", runner.commands)
	}
}
