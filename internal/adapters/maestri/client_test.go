package maestri

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func TestWireTrustAndAuthorization(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Error("missing token")
		}
		_ = json.NewEncoder(w).Encode(domain.WireInfo{ProtocolVersion: 1, Role: "guest", Capabilities: []string{"feedSnapshots"}})
	}))
	defer s.Close()
	h := sha256.Sum256(s.Certificate().RawSubjectPublicKeyInfo)
	pin := base64.StdEncoding.EncodeToString(h[:])
	c, err := New(s.URL, pin, "private-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Info(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	if err = c.Call(context.Background(), "", "POST", "/api/workspaces", map[string]string{"name": "no"}, nil); err == nil {
		t.Fatal("guest wrote")
	}
	if err = c.Call(context.Background(), "missing", "GET", "/api/workspaces", nil, nil); err == nil {
		t.Fatal("missing capability probed")
	}
	if calls.Load() != before {
		t.Fatal("unauthorized request reached host")
	}
	bad, err := New(s.URL, strings.Repeat("00", 32), "private-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bad.Info(context.Background()); err == nil {
		t.Fatal("mismatched pin accepted")
	}
	if calls.Load() != before {
		t.Fatal("token sent before pin verification")
	}
	for _, u := range []string{"http://localhost", "https://user:pass@localhost", "https://localhost/api", "https://localhost?token=secret"} {
		if _, err = New(u, pin, ""); err == nil {
			t.Errorf("accepted %s", u)
		}
	}
}

func TestWireDoesNotReplayMutationsOrLeakErrors(t *testing.T) {
	var writes atomic.Int32
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			_ = json.NewEncoder(w).Encode(domain.WireInfo{ProtocolVersion: 1, Role: "owner", Capabilities: []string{"canvasWrites"}})
			return
		}
		writes.Add(1)
		w.WriteHeader(500)
		_, _ = w.Write([]byte("private-token"))
	}))
	defer s.Close()
	h := sha256.Sum256(s.Certificate().RawSubjectPublicKeyInfo)
	c, _ := New(s.URL, base64.StdEncoding.EncodeToString(h[:]), "private-token")
	err := c.Call(context.Background(), "canvasWrites", "POST", "/api/workspaces/ws/nodes", map[string]string{"kind": "note"}, nil)
	if err == nil || writes.Load() != 1 || strings.Contains(err.Error(), "private-token") {
		t.Fatalf("unsafe failure: writes=%d error=%v", writes.Load(), err)
	}
}

func TestPartituraRoundTripUsesOnlyInstalledCommands(t *testing.T) {
	a := domain.Arrangement{ID: "example", Name: "Equipe", Description: "Teste", Nodes: []domain.ArrangementNode{
		{ID: "a", Kind: "terminal", Name: "Agente", AgentType: "codex", Prompt: "Revise o trabalho.", Manager: true, Width: 600, Height: 420},
		{ID: "n", Kind: "note", Name: "Briefing", Text: "Objetivo", X: 700, Width: 300, Height: 200},
		{ID: "p", Kind: "portal", Name: "App", URL: "https://example.com", Y: 600, Width: 500, Height: 400},
	}, Connections: []domain.ArrangementConnection{{From: "a", To: "n"}, {From: "a", To: "p"}}}
	presets := []domain.AgentPreset{{ID: "safe", AgentType: "codex", Command: "codex"}}
	native, err := NativePartitura(a, presets)
	if err != nil {
		t.Fatal(err)
	}
	if a.Nodes[0].PresetID != "" {
		t.Fatal("export modified the source arrangement")
	}
	checkNativeSchema(t, native)
	b, _ := json.Marshal(native)
	parsed, err := ParsePartitura(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Nodes) != 3 || len(parsed.Connections) != 2 || parsed.Nodes[0].Prompt != a.Nodes[0].Prompt {
		t.Fatal("lost components")
	}
	if strings.Contains(string(b), "skip-permissions") {
		t.Fatal("unsafe command")
	}
	second, _ := NativePartitura(a, presets)
	if native["id"] != second["id"] {
		t.Fatal("export is not deterministic")
	}
	a.Nodes[0].Prompt = "Outra responsabilidade"
	variant, _ := NativePartitura(a, presets)
	if native["id"] == variant["id"] {
		t.Fatal("variant overwrote original")
	}
	if _, err = NativePartitura(a, nil); err == nil {
		t.Fatal("invented preset")
	}
	a.Connections = append(a.Connections, domain.ArrangementConnection{From: "a", To: "missing"})
	if err = a.Validate(); err == nil {
		t.Fatal("dangling connection")
	}
}

func TestPlannerRejectsExtraExecutionFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"workspaceId":"ws","floorId":"","shell":"rm -rf /","arrangement":{}}`}}}})
	}))
	defer s.Close()
	p := &Planner{URL: s.URL, Model: "test"}
	if _, err := p.Plan(context.Background(), "Crie equipe", domain.MaestroPlan{}, nil, nil); err == nil {
		t.Fatal("accepted shell injection")
	}
}
