//go:build !windows

package client

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bridge-core/internal/model"
)

func TestDiscoverNetworkInterfacesAndDefaultRouteWithoutSummingVirtualDevices(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(proc, "sys/kernel/random/boot_id"), "boot-a")
	write(filepath.Join(proc, "net/dev"), "Inter-| Receive | Transmit\n face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n"+
		" lo: 100 0 0 0 0 0 0 0 100 0 0 0 0 0 0 0\n ens3: 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n docker0: 300 0 0 0 0 0 0 0 400 0 0 0 0 0 0 0\n pppoe-wan: 500 0 0 0 0 0 0 0 600 0 0 0 0 0 0 0\n veth-new: 9 0 0 0 0 0 0 0 10 0 0 0 0 0 0 0\n")
	write(filepath.Join(proc, "net/route"), "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\nens3 00000000 01020304 0003 0 0 100 00000000 0 0 0\npppoe-wan 00000000 01020304 0003 0 0 10 00000000 0 0 0\n")
	if err := os.MkdirAll(filepath.Join(sys, "class/net/ens3/device"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sys, "class/net/docker0/bridge"), 0755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(sys, "class/net/veth-new/ifindex"), "9")
	write(filepath.Join(sys, "class/net/veth-new/address"), "02:00:00:00:00:09")
	write(filepath.Join(sys, "class/net/veth-new/flags"), "0x1")
	interfaces := []net.Interface{{Name: "lo", Index: 1, Flags: net.FlagUp | net.FlagLoopback}, {Name: "ens3", Index: 2, Flags: net.FlagUp}, {Name: "docker0", Index: 3, Flags: net.FlagUp}, {Name: "pppoe-wan", Index: 4, Flags: net.FlagUp}}
	first := collectProcNetworkInterfaces(proc, sys, interfaces)
	if first.Error != "" || len(first.Interfaces) != 5 {
		t.Fatalf("discovery: %#v", first)
	}
	byName := map[string]model.NetworkInterfaceCounter{}
	for _, item := range first.Interfaces {
		byName[item.Name] = item
	}
	if !byName["pppoe-wan"].Primary || byName["ens3"].Primary || byName["ens3"].Kind != "physical" || byName["docker0"].Kind != "bridge" || !byName["lo"].Loopback || byName["veth-new"].Index != 9 {
		t.Fatalf("metadata and default route: %#v", byName)
	}
	if byName["ens3"].ReceivedBytes != 0 || byName["ens3"].SentBytes != 0 {
		t.Fatal("zero-counter interfaces must still be discovered")
	}
	write(filepath.Join(proc, "sys/kernel/random/boot_id"), "boot-b")
	second := collectProcNetworkInterfaces(proc, sys, interfaces)
	for i, item := range second.Interfaces {
		if item.Key != first.Interfaces[i].Key || item.Epoch == first.Interfaces[i].Epoch {
			t.Fatal("reboot must preserve interface identity and change counter generation")
		}
	}
	write(filepath.Join(sys, "class/net/pppoe-wan/operstate"), "down")
	third := collectProcNetworkInterfaces(proc, sys, interfaces)
	for _, item := range third.Interfaces {
		if item.Primary && item.Name != "ens3" {
			t.Fatal("down default interface should fall back to an active device")
		}
	}
	// New and removed interfaces are discovered from every procfs sample.
	write(filepath.Join(proc, "net/dev"), "ens3: 20 0 0 0 0 0 0 0 30 0 0 0 0 0 0 0\n")
	fourth := collectProcNetworkInterfaces(proc, sys, interfaces)
	if len(fourth.Interfaces) != 1 || fourth.Interfaces[0].Name != "ens3" {
		t.Fatalf("removed interfaces retained in discovery: %#v", fourth)
	}
}

func TestNetworkInterfaceParserRoutesAndInvalidSamples(t *testing.T) {
	for _, body := range []string{"eth0: 1 2 3", "eth0: bad 0 0 0 0 0 0 0 20 0 0 0 0 0 0 0"} {
		if _, err := parseNetworkInterfaceCounters(strings.NewReader(body)); err == nil {
			t.Fatal("invalid counter must not become a zero sample")
		}
	}
	large, err := parseNetworkInterfaceCounters(strings.NewReader("eth0: 9007199254740993 0 0 0 0 0 0 0 9007199254740995 0 0 0 0 0 0 0"))
	if err != nil || large["eth0"].rx != 9007199254740993 || large["eth0"].tx != 9007199254740995 {
		t.Fatal("counter precision lost", large, err)
	}
	routes := "00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000064 00000000 00000000 00000003 eth6\n00000000000000000000000000000000 00 00000000000000000000000000000000 00 00000000000000000000000000000000 ffffffff 00000000 00000000 00200200 lo\n"
	if names := defaultIPv6Interfaces(routes); !reflect.DeepEqual(names, []string{"eth6"}) {
		t.Fatalf("IPv6 default routes: %#v", names)
	}
	root := t.TempDir()
	result := collectProcNetworkInterfaces(root, root, nil)
	if result.Error == "" || len(result.Interfaces) != 0 {
		t.Fatalf("missing procfs must report failure: %#v", result)
	}
}
