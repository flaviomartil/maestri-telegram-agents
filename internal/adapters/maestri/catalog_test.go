package maestri

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func TestFullGuideCatalog(t *testing.T) {
	dir := os.Getenv("MAESTRI_GUIDE_TEST_DIR")
	if dir == "" {
		t.Skip("set MAESTRI_GUIDE_TEST_DIR to the catalog-sync revision directory")
	}
	c, err := LoadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries := c.List()
	partituras := 0
	types := map[string]bool{}
	for _, a := range entries {
		for _, n := range a.Nodes {
			if n.Kind == "terminal" {
				types[n.AgentType] = true
			}
		}
	}
	presets := []domain.AgentPreset{}
	for kind := range types {
		presets = append(presets, domain.AgentPreset{ID: kind, AgentType: kind, Command: "codex"})
	}
	for _, a := range entries {
		if strings.HasSuffix(a.Source, ".maestripartitura") {
			partituras++
		}
		native, e := NativePartitura(a, presets)
		if e != nil {
			t.Fatalf("%s: %v", a.Name, e)
		}
		checkNativeSchema(t, native)
		b, _ := json.Marshal(native)
		roundtrip, e := ParsePartitura(b)
		if e != nil {
			t.Fatalf("%s: %v", a.Name, e)
		}
		if len(roundtrip.Nodes) != len(a.Nodes) || len(roundtrip.Connections) != len(a.Connections) {
			t.Fatalf("lost components in %s", a.Name)
		}
		for i, n := range a.Nodes {
			got := roundtrip.Nodes[i]
			if n.Prompt != got.Prompt || n.Text != got.Text || n.URL != got.URL {
				t.Fatalf("lost content in %s", a.Name)
			}
		}
	}
	if partituras != 257 {
		t.Fatalf("expected full pinned catalog (257 partituras), got %d", partituras)
	}
	t.Logf("validated %d partituras and %d reference examples", partituras, len(entries)-partituras)
}

func checkNativeSchema(t *testing.T, native map[string]any) {
	t.Helper()
	if len(native) != 11 || native["formatVersion"] != 1 {
		t.Fatal("invalid native top-level schema")
	}
	for _, raw := range native["roles"].([]any) {
		role := raw.(map[string]any)
		if len(role) != 6 || role["schemaVersion"] != 1 {
			t.Fatal("native role requires schemaVersion 1")
		}
	}
	payload := native["payload"].(map[string]any)
	if len(payload) != 10 {
		t.Fatal("invalid native payload")
	}
	for _, kind := range []string{"connections", "noteConnections", "portalConnections"} {
		for _, raw := range payload[kind].([]any) {
			edge := raw.(map[string]any)
			if len(edge["ropePoints"].([][]float64)) != 21 {
				t.Fatal("native connections require 21 rope points")
			}
		}
	}
	for _, raw := range payload["nodes"].([]any) {
		node := raw.(map[string]any)
		if len(node) != 7 {
			t.Fatal("invalid native node schema")
		}
	}
}
