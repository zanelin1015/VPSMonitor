package client

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bridge-core/internal/model"
)

// Real nftables validation runs only inside a newly created network namespace.
// It must never install test accounting rules into the host network namespace.
func TestRealmTrafficKernelCounters(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("requires Linux root and an isolated network namespace")
	}
	for _, name := range []string{"unshare", "ip", "nft"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " is unavailable")
		}
	}
	parentNamespace := os.Getenv("VPSMONITOR_REALM_TEST_PARENT_NETNS")
	currentNamespace, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if parentNamespace == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "unshare", "--net", os.Args[0], "-test.run=^TestRealmTrafficKernelCounters$", "-test.v")
		command.Env = append(os.Environ(), "VPSMONITOR_REALM_TEST_PARENT_NETNS="+currentNamespace)
		body, err := command.CombinedOutput()
		if err != nil {
			if strings.Contains(string(body), "unshare:") && strings.Contains(string(body), "Operation not permitted") {
				t.Skip("network namespace creation is not permitted")
			}
			t.Fatalf("isolated kernel counter test: %v\n%s", err, body)
		}
		t.Log(string(body))
		return
	}
	if parentNamespace == currentNamespace {
		t.Fatal("refusing to run nftables test in the parent network namespace")
	}
	if body, err := exec.Command("ip", "link", "set", "lo", "up").CombinedOutput(); err != nil {
		t.Fatalf("enable isolated loopback: %v %s", err, body)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	udpListener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer udpListener.Close()
	go func() {
		body := make([]byte, 32)
		if count, peer, err := udpListener.ReadFromUDP(body); err == nil {
			_, _ = udpListener.WriteToUDP(body[:count], peer)
		}
	}()
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		body := make([]byte, 5)
		if _, err := io.ReadFull(connection, body); err == nil {
			_, _ = connection.Write([]byte("world"))
		}
	}()
	cfg, realm := realmTrafficFixture()
	cfg.Rules[0].ListenAddress, cfg.Rules[0].ListenPort = "127.0.0.1", port
	realm.Rules = cfg.Rules
	marker := filepath.Join(t.TempDir(), "installed")
	collect := func() *model.RealmTrafficSnapshot {
		t.Helper()
		result := collectRealmTrafficWith(context.Background(), cfg, realm, osCommandRunner{}, "linux", false, "isolated-boot", marker)
		if result.Error != "" || len(result.Counters) != 1 {
			t.Fatalf("kernel collection failed: %#v", result)
		}
		return result
	}
	first := collect()
	connection, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	if _, err := connection.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 5)
	if _, err := io.ReadFull(connection, reply); err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	udpConnection, err := net.DialUDP("udp4", nil, udpListener.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer udpConnection.Close()
	_ = udpConnection.SetDeadline(time.Now().Add(time.Second))
	if _, err := udpConnection.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := udpConnection.Read(reply); err != nil {
		t.Fatal(err)
	}
	second := collect()
	if second.Counters[0].UploadBytes < 10 || second.Counters[0].DownloadBytes < 10 || second.Counters[0].Epoch != first.Counters[0].Epoch {
		t.Fatalf("TCP/UDP traffic was not counted/preserved: %#v", second)
	}
	cfg.TrafficEnabled = false
	if result := collectRealmTrafficWith(context.Background(), cfg, realm, osCommandRunner{}, "linux", false, "isolated-boot", marker); result.Error != "" {
		t.Fatal(result.Error)
	}
	state, err := readRealmNFTState(context.Background(), osCommandRunner{})
	if err != nil || state.Exists {
		t.Fatalf("disabled counter table still exists: %#v %v", state, err)
	}
}
