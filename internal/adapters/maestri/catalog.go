package maestri

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

const GuideRevision = "24ccb073c761864d3352552053a38240f70224a1"
const GuideURL = "https://github.com/arthurspk/guiadomaestri"

type Catalog struct{ entries []domain.Arrangement }

func SyncCatalog(ctx context.Context, dir string) error {
	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://codeload.github.com/arthurspk/guiadomaestri/zip/"+GuideRevision, nil)
	if err != nil {
		return err
	}
	r, err := client.Do(req)
	if err != nil {
		return errors.New("download do guia falhou")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("download do guia: HTTP %d", r.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 64<<20+1))
	if err != nil {
		return err
	}
	if len(b) > 64<<20 {
		return errors.New("arquivo do guia excede 64 MiB")
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	destination := filepath.Join(dir, GuideRevision)
	if _, err = os.Stat(destination); err == nil {
		_, err = LoadCatalog(destination)
		return err
	}
	tmp, err := os.MkdirTemp(dir, ".download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var total uint64
	for _, f := range z.File {
		_, name, ok := strings.Cut(f.Name, "/")
		if !ok || name == "" || f.FileInfo().IsDir() {
			continue
		}
		if !fs.ValidPath(name) || strings.Contains(name, "\\") || f.Mode()&os.ModeSymlink != 0 {
			return errors.New("caminho inseguro no pacote do guia")
		}
		total += f.UncompressedSize64
		if total > 128<<20 || f.UncompressedSize64 > 16<<20 {
			return errors.New("conteúdo descompactado excede limite")
		}
		if !strings.HasSuffix(name, ".maestripartitura") && !isRecipe(name) {
			continue
		}
		in, e := f.Open()
		if e != nil {
			return e
		}
		data, e := io.ReadAll(io.LimitReader(in, 16<<20+1))
		in.Close()
		if e != nil {
			return e
		}
		if len(data) > 16<<20 {
			return errors.New("entrada grande demais")
		}
		path := filepath.Join(tmp, filepath.FromSlash(name))
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		if e = os.WriteFile(path, data, 0600); e != nil {
			return e
		}
	}
	if _, err = LoadCatalog(tmp); err != nil {
		return err
	}
	return os.Rename(tmp, destination)
}

func isRecipe(path string) bool {
	for _, prefix := range []string{"receitas/", "roles/", "notas/", "instrucoes/", "prompts/", "temas/", "workspaces/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func LoadCatalog(dir string) (*Catalog, error) {
	c := &Catalog{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("catálogo não aceita links simbólicos")
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".maestripartitura") && !isRecipe(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > 16<<20 {
			return fmt.Errorf("entrada grande demais: %s", rel)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var a domain.Arrangement
		if strings.HasSuffix(rel, ".maestripartitura") {
			a, err = ParsePartitura(b)
			if err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
		} else {
			name := strings.TrimSuffix(rel, filepath.Ext(rel))
			text := string(b)
			if len(text) > 100000 {
				return fmt.Errorf("receita grande demais: %s", rel)
			}
			a = domain.Arrangement{ID: stableID(rel), Name: name, Description: "Exemplo do guia como nota de referência; comandos não são executados.", Nodes: []domain.ArrangementNode{{ID: "reference", Kind: "note", Name: name, Text: text, X: 100, Y: 100, Width: 640, Height: 480}}}
		}
		a.Source = GuideURL + "/blob/" + GuideRevision + "/" + rel
		a.Revision = GuideRevision
		if err = a.Validate(); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		c.entries = append(c.entries, a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(c.entries) == 0 {
		return nil, errors.New("catálogo vazio; execute catalog-sync")
	}
	sort.Slice(c.entries, func(i, j int) bool { return c.entries[i].Name < c.entries[j].Name })
	ids := map[string]bool{}
	for _, a := range c.entries {
		if ids[a.ID] {
			return nil, fmt.Errorf("ID duplicado no catálogo: %s", a.ID)
		}
		ids[a.ID] = true
	}
	return c, nil
}

func (c *Catalog) List() []domain.Arrangement { return append([]domain.Arrangement(nil), c.entries...) }
func (c *Catalog) Get(id string) (domain.Arrangement, error) {
	for _, a := range c.entries {
		if a.ID == id {
			b, _ := json.Marshal(a)
			var copy domain.Arrangement
			_ = json.Unmarshal(b, &copy)
			return copy, nil
		}
	}
	return domain.Arrangement{}, errors.New("partitura não encontrada")
}

func ParsePartitura(b []byte) (domain.Arrangement, error) {
	var raw struct {
		FormatVersion int    `json:"formatVersion"`
		ID            string `json:"id"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		Roles         []struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
		} `json:"roles"`
		Payload struct {
			Nodes []struct {
				ID      string                     `json:"id"`
				Frame   [][]float64                `json:"frame"`
				Content map[string]json.RawMessage `json:"content"`
			} `json:"nodes"`
			NoteTexts         map[string]string            `json:"noteTexts"`
			Connections       []map[string]json.RawMessage `json:"connections"`
			NoteConnections   []map[string]json.RawMessage `json:"noteConnections"`
			PortalConnections []map[string]json.RawMessage `json:"portalConnections"`
			NoteToNote        []json.RawMessage            `json:"noteToNoteConnections"`
			PortalToPortal    []json.RawMessage            `json:"portalToPortalConnections"`
			Drawings          []json.RawMessage            `json:"drawings"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return domain.Arrangement{}, err
	}
	a := domain.Arrangement{ID: raw.ID, Name: raw.Name, Description: raw.Description}
	if raw.FormatVersion != 1 || len(raw.Payload.Drawings) > 0 || len(raw.Payload.NoteToNote) > 0 || len(raw.Payload.PortalToPortal) > 0 {
		return a, errors.New("formato ou componentes da partitura não suportados")
	}
	roles := map[string]string{}
	for _, r := range raw.Roles {
		roles[r.ID] = r.Prompt
	}
	termIDs := map[string]string{}
	for _, n := range raw.Payload.Nodes {
		if len(n.Content) != 1 || len(n.Frame) != 2 || len(n.Frame[0]) != 2 || len(n.Frame[1]) != 2 {
			return a, errors.New("nó com formato inválido")
		}
		d := domain.ArrangementNode{ID: n.ID, X: n.Frame[0][0], Y: n.Frame[0][1], Width: n.Frame[1][0], Height: n.Frame[1][1]}
		for kind, body := range n.Content {
			var wrapper struct {
				Value json.RawMessage `json:"_0"`
			}
			if err := json.Unmarshal(body, &wrapper); err != nil {
				return a, err
			}
			var v struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				AgentType  string `json:"agentType"`
				RoleID     string `json:"assignedRoleId"`
				Manager    bool   `json:"isManager"`
				FileName   string `json:"fileName"`
				CurrentURL string `json:"currentURL"`
			}
			if err := json.Unmarshal(wrapper.Value, &v); err != nil {
				return a, err
			}
			switch kind {
			case "terminal":
				d.Kind = "terminal"
				d.Name = v.Name
				d.AgentType = v.AgentType
				d.Manager = v.Manager
				d.Prompt = roles[v.RoleID]
				termIDs[v.ID] = n.ID
			case "stickyNote":
				d.Kind = "note"
				d.Name = v.FileName
				d.Text = raw.Payload.NoteTexts[n.ID]
			case "portal":
				d.Kind = "portal"
				d.Name = v.Name
				d.URL = v.CurrentURL
			default:
				return a, fmt.Errorf("nó sem suporte: %s", kind)
			}
		}
		a.Nodes = append(a.Nodes, d)
	}
	str := func(m map[string]json.RawMessage, key string) string {
		var s string
		_ = json.Unmarshal(m[key], &s)
		return s
	}
	for _, c := range raw.Payload.Connections {
		a.Connections = append(a.Connections, domain.ArrangementConnection{From: termIDs[str(c, "terminalIdA")], To: termIDs[str(c, "terminalIdB")]})
	}
	for _, c := range raw.Payload.NoteConnections {
		a.Connections = append(a.Connections, domain.ArrangementConnection{From: termIDs[str(c, "terminalId")], To: str(c, "noteNodeId")})
	}
	for _, c := range raw.Payload.PortalConnections {
		a.Connections = append(a.Connections, domain.ArrangementConnection{From: termIDs[str(c, "terminalId")], To: str(c, "portalNodeId")})
	}
	return a, a.Validate()
}

func stableID(s string) string {
	b := sha256.Sum256([]byte(s))
	b[6] = (b[6] & 15) | 80
	b[8] = (b[8] & 63) | 128
	h := strings.ToUpper(hex.EncodeToString(b[:16]))
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
