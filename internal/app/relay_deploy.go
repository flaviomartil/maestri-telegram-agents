package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

type ImportRequired struct{ Native map[string]any }

func (*ImportRequired) Error() string {
	return "partitura com portais precisa ser importada na biblioteca do host antes da aplicação automática"
}

func (r *Relay) step(ctx context.Context, jobID, step, cap, method, path string, body any) (json.RawMessage, error) {
	r.mu.Lock()
	job := r.state.Jobs[jobID]
	if result, ok := job.Results[step]; ok {
		r.mu.Unlock()
		return result, nil
	}
	if job.Pending != "" {
		pending := job.Pending
		r.mu.Unlock()
		return nil, fmt.Errorf("etapa %s teve resultado incerto; reconcilie os recursos no host antes de retomar", pending)
	}
	job.Pending = step
	err := r.save()
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	var result json.RawMessage
	if err = r.Wire.Call(ctx, cap, method, path, body, &result); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job.Results[step] = result
	job.Pending = ""
	return result, r.save()
}

func resultID(raw json.RawMessage, key string) (string, error) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	var id string
	if err := json.Unmarshal(value[key], &id); err != nil || !domain.ValidRelayID(id) {
		return "", fmt.Errorf("resposta Wire não contém %s válido", key)
	}
	return id, nil
}

func (r *Relay) Deploy(ctx context.Context, jobID string, plan domain.MaestroPlan) (string, error) {
	r.provision.Lock()
	defer r.provision.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if plan.Action != "" && plan.Action != "create" {
		return "", errors.New("somente planos com action create podem ser aplicados")
	}
	if !domain.ValidRelayID(jobID) {
		return "", errors.New("ID de criação inválido")
	}
	if err := plan.Arrangement.Validate(); err != nil {
		return "", err
	}
	if plan.WorkspaceID == "" {
		if strings.TrimSpace(plan.WorkspaceName) == "" || strings.TrimSpace(plan.Directory) == "" {
			return "", errors.New("novo workspace precisa de nome e diretório explícitos")
		}
		if !r.Config.Allows("*") {
			return "", errors.New("configuração não autoriza criar novos workspaces")
		}
	} else if !domain.ValidRelayID(plan.WorkspaceID) || !r.Config.Allows(plan.WorkspaceID) {
		return "", errors.New("workspace não autorizado")
	}
	if plan.FloorID != "" && !domain.ValidRelayID(plan.FloorID) {
		return "", errors.New("andar inválido")
	}
	if plan.FloorID != "" && plan.FloorName != "" {
		return "", errors.New("escolha andar existente ou nome para um novo")
	}
	info, err := r.Wire.Info(ctx)
	if err != nil {
		return "", err
	}
	if info.Role != "owner" {
		return "", errors.New("criação exige pareamento owner")
	}
	for _, cap := range []string{"canvasWrites", "feedSnapshots"} {
		if !info.Has(cap) {
			return "", fmt.Errorf("host sem %s", cap)
		}
	}
	if plan.WorkspaceID == "" && !info.Has("workspaceManagement") {
		return "", errors.New("host sem workspaceManagement")
	}
	if plan.FloorName != "" && !info.Has("floorManagement") {
		return "", errors.New("host sem floorManagement")
	}
	for _, node := range plan.Arrangement.Nodes {
		if node.Kind == "terminal" && (!info.Has("roleManagement") || !info.Has("terminalDrafts")) {
			return "", errors.New("host precisa de roleManagement e terminalDrafts")
		}
	}
	if plan.WorkspaceID != "" {
		feed, e := r.Wire.Feed(ctx, plan.WorkspaceID, plan.FloorID)
		if e != nil {
			return "", e
		}
		found := plan.FloorID == ""
		for _, f := range feed.Floors {
			if f.ID == plan.FloorID && !f.IsCloneMissing {
				found = true
			}
		}
		if !found {
			return "", errors.New("andar de destino inexistente ou sem checkout")
		}
	}
	r.mu.Lock()
	job, exists := r.state.Jobs[jobID]
	if exists && digest(job.Plan) != digest(plan) {
		r.mu.Unlock()
		return "", errors.New("ID de criação já usado por outro plano")
	}
	if !exists {
		job = &relayJob{Plan: plan, Results: map[string]json.RawMessage{}}
		r.state.Jobs[jobID] = job
	}
	if job.Done {
		r.mu.Unlock()
		return "Criação " + jobID + " já concluída; nada foi repetido.", nil
	}
	err = r.save()
	r.mu.Unlock()
	if err != nil {
		return "", err
	}
	plan.Arrangement.Nodes = append([]domain.ArrangementNode(nil), plan.Arrangement.Nodes...)
	presets, err := r.Wire.Presets(ctx)
	if err != nil {
		return "", err
	}
	byID := map[string]domain.AgentPreset{}
	for _, p := range presets {
		byID[p.ID] = p
	}
	for i, n := range plan.Arrangement.Nodes {
		if n.Kind != "terminal" {
			continue
		}
		if n.PresetID == "" {
			matches := []domain.AgentPreset{}
			for _, p := range presets {
				if p.AgentType == n.AgentType || (n.AgentType == "" && p.IsDefault) {
					matches = append(matches, p)
				}
			}
			if len(matches) != 1 {
				return "", fmt.Errorf("selecione preset instalado para %s; correspondências: %d", n.Name, len(matches))
			}
			plan.Arrangement.Nodes[i].PresetID = matches[0].ID
		} else if _, ok := byID[n.PresetID]; !ok {
			return "", fmt.Errorf("preset inexistente: %s", n.PresetID)
		}
	}
	hasPortal := false
	for _, n := range plan.Arrangement.Nodes {
		hasPortal = hasPortal || n.Kind == "portal"
	}
	nativeID := ""
	if hasPortal {
		if r.Native == nil {
			return "", errors.New("exportador de partituras não configurado")
		}
		native, e := r.Native(plan.Arrangement, presets)
		if e != nil {
			return "", e
		}
		nativeID, _ = native["id"].(string)
		if !info.Has("partituras") {
			return "", &ImportRequired{Native: native}
		}
		var installed struct {
			Partituras []struct {
				ID string `json:"id"`
			} `json:"partituras"`
		}
		if e = r.Wire.Call(ctx, "partituras", "GET", "/api/partituras", nil, &installed); e != nil {
			return "", e
		}
		found := false
		for _, p := range installed.Partituras {
			if strings.EqualFold(p.ID, nativeID) {
				found = true
			}
		}
		if !found {
			return "", &ImportRequired{Native: native}
		}
	}
	ws, floor := plan.WorkspaceID, plan.FloorID
	if ws == "" {
		result, e := r.step(ctx, jobID, "workspace", "workspaceManagement", "POST", "/api/workspaces", map[string]any{"name": plan.WorkspaceName, "workingDirectory": plan.Directory, "createDirectory": true})
		if e != nil {
			return "", e
		}
		ws, e = resultID(result, "workspaceId")
		if e != nil {
			return "", e
		}
	}
	if plan.FloorName != "" {
		result, e := r.step(ctx, jobID, "floor", "floorManagement", "POST", "/api/workspaces/"+ws+"/floors", map[string]any{"name": plan.FloorName, "gitIsolation": false})
		if e != nil {
			return "", e
		}
		floor, e = resultID(result, "floorId")
		if e != nil {
			return "", e
		}
		for {
			feed, e := r.Wire.Feed(ctx, ws, "ground")
			if e != nil {
				return "", e
			}
			ready := false
			for _, f := range feed.Floors {
				if f.ID == floor {
					ready = true
				}
			}
			if ready {
				break
			}
			for _, p := range feed.PendingFloors {
				if p.ID == floor && p.Error != "" {
					return "", errors.New("criação do andar falhou no host")
				}
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	base := "/api/workspaces/" + ws
	withFloor := func(body map[string]any) map[string]any {
		if floor != "" {
			body["floorId"] = floor
		}
		return body
	}
	if nativeID != "" {
		r.mu.Lock()
		beforeRaw, hasBefore := job.Results["native-before"]
		r.mu.Unlock()
		if !hasBefore {
			before, e := r.Wire.Feed(ctx, ws, floor)
			if e != nil {
				return "", e
			}
			if _, e = relayCanvas(before.Canvas); e != nil {
				return "", e
			}
			beforeRaw = append(json.RawMessage(nil), before.Canvas...)
			r.mu.Lock()
			job.Results["native-before"] = beforeRaw
			e = r.save()
			r.mu.Unlock()
			if e != nil {
				return "", e
			}
		}
		_, err = r.step(ctx, jobID, "partitura", "partituras", "POST", base+"/partituras", withFloor(map[string]any{"partituraId": nativeID, "x": 1000, "y": 1000, "mutationId": jobID + "-partitura"}))
		if err != nil {
			return "", err
		}
		after, e := r.Wire.Feed(ctx, ws, floor)
		if e != nil {
			return "", e
		}
		beforeNodes, e := relayCanvas(beforeRaw)
		if e != nil {
			return "", e
		}
		afterNodes, e := relayCanvas(after.Canvas)
		if e != nil {
			return "", e
		}
		counts := map[string]int{}
		for id, kind := range afterNodes {
			if _, existed := beforeNodes[id]; !existed {
				counts[kind]++
			}
		}
		for _, n := range plan.Arrangement.Nodes {
			counts[n.Kind]--
		}
		for _, count := range counts {
			if count < 0 {
				return "", errors.New("a partitura ainda não foi confirmada no canvas; retome com /resume " + jobID)
			}
		}
	} else {
		ids := map[string]string{}
		for _, n := range plan.Arrangement.Nodes {
			body := withFloor(map[string]any{"x": n.X, "y": n.Y, "mutationId": jobID + "-" + n.ID})
			path := base + "/nodes"
			if n.Kind == "terminal" {
				role, e := r.step(ctx, jobID, "role-"+n.ID, "roleManagement", "POST", "/api/roles", map[string]any{"name": n.Name + " · " + jobID, "prompt": n.Prompt, "workspaceId": ws, "color": "#007AFF", "icon": "person.text.rectangle"})
				if e != nil {
					return "", e
				}
				roleID, e := resultID(role, "id")
				if e != nil {
					return "", e
				}
				body["presetId"] = n.PresetID
				body["name"] = n.Name
				body["roleId"] = roleID
				body["maestroMode"] = n.Manager
				body["monitorActivity"] = true
				path = base + "/terminals"
			} else {
				body["kind"] = "note"
				body["text"] = n.Text
			}
			result, e := r.step(ctx, jobID, "node-"+n.ID, "canvasWrites", "POST", path, body)
			if e != nil {
				return "", e
			}
			id, e := resultID(result, "nodeId")
			if e != nil {
				return "", e
			}
			ids[n.ID] = id
			_, e = r.step(ctx, jobID, "frame-"+n.ID, "canvasWrites", "POST", base+"/nodes/"+id+"/frame", map[string]any{"x": n.X, "y": n.Y, "width": n.Width, "height": n.Height, "mutationId": jobID + "-frame-" + n.ID})
			if e != nil {
				return "", e
			}
		}
		for i, c := range plan.Arrangement.Connections {
			step := fmt.Sprintf("edge-%d", i)
			_, e := r.step(ctx, jobID, step, "canvasWrites", "POST", base+"/connections", map[string]any{"fromNodeId": ids[c.From], "toNodeId": ids[c.To], "mutationId": jobID + "-" + step})
			if e != nil {
				return "", e
			}
		}
		feed, e := r.Wire.Feed(ctx, ws, floor)
		if e != nil {
			return "", e
		}
		var canvas struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		}
		if e = json.Unmarshal(feed.Canvas, &canvas); e != nil {
			return "", errors.New("não foi possível verificar os nós criados")
		}
		found := map[string]bool{}
		for _, n := range canvas.Nodes {
			found[n.ID] = true
		}
		for _, id := range ids {
			if !found[id] {
				return "", errors.New("host ainda não confirmou todos os nós; retome com /resume " + jobID)
			}
		}
	}
	r.mu.Lock()
	job.Done = true
	err = r.save()
	r.mu.Unlock()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Equipe %s criada. Workspace %s, andar %s. Criação %s.", plan.Arrangement.Name, ws, floor, jobID), nil
}

func relayCanvas(raw json.RawMessage) (map[string]string, error) {
	var canvas struct {
		Nodes []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &canvas); err != nil {
		return nil, errors.New("canvas inválido; criação não verificada")
	}
	result := map[string]string{}
	for _, n := range canvas.Nodes {
		if n.ID == "" {
			return nil, errors.New("canvas contém nó sem ID")
		}
		result[n.ID] = n.Kind
	}
	return result, nil
}
