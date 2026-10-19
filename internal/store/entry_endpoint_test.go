package store

import (
	"bridge-core/internal/model"
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeEntryPreservesConnectionMapping(t *testing.T) {
	entry := model.AgentEntryConfig{Connection: model.EntryConnectionConfig{Deployment: "docker", DockerNetwork: "bridge", PublicAccess: "nat", PublicAddress: "entry.example.com"}, Mappings: []model.AgentEntryMapping{{UseForExport: true, TargetType: "xui", TargetID: "7", InternalPort: 20001, HostPort: 30001, ExternalPort: 333, Transport: "tcp", DefaultExport: true}}}
	// Address and legacy protocol may be empty: they inherit from the connection.
	normalized := normalizeEntryConfig(entry)
	data, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	var restored model.AgentEntryConfig
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Connection, entry.Connection) || !reflect.DeepEqual(restored.Mappings, entry.Mappings) {
		t.Fatalf("mapping lost during normalization: %s", data)
	}
	if !hasEntryConfig(model.AgentEntryConfig{Connection: entry.Connection}) {
		t.Fatal("connection-only config considered empty")
	}
}
