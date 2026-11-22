package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"bridge-core/internal/model"
)

func networkInterfaceKey(name, mac string) string {
	digest := sha256.Sum256([]byte(name + "\x00" + strings.ToLower(strings.ReplaceAll(mac, "-", ":"))))
	return hex.EncodeToString(digest[:12])
}

// Only one default-route interface is suggested. Bridges, VLANs and their
// parent devices remain selectable individually, without automatic summation.
func finalizeNetworkInterfaces(items []model.NetworkInterfaceCounter, bootID string, preferred []string) []model.NetworkInterfaceCounter {
	if items == nil {
		items = []model.NetworkInterfaceCounter{}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	for i := range items {
		items[i].MAC = strings.ToLower(strings.ReplaceAll(items[i].MAC, "-", ":"))
		items[i].Key = networkInterfaceKey(items[i].Name, items[i].MAC)
		items[i].Epoch = fmt.Sprintf("%s:%d", bootID, items[i].Index)
		items[i].Primary = false
	}
	primary := -1
	for _, name := range preferred {
		for i, item := range items {
			if item.Name == name && item.Up && !item.Loopback {
				primary = i
				break
			}
		}
		if primary >= 0 {
			break
		}
	}
	if primary < 0 {
		for _, physicalOnly := range []bool{true, false} {
			for i, item := range items {
				if item.Up && !item.Loopback && (!physicalOnly || item.Kind == "physical") {
					primary = i
					break
				}
			}
			if primary >= 0 {
				break
			}
		}
	}
	if primary >= 0 {
		items[primary].Primary = true
	}
	return items
}
