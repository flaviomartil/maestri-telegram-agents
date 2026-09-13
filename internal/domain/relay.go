package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
)

type WireInfo struct {
	Name            string   `json:"name"`
	ProtocolVersion int      `json:"protocolVersion"`
	Capabilities    []string `json:"capabilities"`
	Role            string   `json:"role"`
}

func (i WireInfo) Has(cap string) bool {
	for _, c := range i.Capabilities {
		if c == cap {
			return true
		}
	}
	return cap == ""
}

type WireWorkspace struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	WorkingDirectory string      `json:"workingDirectory"`
	IsLocked         bool        `json:"isLocked"`
	Floors           []WireFloor `json:"floors,omitempty"`
}

type WireFloor struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	IsCloneMissing bool   `json:"isCloneMissing"`
}

type WireTerminal struct {
	ID             string   `json:"id"`
	NodeID         string   `json:"nodeId"`
	Name           string   `json:"name"`
	AgentType      string   `json:"agentType"`
	FloorID        string   `json:"floorId"`
	FloorName      string   `json:"floorName"`
	Status         string   `json:"status"`
	IsRunning      bool     `json:"isRunning"`
	IsLive         bool     `json:"isLive"`
	IsManager      bool     `json:"isManager"`
	NeedsAttention bool     `json:"needsAttention"`
	Preview        []string `json:"preview"`
}

type WireItem struct {
	Kind     string       `json:"kind"`
	Terminal WireTerminal `json:"terminal"`
	Prompt   string       `json:"prompt"`
}

type WireFeed struct {
	Workspace     WireWorkspace   `json:"workspace"`
	Floors        []WireFloor     `json:"floors"`
	Items         []WireItem      `json:"items"`
	Epoch         string          `json:"epoch"`
	Canvas        json.RawMessage `json:"canvas"`
	PendingFloors []struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	} `json:"pendingFloors"`
}

type AgentPreset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AgentType string `json:"agentType"`
	Command   string `json:"command"`
	IsDefault bool   `json:"isDefault"`
}

type WireGateway interface {
	Info(context.Context) (WireInfo, error)
	Workspaces(context.Context) ([]WireWorkspace, error)
	Feed(context.Context, string, string) (WireFeed, error)
	Presets(context.Context) ([]AgentPreset, error)
	Call(context.Context, string, string, string, any, any) error
	Input(context.Context, string, []byte) error
	Screen(context.Context, string) (string, error)
	Upload(context.Context, string, string, string, []byte) (string, error)
}

type RelayConfig struct {
	WireURL     string   `json:"wireURL"`
	WirePin     string   `json:"wirePin"`
	WireToken   string   `json:"-"`
	BotToken    string   `json:"-"`
	ChatID      int64    `json:"chatID"`
	Operators   []int64  `json:"operators"`
	Observers   []int64  `json:"observers"`
	Workspaces  []string `json:"workspaces"`
	StateDir    string   `json:"stateDir"`
	CatalogDir  string   `json:"catalogDir"`
	PollSeconds int      `json:"pollSeconds"`
	LLMURL      string   `json:"llmURL"`
	LLMModel    string   `json:"llmModel"`
	LLMKey      string   `json:"-"`
}

func (c RelayConfig) Allows(ws string) bool {
	for _, id := range c.Workspaces {
		if id == ws || id == "*" {
			return true
		}
	}
	return false
}

func (c RelayConfig) Operator(id int64) bool {
	for _, allowed := range c.Operators {
		if allowed == id && id > 0 {
			return true
		}
	}
	return false
}

type ArrangementNode struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Name      string  `json:"name"`
	PresetID  string  `json:"presetId,omitempty"`
	AgentType string  `json:"agentType,omitempty"`
	Prompt    string  `json:"prompt,omitempty"`
	Text      string  `json:"text,omitempty"`
	URL       string  `json:"url,omitempty"`
	Manager   bool    `json:"manager,omitempty"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
}

type ArrangementConnection struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Arrangement struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Source      string                  `json:"source,omitempty"`
	Revision    string                  `json:"revision,omitempty"`
	Nodes       []ArrangementNode       `json:"nodes"`
	Connections []ArrangementConnection `json:"connections"`
}

var relayID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

func ValidRelayID(id string) bool { return relayID.MatchString(id) }

func (a Arrangement) Validate() error {
	if strings.TrimSpace(a.Name) == "" || len(a.Name) > 200 || len(a.Nodes) == 0 || len(a.Nodes) > 80 || len(a.Connections) > 400 {
		return errors.New("arranjo precisa de nome, 1 a 80 nós e no máximo 400 conexões")
	}
	nodes := map[string]ArrangementNode{}
	for _, n := range a.Nodes {
		if !ValidRelayID(n.ID) || n.Name == "" || len(n.Name) > 200 {
			return fmt.Errorf("identidade de nó inválida: %q", n.ID)
		}
		if _, ok := nodes[n.ID]; ok {
			return fmt.Errorf("nó duplicado: %s", n.ID)
		}
		for _, v := range []float64{n.X, n.Y, n.Width, n.Height} {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 100000 {
				return errors.New("geometria inválida")
			}
		}
		if n.Width < 40 || n.Height < 40 || len(n.Text) > 100000 || len(n.Prompt) > 50000 {
			return errors.New("dimensões ou conteúdo fora dos limites")
		}
		switch n.Kind {
		case "terminal":
			if strings.TrimSpace(n.Prompt) == "" {
				return fmt.Errorf("responsabilidade ausente: %s", n.Name)
			}
		case "note":
		case "portal":
			u, err := url.Parse(n.URL)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
				return errors.New("portal exige URL HTTP(S) sem credenciais")
			}
		default:
			return fmt.Errorf("tipo de nó sem suporte: %s", n.Kind)
		}
		nodes[n.ID] = n
	}
	edges := map[string]bool{}
	for _, c := range a.Connections {
		_, from := nodes[c.From]
		_, to := nodes[c.To]
		if !from || !to || c.From == c.To {
			return errors.New("conexão com destino inválido")
		}
		key := c.From + "/" + c.To
		reverse := c.To + "/" + c.From
		if edges[key] || edges[reverse] {
			return errors.New("conexão duplicada")
		}
		edges[key] = true
	}
	return nil
}

type MaestroPlan struct {
	Action        string      `json:"action,omitempty"`
	Message       string      `json:"message,omitempty"`
	WorkspaceID   string      `json:"workspaceId"`
	WorkspaceName string      `json:"workspaceName,omitempty"`
	Directory     string      `json:"directory,omitempty"`
	FloorID       string      `json:"floorId"`
	FloorName     string      `json:"floorName,omitempty"`
	Arrangement   Arrangement `json:"arrangement"`
}

type ArrangementCatalog interface {
	List() []Arrangement
	Get(string) (Arrangement, error)
}

type ArrangementPlanner interface {
	Plan(context.Context, string, MaestroPlan, []WireWorkspace, []AgentPreset) (MaestroPlan, error)
}

type RelayStore interface {
	Load() ([]byte, error)
	Save([]byte) error
}
