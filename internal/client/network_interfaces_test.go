package client

import (
	"context"
	"runtime"
	"testing"

	"bridge-core/internal/model"
)

func TestNetworkInterfaceSelectionAndWindowsPayload(t *testing.T) {
	items := finalizeNetworkInterfaces([]model.NetworkInterfaceCounter{
		{Name: "docker0", Index: 4, Up: true, Kind: "bridge"}, {Name: "ens3", Index: 2, Up: true, Kind: "physical", MAC: "AA-BB-CC-DD-EE-FF"}, {Name: "lo", Index: 1, Up: true, Kind: "loopback", Loopback: true},
	}, "boot", nil)
	selected := 0
	for _, item := range items {
		if item.Primary {
			selected++
			if item.Name != "ens3" {
				t.Fatal("fallback selected a virtual/loopback interface")
			}
		}
	}
	if selected != 1 {
		t.Fatal("automatic totals must suggest exactly one interface")
	}
	if networkInterfaceKey("ens3", "AA-BB-CC-DD-EE-FF") != networkInterfaceKey("ens3", "aa:bb:cc:dd:ee:ff") {
		t.Fatal("MAC formatting changed identity")
	}
	payload := []byte(`{"boot_id":"windows-boot","preferred":["Ethernet"],"interfaces":[{"name":"Ethernet","index":3,"mac":"AA-BB-CC-DD-EE-FF","kind":"physical","up":true,"received_bytes":100,"sent_bytes":200},{"name":"vEthernet","index":8,"kind":"virtual","up":true,"received_bytes":100,"sent_bytes":200},{"name":"Unavailable","index":9,"error":"driver unavailable"}]}`)
	result := decodeWindowsNetworkInterfaces(payload)
	if result.Error != "" || len(result.Interfaces) != 2 || len(result.Warnings) != 1 || !result.Interfaces[0].Primary || result.Interfaces[0].SentBytes != 200 {
		t.Fatalf("Windows interface decode: %#v", result)
	}
	for _, bad := range [][]byte{[]byte(`not JSON`), []byte(`{"interfaces":[]}`), []byte(`{"boot_id":"b","interfaces":[{"name":"bad","index":2,"error":"failed"}]}`)} {
		if result := decodeWindowsNetworkInterfaces(bad); result.Error == "" {
			t.Fatal("failed collection must not be reported as zeros")
		}
	}
}

func TestReadNetworkInterfacesOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native procfs is available on Linux only")
	}
	result := collectNetworkInterfaces(context.Background())
	if result.Error != "" || len(result.Interfaces) == 0 {
		t.Fatalf("native discovery failed: %#v", result)
	}
	for _, item := range result.Interfaces {
		if item.Key == "" || item.Epoch == "" || item.Index <= 0 {
			t.Fatalf("invalid native interface identity: %#v", item)
		}
	}
}
