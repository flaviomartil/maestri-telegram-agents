package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

type relayTopic struct {
	Workspace string
	Floor     string
	Name      string
	Thread    int
	Pending   bool
	Muted     bool
}

type relayTarget struct {
	Workspace string
	Floor     string
	Terminal  string
	Epoch     string
	Thread    int
	Prompt    string
	At        time.Time
	Actions   []string
}

type relayJob struct {
	Plan    domain.MaestroPlan
	Pending string
	Results map[string]json.RawMessage
	Done    bool
}

type relayState struct {
	Version int
	ChatID  int64
	Host    string
	Topics  map[string]*relayTopic
	Replies map[int]relayTarget
	Seen    map[string]time.Time
	Jobs    map[string]*relayJob
}

type Relay struct {
	Config    domain.RelayConfig
	Wire      domain.WireGateway
	Telegram  domain.TelegramGateway
	Catalog   domain.ArrangementCatalog
	Planner   domain.ArrangementPlanner
	Native    func(domain.Arrangement, []domain.AgentPreset) (map[string]any, error)
	Store     domain.RelayStore
	mu        sync.Mutex
	provision sync.Mutex
	refresh   sync.Mutex
	state     relayState
	feeds     map[string]domain.WireFeed
	statuses  map[string]string
	locks     sync.Map
	failure   error
}

func NewRelay(cfg domain.RelayConfig, wire domain.WireGateway, tg domain.TelegramGateway, store domain.RelayStore) (*Relay, error) {
	r := &Relay{Config: cfg, Wire: wire, Telegram: tg, Store: store, feeds: map[string]domain.WireFeed{}, statuses: map[string]string{}}
	b, err := store.Load()
	if err != nil {
		return nil, err
	}
	if len(b) > 0 {
		if err = json.Unmarshal(b, &r.state); err != nil {
			return nil, errors.New("estado Relay corrompido; preserve o arquivo para recuperação")
		}
	}
	if r.state.Version != 0 && r.state.Version != 1 {
		return nil, errors.New("versão de estado Relay não suportada")
	}
	if r.state.Version != 0 && (r.state.ChatID != cfg.ChatID || r.state.Host != cfg.WireURL+"|"+cfg.WirePin) {
		return nil, errors.New("estado pertence a outro grupo ou host; use outro stateDir")
	}
	r.state.Version = 1
	r.state.ChatID = cfg.ChatID
	r.state.Host = cfg.WireURL + "|" + cfg.WirePin
	if r.state.Topics == nil {
		r.state.Topics = map[string]*relayTopic{}
	}
	if r.state.Replies == nil {
		r.state.Replies = map[int]relayTarget{}
	}
	if r.state.Seen == nil {
		r.state.Seen = map[string]time.Time{}
	}
	if r.state.Jobs == nil {
		r.state.Jobs = map[string]*relayJob{}
	}
	return r, nil
}

func (r *Relay) save() error {
	if r.failure != nil {
		return r.failure
	}
	b, err := json.Marshal(r.state)
	if err == nil {
		err = r.Store.Save(b)
	}
	if err != nil {
		r.failure = errors.New("falha ao persistir estado; Relay interrompido para evitar duplicação")
		return r.failure
	}
	return nil
}

func scope(ws, floor string) string {
	if floor == "ground" {
		floor = ""
	}
	return ws + "/" + floor
}

func (r *Relay) ensureTopic(ctx context.Context, ws, floor, name string) (int, error) {
	key := scope(ws, floor)
	t, ok := r.state.Topics[key]
	if ok {
		if t.Pending {
			return 0, fmt.Errorf("criação do tópico %s incerta; vincule o tópico existente com /bind", name)
		}
		if t.Name != name {
			if err := r.Telegram.EditTopic(ctx, t.Thread, domain.TopicPatch{Name: &name}); err != nil {
				return 0, err
			}
			t.Name = name
			if err := r.save(); err != nil {
				return 0, err
			}
		}
		return t.Thread, nil
	}
	t = &relayTopic{Workspace: ws, Floor: floor, Name: name, Pending: true}
	r.state.Topics[key] = t
	if err := r.save(); err != nil {
		return 0, err
	}
	created, err := r.Telegram.CreateTopic(ctx, name, domain.StatusIdle)
	if err != nil {
		return 0, err
	}
	t.Thread = created.ThreadID
	t.Pending = false
	return t.Thread, r.save()
}

func (r *Relay) Refresh(ctx context.Context) error {
	r.refresh.Lock()
	defer r.refresh.Unlock()
	if _, err := r.Wire.Info(ctx); err != nil {
		return err
	}
	wss, err := r.Wire.Workspaces(ctx)
	if err != nil {
		return err
	}
	next := map[string]domain.WireFeed{}
	for _, ws := range wss {
		if !r.Config.Allows(ws.ID) || ws.IsLocked {
			continue
		}
		feed, e := r.Wire.Feed(ctx, ws.ID, "ground")
		if e != nil {
			continue
		}
		feed.Workspace = ws
		next[ws.ID] = feed
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feeds = next
	if _, err = r.ensureTopic(ctx, "maestro", "", "Maestro"); err != nil {
		return err
	}
	for _, ws := range wss {
		feed, ok := next[ws.ID]
		if !ok {
			continue
		}
		floors := append([]domain.WireFloor{{Name: "Térreo"}}, feed.Floors...)
		seen := map[string]bool{}
		for _, floor := range floors {
			if seen[floor.ID] {
				continue
			}
			seen[floor.ID] = true
			if floor.IsCloneMissing {
				continue
			}
			if floor.Name == "" {
				floor.Name = "Térreo"
			}
			if _, err = r.ensureTopic(ctx, ws.ID, floor.ID, ws.Name+" · "+floor.Name); err != nil {
				return err
			}
		}
		for _, item := range feed.Items {
			if item.Kind != "terminal" && item.Kind != "pendingPrompt" {
				continue
			}
			a := item.Terminal
			t := r.state.Topics[scope(ws.ID, a.FloorID)]
			if t == nil || t.Pending {
				continue
			}
			key := ws.ID + "/" + a.ID
			status := feed.Epoch + "/" + a.Status + "/" + strconv.FormatBool(a.NeedsAttention) + "/" + item.Prompt
			old, exists := r.statuses[key]
			r.statuses[key] = status
			if (exists && old == status) || t.Muted {
				continue
			}
			if !a.NeedsAttention && (!exists || a.Status == "running") {
				continue
			}
			target := relayTarget{Workspace: ws.ID, Floor: a.FloorID, Terminal: a.ID, Epoch: feed.Epoch, Thread: t.Thread, Prompt: item.Prompt, At: time.Now()}
			var buttons []domain.Button
			if item.Kind == "pendingPrompt" {
				buttons = []domain.Button{{Text: "Aprovar", Data: "approve", Row: 1}, {Text: "Recusar", Data: "reject", Row: 1}}
			}
			if err = r.sendTarget(ctx, target, "["+a.Name+"] "+a.Status+"\n"+strings.Join(a.Preview, "\n"), buttons, a.NeedsAttention); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Relay) sendTarget(ctx context.Context, target relayTarget, text string, buttons []domain.Button, notify bool) error {
	for _, b := range buttons {
		target.Actions = append(target.Actions, b.Data)
	}
	redactor := domain.NewRedactor(r.Config.WireToken, r.Config.BotToken, r.Config.LLMKey)
	text, _ = redactor.Redact(text)
	id, err := r.Telegram.Send(ctx, domain.Outgoing{ThreadID: target.Thread, Text: text, Buttons: buttons, Notify: notify, MaxParts: 1})
	if err != nil {
		return err
	}
	r.state.Replies[id] = target
	if len(r.state.Replies) > 4000 {
		ids := make([]int, 0, len(r.state.Replies))
		for k := range r.state.Replies {
			ids = append(ids, k)
		}
		sort.Ints(ids)
		for _, k := range ids[:1000] {
			delete(r.state.Replies, k)
		}
	}
	return r.save()
}

func (r *Relay) say(ctx context.Context, thread int, text string) error {
	redactor := domain.NewRedactor(r.Config.WireToken, r.Config.BotToken, r.Config.LLMKey)
	text, _ = redactor.Redact(text)
	_, err := r.Telegram.Send(ctx, domain.Outgoing{ThreadID: thread, Text: text, MaxParts: 2})
	return err
}

func (r *Relay) target(ctx context.Context, thread, reply int, explicit string) (relayTarget, domain.WireTerminal, error) {
	r.mu.Lock()
	var topic *relayTopic
	for _, t := range r.state.Topics {
		if t.Thread == thread && !t.Pending {
			copy := *t
			topic = &copy
			break
		}
	}
	bound, hasReply := r.state.Replies[reply]
	r.mu.Unlock()
	if topic == nil || topic.Workspace == "maestro" || !r.Config.Allows(topic.Workspace) {
		return relayTarget{}, domain.WireTerminal{}, errors.New("tópico sem andar autorizado")
	}
	feed, err := r.Wire.Feed(ctx, topic.Workspace, topic.Floor)
	if err != nil {
		return relayTarget{}, domain.WireTerminal{}, err
	}
	if feed.Workspace.ID != topic.Workspace || !domain.ValidRelayID(feed.Epoch) {
		return relayTarget{}, domain.WireTerminal{}, errors.New("feed sem identidade de workspace e sessão válida")
	}
	if reply != 0 && (!hasReply || bound.Thread != thread || bound.Workspace != topic.Workspace || bound.Floor != topic.Floor || bound.Epoch != feed.Epoch) {
		return relayTarget{}, domain.WireTerminal{}, errors.New("resposta antiga ou sem destino válido; use /agents")
	}
	var candidates []domain.WireTerminal
	prompts := map[string]string{}
	for _, item := range feed.Items {
		a := item.Terminal
		if a.ID == "" || a.FloorID != topic.Floor || !a.IsLive || !a.IsRunning {
			continue
		}
		prompts[a.ID] = item.Prompt
		if reply != 0 {
			if a.ID == bound.Terminal {
				candidates = append(candidates, a)
			}
		} else if explicit != "" {
			if a.ID == explicit {
				candidates = append(candidates, a)
			}
		} else if a.IsManager {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) != 1 {
		return relayTarget{}, domain.WireTerminal{}, errors.New("destino ausente ou ambíguo; use /agents e responda ao agente desejado")
	}
	a := candidates[0]
	return relayTarget{Workspace: topic.Workspace, Floor: topic.Floor, Terminal: a.ID, Epoch: feed.Epoch, Thread: thread, Prompt: prompts[a.ID], At: time.Now()}, a, nil
}

func (r *Relay) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rights, err := r.Telegram.Rights(ctx)
	if err != nil {
		return err
	}
	if !rights.IsForum || !rights.IsAdmin || !rights.CanManageTopics {
		return errors.New("bot precisa ser administrador de um grupo com tópicos e permissão de gerenciá-los")
	}
	if err = r.Refresh(ctx); err != nil {
		return err
	}
	interval := r.Config.PollSeconds
	if interval < 2 {
		interval = 5
	}
	tick := time.NewTicker(time.Duration(interval) * time.Second)
	defer tick.Stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, 8)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			_ = r.Refresh(ctx)
			r.mu.Lock()
			failure := r.failure
			r.mu.Unlock()
			if failure != nil {
				cancel()
				return failure
			}
		case ev, ok := <-r.Telegram.Events():
			if !ok {
				cancel()
				return errors.New("conexão Telegram encerrada")
			}
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-slots }()
				thread, e := r.Handle(ctx, ev)
				if e != nil {
					_ = r.say(ctx, thread, e.Error())
				}
			}()
		}
	}
}

func (r *Relay) once(key string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.state.Seen[key]; ok {
		return false, nil
	}
	now := time.Now()
	for id, at := range r.state.Seen {
		if now.Sub(at) > 48*time.Hour {
			delete(r.state.Seen, id)
		}
	}
	r.state.Seen[key] = now
	return true, r.save()
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
