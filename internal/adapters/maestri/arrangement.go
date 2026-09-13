package maestri

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func ResolvePresets(a *domain.Arrangement, presets []domain.AgentPreset) error {
	a.Nodes = append([]domain.ArrangementNode(nil), a.Nodes...)
	for i, n := range a.Nodes {
		if n.Kind != "terminal" {
			continue
		}
		var candidates []domain.AgentPreset
		for _, p := range presets {
			if n.PresetID != "" {
				if p.ID == n.PresetID {
					candidates = append(candidates, p)
				}
			} else if p.AgentType == n.AgentType || (n.AgentType == "" && p.IsDefault) {
				candidates = append(candidates, p)
			}
		}
		if len(candidates) != 1 {
			return fmt.Errorf("selecione um preset instalado para %s; encontrados %d", n.Name, len(candidates))
		}
		a.Nodes[i].PresetID = candidates[0].ID
		a.Nodes[i].AgentType = candidates[0].AgentType
	}
	return nil
}

func NativePartitura(a domain.Arrangement, presets []domain.AgentPreset) (map[string]any, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if err := ResolvePresets(&a, presets); err != nil {
		return nil, err
	}
	commands := map[string]string{}
	for _, p := range presets {
		for _, n := range a.Nodes {
			if n.Kind == "terminal" && n.PresetID == p.ID {
				commands[p.ID] = p.Command
			}
		}
	}
	serialized, _ := json.Marshal(struct {
		Arrangement domain.Arrangement
		Commands    map[string]string
	}{a, commands})
	id := stableID(string(serialized))
	workspace := stableID("maestri-relay-export")
	stamp := "2026-09-13T00:00:00Z"
	roles := []any{}
	nodes := []any{}
	connections := []any{}
	noteConnections := []any{}
	portalConnections := []any{}
	noteTexts := map[string]string{}
	ids := map[string]string{}
	terms := map[string]string{}
	byID := map[string]domain.ArrangementNode{}
	for _, n := range a.Nodes {
		ids[n.ID] = stableID(id + "/node/" + n.ID)
		terms[n.ID] = stableID(id + "/terminal/" + n.ID)
		byID[n.ID] = n
	}
	for _, n := range a.Nodes {
		var content map[string]any
		switch n.Kind {
		case "terminal":
			command := commands[n.PresetID]
			if strings.TrimSpace(command) == "" {
				return nil, fmt.Errorf("preset %s não expõe comando para exportação nativa", n.PresetID)
			}
			role := stableID(id + "/role/" + n.ID)
			roles = append(roles, map[string]any{"id": role, "name": n.Name, "prompt": n.Prompt, "icon": "person.text.rectangle", "color": "#007AFF", "schemaVersion": 1})
			content = map[string]any{"terminal": map[string]any{"_0": map[string]any{"id": terms[n.ID], "name": n.Name, "agentType": n.AgentType, "command": command, "assignedRoleId": role, "isManager": n.Manager, "monitorWithOmbro": true, "status": "restored", "isUnloaded": false, "color": "#007AFF", "icon": "terminal", "workingDirectory": "", "shellPath": "", "scrollbackFile": "", "scrollbackLineCount": 0, "autoScrollLocked": false, "lastActiveAt": "1970-01-01T00:00:00Z", "shortcutMode": map[string]string{"kind": "automatic"}}}}
		case "note":
			content = map[string]any{"stickyNote": map[string]any{"_0": map[string]any{"fileName": n.Name, "color": "blue", "fontSize": 14, "hasCustomName": true, "isContentLocked": false, "isPreviewing": true, "storageMode": map[string]any{"managed": map[string]any{}}}}}
			noteTexts[ids[n.ID]] = n.Text
		case "portal":
			content = map[string]any{"portal": map[string]any{"_0": map[string]any{"id": stableID(id + "/portal/" + n.ID), "name": n.Name, "currentURL": n.URL, "source": map[string]any{"url": map[string]string{"_0": n.URL}}, "surface": map[string]any{"browser": map[string]any{}}, "status": "idle", "isUnloaded": false, "chromeHidden": false, "storageScope": "isolated"}}}
		}
		nodes = append(nodes, map[string]any{"id": ids[n.ID], "content": content, "frame": [][]float64{{n.X, n.Y}, {n.Width, n.Height}}, "createdAt": stamp, "lastModifiedAt": stamp, "isLocked": false, "zIndex": 50 + len(nodes)})
	}
	for _, c := range a.Connections {
		from, to := byID[c.From], byID[c.To]
		if from.Kind != "terminal" && to.Kind == "terminal" {
			from, to = to, from
		}
		points := make([][]float64, 21)
		for i := range points {
			fraction := float64(i) / 20
			points[i] = []float64{(from.X+from.Width/2)*(1-fraction) + (to.X+to.Width/2)*fraction, (from.Y+from.Height/2)*(1-fraction) + (to.Y+to.Height/2)*fraction}
		}
		entry := map[string]any{"id": stableID(id + "/edge/" + c.From + "/" + c.To), "createdAt": stamp, "ropePoints": points}
		if from.Kind != "terminal" {
			return nil, errors.New("exportação desta conexão entre nós ainda não suportada")
		}
		switch to.Kind {
		case "terminal":
			entry["terminalIdA"] = terms[from.ID]
			entry["terminalIdB"] = terms[to.ID]
			connections = append(connections, entry)
		case "note":
			entry["terminalId"] = terms[from.ID]
			entry["noteNodeId"] = ids[to.ID]
			noteConnections = append(noteConnections, entry)
		case "portal":
			entry["terminalId"] = terms[from.ID]
			entry["portalNodeId"] = ids[to.ID]
			portalConnections = append(portalConnections, entry)
		}
	}
	return map[string]any{"formatVersion": 1, "appVersion": "0.45.6", "id": id, "name": "Relay · " + a.Name, "description": a.Description, "createdAt": stamp, "icon": "paperplane", "color": "#007AFF", "workspaceId": workspace, "roles": roles, "payload": map[string]any{"nodes": nodes, "connections": connections, "noteConnections": noteConnections, "portalConnections": portalConnections, "noteTexts": noteTexts, "drawings": []any{}, "noteToNoteConnections": []any{}, "portalToPortalConnections": []any{}, "sourceWorkspaceId": workspace, "sourceWorkingDirectory": ""}}, nil
}
