package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
	"github.com/permgps/herdr-telegram-agents/internal/testkit"
)

type relayMemory struct {
	b    []byte
	fail bool
}

func (m *relayMemory) Load() ([]byte, error) { return append([]byte(nil), m.b...), nil }
func (m *relayMemory) Save(b []byte) error {
	if m.fail {
		return errors.New("disk full")
	}
	m.b = append([]byte(nil), b...)
	return nil
}

type relayWireFake struct {
	domain.WireGateway
	feeds  map[string]domain.WireFeed
	calls  []string
	fail   bool
	native bool
}

func (f *relayWireFake) Info(context.Context) (domain.WireInfo, error) {
	return domain.WireInfo{ProtocolVersion: 1, Role: "owner", Capabilities: []string{"feedSnapshots", "canvasWrites", "roleManagement", "terminalDrafts", "partituras"}}, nil
}
func (f *relayWireFake) Workspaces(context.Context) ([]domain.WireWorkspace, error) {
	return []domain.WireWorkspace{{ID: "ws", Name: "Projeto Demo"}, {ID: "other", Name: "Outro"}}, nil
}
func (f *relayWireFake) Feed(_ context.Context, ws, _ string) (domain.WireFeed, error) {
	feed, ok := f.feeds[ws]
	if !ok {
		return feed, errors.New("offline")
	}
	return feed, nil
}
func (f *relayWireFake) Presets(context.Context) ([]domain.AgentPreset, error) {
	return []domain.AgentPreset{{ID: "preset", AgentType: "codex", Command: "codex"}}, nil
}
func (f *relayWireFake) Call(_ context.Context, _, method, path string, _ any, out any) error {
	f.calls = append(f.calls, method+" "+path)
	if f.fail {
		return errors.New("connection lost")
	}
	result := `{"ok":true}`
	if strings.HasSuffix(path, "/roles") {
		result = `{"id":"role"}`
	}
	if strings.HasSuffix(path, "/terminals") {
		result = `{"nodeId":"created","terminalId":"new-terminal"}`
	}
	if strings.HasSuffix(path, "/partituras") && method == "GET" {
		result = `{"partituras":[]}`
		if f.native {
			result = `{"partituras":[{"id":"native"}]}`
		}
	}
	if out != nil {
		return json.Unmarshal([]byte(result), out)
	}
	return nil
}

func newRelayTest(t *testing.T) (*Relay, *relayWireFake, *relayMemory) {
	t.Helper()
	wire := &relayWireFake{feeds: map[string]domain.WireFeed{}}
	feed := domain.WireFeed{Workspace: domain.WireWorkspace{ID: "ws", Name: "Projeto Demo"}, Epoch: "epoch", Floors: []domain.WireFloor{{ID: "floor", Name: "Redesign"}}, Canvas: json.RawMessage(`{"nodes":[{"id":"created"}]}`)}
	for _, id := range []string{"agent-a", "agent-b"} {
		feed.Items = append(feed.Items, domain.WireItem{Kind: "terminal", Terminal: domain.WireTerminal{ID: id, Name: "Mesmo nome", FloorID: "floor", IsLive: true, IsRunning: true, IsManager: true}})
	}
	wire.feeds["ws"] = feed
	store := &relayMemory{}
	r, err := NewRelay(domain.RelayConfig{WireURL: "https://host", ChatID: -100123, Operators: []int64{7}, Workspaces: []string{"ws"}}, wire, testkit.NewFakeTelegram(nil), store)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Topics[scope("ws", "floor")] = &relayTopic{Workspace: "ws", Floor: "floor", Thread: 100, Name: "Projeto Demo · Redesign"}
	r.state.Replies[10] = relayTarget{Workspace: "ws", Floor: "floor", Terminal: "agent-b", Epoch: "epoch", Thread: 100, At: time.Now()}
	return r, wire, store
}

func TestRelayRoutesRepliesAndRejectsAmbiguity(t *testing.T) {
	r, wire, _ := newRelayTest(t)
	ctx := context.Background()
	if _, _, err := r.target(ctx, 100, 0, ""); err == nil {
		t.Fatal("chose arbitrary manager")
	}
	target, _, err := r.target(ctx, 100, 10, "")
	if err != nil || target.Terminal != "agent-b" {
		t.Fatalf("wrong target: %+v %v", target, err)
	}
	ev := domain.TopicMessage{ThreadID: 100, MessageID: 55, ReplyTo: 10, FromID: 7, Text: "Olá"}
	if _, err = r.Handle(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Handle(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if len(wire.calls) != 1 || !strings.Contains(wire.calls[0], "agent-b/prompt") {
		t.Fatalf("duplicate or wrong command: %v", wire.calls)
	}
	ev.MessageID = 56
	ev.FromID = 8
	_, _ = r.Handle(ctx, ev)
	if len(wire.calls) != 1 {
		t.Fatal("unauthorized operator wrote")
	}
	feed := wire.feeds["ws"]
	feed.Epoch = "restarted"
	wire.feeds["ws"] = feed
	if _, _, err = r.target(ctx, 100, 10, ""); err == nil {
		t.Fatal("old session accepted")
	}
	if _, _, err = r.target(ctx, 999, 10, ""); err == nil {
		t.Fatal("cross-topic reply accepted")
	}
}

func TestRelayPersistenceFailureStopsBeforePrompt(t *testing.T) {
	r, wire, store := newRelayTest(t)
	store.fail = true
	_, err := r.Handle(context.Background(), domain.TopicMessage{ThreadID: 100, MessageID: 42, ReplyTo: 10, FromID: 7, Text: "execute"})
	if err == nil || len(wire.calls) != 0 {
		t.Fatal("wrote despite persistence failure")
	}
}

func TestRelayButtonCannotEscalateAction(t *testing.T) {
	r, wire, _ := newRelayTest(t)
	bound := r.state.Replies[10]
	bound.Actions = []string{"screen"}
	r.state.Replies[10] = bound
	err := r.button(context.Background(), domain.ButtonPressed{ThreadID: 100, MessageID: 10, Data: "kill"})
	if err == nil || len(wire.calls) > 0 {
		t.Fatal("forged button executed kill")
	}
}

func testRelayPlan() domain.MaestroPlan {
	return domain.MaestroPlan{WorkspaceID: "ws", FloorID: "floor", Arrangement: domain.Arrangement{Name: "Time", Nodes: []domain.ArrangementNode{{ID: "a", Kind: "terminal", Name: "Agent", PresetID: "preset", AgentType: "codex", Prompt: "Revise.", Width: 600, Height: 420}}}}
}

func TestRelayDeployPersistsAndDoesNotRepeat(t *testing.T) {
	r, wire, store := newRelayTest(t)
	ctx := context.Background()
	plan := testRelayPlan()
	plan.Arrangement.Nodes[0].PresetID = ""
	if _, err := r.Deploy(ctx, "creation", plan); err != nil {
		t.Fatal(err)
	}
	if plan.Arrangement.Nodes[0].PresetID != "" {
		t.Fatal("deployment modified original plan")
	}
	before := len(wire.calls)
	restarted, err := NewRelay(r.Config, wire, r.Telegram, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Deploy(ctx, "creation", plan); err != nil {
		t.Fatal(err)
	}
	if len(wire.calls) != before {
		t.Fatal("creation replayed")
	}
	plan.Arrangement.Name = "Different"
	if _, err = restarted.Deploy(ctx, "creation", plan); err == nil {
		t.Fatal("job ID reused for different plan")
	}
}

func TestRelayDeployUncertainWriteIsNotRetried(t *testing.T) {
	r, wire, _ := newRelayTest(t)
	wire.fail = true
	ctx := context.Background()
	plan := testRelayPlan()
	if _, err := r.Deploy(ctx, "uncertain", plan); err == nil {
		t.Fatal("expected failure")
	}
	before := len(wire.calls)
	wire.fail = false
	if _, err := r.Deploy(ctx, "uncertain", plan); err == nil {
		t.Fatal("uncertain operation replayed")
	}
	if len(wire.calls) != before {
		t.Fatal("write retried after uncertain outcome")
	}
}

func TestRelayPortalPreflightPreservesAllComponents(t *testing.T) {
	r, wire, _ := newRelayTest(t)
	plan := testRelayPlan()
	plan.Arrangement.Nodes = append(plan.Arrangement.Nodes, domain.ArrangementNode{ID: "portal", Kind: "portal", Name: "App", URL: "https://example.com", Width: 600, Height: 400})
	r.Native = func(domain.Arrangement, []domain.AgentPreset) (map[string]any, error) {
		return map[string]any{"id": "native"}, nil
	}
	_, err := r.Deploy(context.Background(), "import", plan)
	var required *ImportRequired
	if !errors.As(err, &required) {
		t.Fatalf("expected import requirement, got %v", err)
	}
	for _, call := range wire.calls {
		if strings.HasPrefix(call, "POST") {
			t.Fatal("partial deployment before portal preflight")
		}
	}
}

func TestRelayNativeStampVerifiesAndResumesWithoutReplay(t *testing.T) {
	r, wire, store := newRelayTest(t)
	wire.native = true
	plan := testRelayPlan()
	plan.Arrangement.Nodes = append(plan.Arrangement.Nodes, domain.ArrangementNode{ID: "portal", Kind: "portal", Name: "App", URL: "https://example.com", Width: 600, Height: 400})
	r.Native = func(domain.Arrangement, []domain.AgentPreset) (map[string]any, error) {
		return map[string]any{"id": "native"}, nil
	}
	ctx := context.Background()
	if _, err := r.Deploy(ctx, "stamp", plan); err == nil || !strings.Contains(err.Error(), "/resume") {
		t.Fatalf("unverified stamp accepted: %v", err)
	}
	if r.state.Jobs["stamp"].Done {
		t.Fatal("marked unfinished stamp done")
	}
	feed := wire.feeds["ws"]
	feed.Canvas = json.RawMessage(`{"nodes":[{"id":"created"},{"id":"new-agent","kind":"terminal"},{"id":"new-portal","kind":"portal"}]}`)
	wire.feeds["ws"] = feed
	restarted, err := NewRelay(r.Config, wire, r.Telegram, store)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Native = r.Native
	if _, err = restarted.Deploy(ctx, "stamp", plan); err != nil {
		t.Fatal(err)
	}
	stamps := 0
	for _, call := range wire.calls {
		if call == "POST /api/workspaces/ws/partituras" {
			stamps++
		}
	}
	if stamps != 1 || !restarted.state.Jobs["stamp"].Done {
		t.Fatalf("stamps=%d done=%v", stamps, restarted.state.Jobs["stamp"].Done)
	}
}

func TestRelayRejectsNonCreationPlansAndMissingSession(t *testing.T) {
	r, wire, _ := newRelayTest(t)
	for _, action := range []string{"answer", "preview", "destroy"} {
		plan := testRelayPlan()
		plan.Action = action
		if _, err := r.Deploy(context.Background(), "guard", plan); err == nil {
			t.Fatalf("accepted action %s", action)
		}
	}
	if len(wire.calls) != 0 {
		t.Fatal("noncreation plan wrote to Wire")
	}
	feed := wire.feeds["ws"]
	feed.Epoch = ""
	wire.feeds["ws"] = feed
	if _, _, err := r.target(context.Background(), 100, 0, "agent-a"); err == nil {
		t.Fatal("accepted feed without session identity")
	}
}

type relayPlanningCheck struct{ t *testing.T }

func (p relayPlanningCheck) Plan(_ context.Context, _ string, _ domain.MaestroPlan, workspaces []domain.WireWorkspace, _ []domain.AgentPreset) (domain.MaestroPlan, error) {
	p.t.Helper()
	if len(workspaces) != 1 || workspaces[0].ID != "ws" || len(workspaces[0].Floors) != 1 || workspaces[0].Floors[0].ID != "floor" {
		p.t.Fatalf("planner lacks authorized floor context: %+v", workspaces)
	}
	return domain.MaestroPlan{Action: "answer", Message: "Andar Redesign disponível."}, nil
}

func TestRelayMaestroReceivesFloorsAndAnswersWithoutCreation(t *testing.T) {
	r, wire, _ := newRelayTest(t)
	r.Planner = relayPlanningCheck{t: t}
	topic, err := r.Telegram.CreateTopic(context.Background(), "Maestro", domain.StatusIdle)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.maestro(context.Background(), topic.ThreadID, "m:888", "/create", "Quais andares existem?"); err != nil {
		t.Fatal(err)
	}
	if len(wire.calls) != 0 || len(r.state.Jobs) != 0 {
		t.Fatal("answer unexpectedly created resources")
	}
}
