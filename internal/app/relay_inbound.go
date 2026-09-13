package app

import (
	"context"
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

const relayHelp = `Maestri Relay
/status ou /workspaces: andares e conexões
/agents: agentes do tópico; responda ao card para conversar
/screen: prévia do terminal escolhido
/focus, /stop, /interrupt, /close: controlar o agente escolhido
/mute e /unmute: notificações do andar
/partituras <descrição>: buscar todo o catálogo
/apply <id>: aplicar exemplo neste andar
/adapt <id> <descrição>: adaptar exemplo com IA
/create <descrição>: criar equipe com IA
/preview <descrição>: somente gerar o plano
/resume <id>: retomar criação com resultados já registrados
/bind <workspace-id> <floor-id|ground> <topic-id>: recuperar vínculo de tópico
Texto sem resposta vai ao coordenador do andar. No tópico Maestro, texto cria uma equipe pela descrição.`

func (r *Relay) Handle(ctx context.Context, ev domain.Event) (int, error) {
	var thread, msg, reply int
	var user int64
	var text, key string
	switch e := ev.(type) {
	case domain.TopicMessage:
		thread, msg, reply, user, text = e.ThreadID, e.MessageID, e.ReplyTo, e.FromID, e.Text
		key = "m:" + strconv.Itoa(msg)
	case domain.GeneralCommand:
		thread, msg, user, text = 0, e.MessageID, e.FromID, e.Text
		key = "m:" + strconv.Itoa(msg)
		if !r.Config.Operator(user) {
			if e.Role == domain.RoleObserver && (text == "/help" || text == "/status") {
				if text == "/help" {
					return 0, r.say(ctx, 0, relayHelp)
				}
				return 0, r.status(ctx, 0)
			}
			return 0, nil
		}
	case domain.ButtonPressed:
		if !r.Config.Operator(e.FromID) {
			return e.ThreadID, nil
		}
		ok, err := r.once("b:" + e.CallbackID)
		if err != nil || !ok {
			return e.ThreadID, err
		}
		_ = r.Telegram.AnswerButton(ctx, e.CallbackID, "Recebido")
		return e.ThreadID, r.button(ctx, e)
	case domain.TopicAttachment:
		if !r.Config.Operator(e.FromID) {
			return e.ThreadID, nil
		}
		ok, err := r.once("m:" + strconv.Itoa(e.MessageID))
		if err != nil || !ok {
			return e.ThreadID, err
		}
		return e.ThreadID, r.attachment(ctx, e)
	default:
		return 0, nil
	}
	if !r.Config.Operator(user) {
		return thread, nil
	}
	ok, err := r.once(key)
	if err != nil || !ok {
		return thread, err
	}
	word, args, _ := strings.Cut(strings.TrimSpace(text), " ")
	word, _, _ = strings.Cut(word, "@")
	args = strings.TrimSpace(args)
	switch word {
	case "/help", "/start":
		return thread, r.say(ctx, thread, relayHelp)
	case "/status", "/workspaces", "/floors":
		return thread, r.status(ctx, thread)
	case "/agents":
		return thread, r.agents(ctx, thread)
	case "/partituras", "/catalog":
		return thread, r.catalog(ctx, thread, args)
	case "/apply", "/adapt", "/create", "/preview":
		return thread, r.maestro(ctx, thread, key, word, args)
	case "/resume":
		r.mu.Lock()
		job := r.state.Jobs[args]
		var plan domain.MaestroPlan
		if job != nil {
			plan = job.Plan
		}
		r.mu.Unlock()
		if job == nil {
			return thread, errors.New("criação não encontrada")
		}
		result, e := r.Deploy(ctx, args, plan)
		if e != nil {
			return thread, e
		}
		return thread, r.say(ctx, thread, result)
	case "/bind":
		return thread, r.bind(ctx, args)
	case "/mute", "/unmute":
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, t := range r.state.Topics {
			if t.Thread == thread {
				t.Muted = word == "/mute"
				return thread, r.save()
			}
		}
		return thread, errors.New("tópico desconhecido")
	}
	r.mu.Lock()
	maestro := r.state.Topics[scope("maestro", "")]
	isMaestro := maestro != nil && maestro.Thread == thread
	r.mu.Unlock()
	if isMaestro {
		return thread, r.maestro(ctx, thread, key, "/create", text)
	}
	target, agent, err := r.target(ctx, thread, reply, "")
	if err != nil {
		return thread, err
	}
	lock, _ := r.locks.LoadOrStore(target.Terminal, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	target, agent, err = r.target(ctx, thread, reply, target.Terminal)
	if err != nil {
		return thread, err
	}
	switch word {
	case "/screen":
		r.mu.Lock()
		defer r.mu.Unlock()
		return thread, r.sendTarget(ctx, target, "["+agent.Name+"]\n"+strings.Join(agent.Preview, "\n"), nil, false)
	case "/focus":
		return thread, r.Wire.Call(ctx, "terminalFocus", "POST", "/api/terminals/"+target.Terminal+"/focus", map[string]any{}, nil)
	case "/stop":
		return thread, r.Wire.Input(ctx, target.Terminal, []byte{27})
	case "/interrupt":
		return thread, r.Wire.Input(ctx, target.Terminal, []byte{3})
	case "/close":
		r.mu.Lock()
		defer r.mu.Unlock()
		return thread, r.sendTarget(ctx, target, "Encerrar o processo de "+agent.Name+"?", []domain.Button{{Text: "Sim, encerrar", Data: "kill"}, {Text: "Cancelar", Data: "cancel"}}, false)
	case "/approve", "/reject":
		return thread, errors.New("responda usando os botões da pergunta atual")
	}
	if strings.HasPrefix(word, "/") && word != "/clear" && word != "/compact" && word != "/model" && word != "/usage" {
		return thread, errors.New("comando desconhecido; consulte /help")
	}
	err = r.Wire.Call(ctx, "", "POST", "/api/terminals/"+target.Terminal+"/prompt", map[string]string{"text": text}, nil)
	if err != nil {
		return thread, fmt.Errorf("envio não confirmado; não foi repetido automaticamente: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.Replies[msg] = target
	return thread, r.save()
}

func (r *Relay) status(ctx context.Context, thread int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	lines := []string{"Maestri Relay"}
	keys := make([]string, 0, len(r.state.Topics))
	for key := range r.state.Topics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t := r.state.Topics[key]
		state := "conectado"
		if _, ok := r.feeds[t.Workspace]; !ok && t.Workspace != "maestro" {
			state = "indisponível"
		}
		if t.Pending {
			state = "criação incerta"
		}
		lines = append(lines, fmt.Sprintf("%s: %s (%s)", t.Name, state, topicLink(r.Config.ChatID, t.Thread)))
	}
	return r.say(ctx, thread, strings.Join(lines, "\n"))
}

func (r *Relay) agents(ctx context.Context, thread int) error {
	r.mu.Lock()
	var topic relayTopic
	for _, t := range r.state.Topics {
		if t.Thread == thread {
			topic = *t
		}
	}
	r.mu.Unlock()
	if !r.Config.Allows(topic.Workspace) {
		return errors.New("abra um tópico Workspace · Andar")
	}
	feed, err := r.Wire.Feed(ctx, topic.Workspace, topic.Floor)
	if err != nil {
		return err
	}
	count := 0
	for _, item := range feed.Items {
		a := item.Terminal
		if a.ID == "" || a.FloorID != topic.Floor {
			continue
		}
		count++
		target := relayTarget{Workspace: topic.Workspace, Floor: topic.Floor, Thread: thread, Terminal: a.ID, Epoch: feed.Epoch, Prompt: item.Prompt, At: time.Now()}
		r.mu.Lock()
		err = r.sendTarget(ctx, target, fmt.Sprintf("[%s] %s\nResponda a esta mensagem para conversar com este agente.", a.Name, a.Status), []domain.Button{{Text: "Tela", Data: "screen", Row: 1}, {Text: "Foco", Data: "focus", Row: 1}}, false)
		r.mu.Unlock()
		if err != nil {
			return err
		}
	}
	if count == 0 {
		return r.say(ctx, thread, "Nenhum agente neste andar.")
	}
	return nil
}

func (r *Relay) button(ctx context.Context, e domain.ButtonPressed) error {
	r.mu.Lock()
	bound, ok := r.state.Replies[e.MessageID]
	r.mu.Unlock()
	allowed := false
	for _, action := range bound.Actions {
		if action == e.Data {
			allowed = true
		}
	}
	if !ok || !allowed || bound.Thread != e.ThreadID || time.Since(bound.At) > 10*time.Minute {
		return errors.New("botão expirado ou inválido; consulte /agents")
	}
	if e.Data == "cancel" {
		return r.Telegram.EditButtons(ctx, e.MessageID, nil)
	}
	lock, _ := r.locks.LoadOrStore(bound.Terminal, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	target, agent, err := r.target(ctx, e.ThreadID, e.MessageID, "")
	if err != nil {
		return err
	}
	switch e.Data {
	case "screen":
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.sendTarget(ctx, target, "["+agent.Name+"]\n"+strings.Join(agent.Preview, "\n"), nil, false)
	case "approve", "reject":
		if target.Prompt == "" || target.Prompt != bound.Prompt {
			return errors.New("a pergunta mudou; aguarde a notificação atual")
		}
	case "kill":
	case "focus":
	default:
		return errors.New("ação desconhecida")
	}
	r.mu.Lock()
	stored := r.state.Replies[e.MessageID]
	stored.Actions = nil
	r.state.Replies[e.MessageID] = stored
	err = r.save()
	r.mu.Unlock()
	if err != nil {
		return err
	}
	cap := ""
	if e.Data == "focus" {
		cap = "terminalFocus"
	}
	err = r.Wire.Call(ctx, cap, "POST", "/api/terminals/"+target.Terminal+"/"+e.Data, map[string]any{}, nil)
	if err == nil {
		_ = r.Telegram.EditButtons(ctx, e.MessageID, nil)
	}
	return err
}

func (r *Relay) attachment(ctx context.Context, e domain.TopicAttachment) error {
	target, _, err := r.target(ctx, e.ThreadID, e.ReplyTo, "")
	if err != nil {
		return err
	}
	lock, _ := r.locks.LoadOrStore(target.Terminal, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	target, _, err = r.target(ctx, e.ThreadID, e.ReplyTo, target.Terminal)
	if err != nil {
		return err
	}
	b, err := r.Telegram.Download(ctx, e.FileID, 8<<20)
	if err != nil {
		return err
	}
	name := e.Name
	if name == "" {
		name = string(e.Kind) + ".bin"
	}
	id, err := r.Wire.Upload(ctx, target.Terminal, name, e.MIME, b)
	if err != nil {
		return err
	}
	segments := []map[string]string{{"attachmentId": id}}
	if e.Caption != "" {
		segments = append([]map[string]string{{"text": e.Caption}}, segments...)
	}
	return r.Wire.Call(ctx, "promptSegments", "POST", "/api/terminals/"+target.Terminal+"/prompt", map[string]any{"segments": segments}, nil)
}

func (r *Relay) catalog(ctx context.Context, thread int, query string) error {
	if r.Catalog == nil {
		return errors.New("catálogo ausente; execute catalog-sync")
	}
	entries := r.Catalog.List()
	type match struct {
		a     domain.Arrangement
		score int
	}
	matches := []match{}
	words := strings.Fields(strings.ToLower(query))
	for _, a := range entries {
		score := 0
		hay := strings.ToLower(a.Name + " " + a.Description)
		for _, w := range words {
			if strings.Contains(hay, w) {
				score++
			}
		}
		if len(words) == 0 || score > 0 {
			matches = append(matches, match{a, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	lines := []string{fmt.Sprintf("Catálogo: %d entradas; %d resultados. Use /apply ID neste andar ou /adapt ID descrição.", len(entries), len(matches))}
	for i, m := range matches {
		if i >= 12 {
			lines = append(lines, "Refine a descrição para ver outros resultados.")
			break
		}
		lines = append(lines, m.a.Name+"\n"+m.a.ID)
	}
	return r.say(ctx, thread, strings.Join(lines, "\n\n"))
}

func (r *Relay) bind(ctx context.Context, args string) error {
	f := strings.Fields(args)
	if len(f) != 3 {
		return errors.New("use /bind workspace-id floor-id|ground topic-id")
	}
	thread, err := strconv.Atoi(f[2])
	if err != nil || thread <= 0 {
		return errors.New("topic-id inválido")
	}
	floor := f[1]
	if floor == "ground" {
		floor = ""
	}
	if !r.Config.Allows(f[0]) {
		return errors.New("workspace não autorizado")
	}
	feed, err := r.Wire.Feed(ctx, f[0], floor)
	if err != nil {
		return err
	}
	name := "Térreo"
	exists := floor == ""
	for _, v := range feed.Floors {
		if v.ID == floor {
			name = v.Name
			exists = true
		}
	}
	if !exists {
		return errors.New("andar inexistente")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, t := range r.state.Topics {
		if t.Thread == thread && k != scope(f[0], floor) {
			return errors.New("tópico já vinculado a outro andar")
		}
	}
	r.state.Topics[scope(f[0], floor)] = &relayTopic{Workspace: f[0], Floor: floor, Thread: thread, Name: feed.Workspace.Name + " · " + name}
	return r.save()
}

func (r *Relay) maestro(ctx context.Context, thread int, key, command, args string) error {
	base := domain.MaestroPlan{}
	r.mu.Lock()
	for _, t := range r.state.Topics {
		if t.Thread == thread && t.Workspace != "maestro" {
			base.WorkspaceID = t.Workspace
			base.FloorID = t.Floor
		}
	}
	r.mu.Unlock()
	if command == "/apply" || command == "/adapt" {
		if r.Catalog == nil {
			return errors.New("catálogo não instalado")
		}
		id, rest, _ := strings.Cut(args, " ")
		a, err := r.Catalog.Get(id)
		if err != nil {
			return err
		}
		base.Arrangement = a
		args = strings.TrimSpace(rest)
		if command == "/apply" && base.WorkspaceID == "" {
			return errors.New("use /apply no tópico Workspace · Andar desejado")
		}
	}
	plan := base
	if command != "/apply" {
		if strings.TrimSpace(args) == "" {
			return errors.New("descreva a equipe ou adaptação desejada")
		}
		if r.Planner == nil {
			return errors.New("planejador não configurado")
		}
		wss, err := r.Wire.Workspaces(ctx)
		if err != nil {
			return err
		}
		allowed := []domain.WireWorkspace{}
		for _, ws := range wss {
			if r.Config.Allows(ws.ID) && !ws.IsLocked {
				feed, e := r.Wire.Feed(ctx, ws.ID, "ground")
				if e != nil {
					return e
				}
				ws.Floors = feed.Floors
				allowed = append(allowed, ws)
			}
		}
		presets, err := r.Wire.Presets(ctx)
		if err != nil {
			return err
		}
		plan, err = r.Planner.Plan(ctx, args, base, allowed, presets)
		if err != nil {
			return err
		}
		if plan.Action == "answer" {
			return r.say(ctx, thread, plan.Message)
		}
		if base.WorkspaceID != "" && (plan.WorkspaceID != base.WorkspaceID || plan.FloorID != base.FloorID || plan.WorkspaceName != "" || plan.FloorName != "") {
			return errors.New("pedido mudou o destino do tópico; use Maestro para criar outro workspace/andar")
		}
	}
	if command == "/preview" || plan.Action == "preview" {
		b, _ := json.MarshalIndent(plan, "", "  ")
		return r.Telegram.SendDocument(ctx, domain.Document{ThreadID: thread, Name: "maestri-plan.json", Data: b, Caption: "Prévia; nada foi criado."})
	}
	jobID := strings.ReplaceAll(key, ":", "-")
	result, err := r.Deploy(ctx, jobID, plan)
	var required *ImportRequired
	if errors.As(err, &required) {
		b, _ := json.MarshalIndent(required.Native, "", "  ")
		if e := r.Telegram.SendDocument(ctx, domain.Document{ThreadID: thread, Name: "Relay.maestripartitura", Data: b, Caption: "Importe esta partitura no Maestri; o Wire não oferece importação da biblioteca. Depois use /resume " + jobID}); e != nil {
			return e
		}
	}
	if err != nil {
		return fmt.Errorf("criação %s: %w", jobID, err)
	}
	_ = r.Refresh(ctx)
	return r.say(ctx, thread, result)
}
