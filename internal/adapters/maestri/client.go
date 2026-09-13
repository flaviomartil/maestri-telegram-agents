package maestri

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

type Client struct {
	base  string
	token string
	http  *http.Client
	mu    sync.RWMutex
	info  domain.WireInfo
}

func New(base, pin, token string) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("wireURL deve ser a origem HTTPS do host")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if pin != "" {
		normalized := strings.ReplaceAll(strings.TrimPrefix(pin, "sha256//"), ":", "")
		want, e := hex.DecodeString(normalized)
		if e != nil || len(want) != 32 {
			want, e = base64.StdEncoding.DecodeString(strings.TrimPrefix(pin, "sha256//"))
		}
		if e != nil || len(want) != 32 {
			return nil, errors.New("wirePin precisa ser SHA-256 SPKI em hex ou base64")
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
		transport.TLSClientConfig.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("certificado Wire ausente")
			}
			got := sha256.Sum256(cs.PeerCertificates[0].RawSubjectPublicKeyInfo)
			if subtle.ConstantTimeCompare(got[:], want) != 1 {
				return errors.New("chave de segurança Wire divergente")
			}
			return nil
		}
	}
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirecionamento Wire recusado") }}}, nil
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, mime string, out any) error {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return errors.New("rota Wire inválida")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if mime != "" {
		req.Header.Set("Content-Type", mime)
	}
	r, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("conexão Wire falhou; confira endereço, pareamento e chave de segurança")
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("erro Wire %s: HTTP %d", method, r.StatusCode)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
		return err
	}
	err = json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(out)
	if err != nil {
		return errors.New("resposta Wire inválida")
	}
	return nil
}

func (c *Client) Info(ctx context.Context) (domain.WireInfo, error) {
	var info domain.WireInfo
	err := c.request(ctx, "GET", "/api/info", nil, "", &info)
	if err == nil && info.ProtocolVersion != 1 {
		err = fmt.Errorf("protocolo Wire não suportado: %d", info.ProtocolVersion)
	}
	if err == nil {
		c.mu.Lock()
		c.info = info
		c.mu.Unlock()
	}
	return info, err
}

func (c *Client) Call(ctx context.Context, cap, method, path string, body, out any) error {
	c.mu.RLock()
	info := c.info
	c.mu.RUnlock()
	if info.ProtocolVersion != 1 {
		var err error
		info, err = c.Info(ctx)
		if err != nil {
			return err
		}
	}
	if !info.Has(cap) {
		return fmt.Errorf("host sem capability %s", cap)
	}
	if method != "GET" && info.Role != "owner" {
		return errors.New("operação exige pareamento Wire owner")
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	return c.request(ctx, method, path, bytes.NewReader(data), "application/json", out)
}

func (c *Client) Pair(ctx context.Context, code string) (string, error) {
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return "", errors.New("código precisa ter seis dígitos")
	}
	data, _ := json.Marshal(map[string]string{"deviceName": "Maestri Relay", "code": code})
	var result struct {
		Token           string `json:"token"`
		ProtocolVersion int    `json:"protocolVersion"`
	}
	if err := c.request(ctx, "POST", "/pair", bytes.NewReader(data), "application/json", &result); err != nil {
		return "", err
	}
	decoded, err := hex.DecodeString(result.Token)
	if err != nil || len(decoded) != 32 || result.ProtocolVersion != 1 {
		return "", errors.New("pareamento Wire inválido")
	}
	return result.Token, nil
}

func (c *Client) Workspaces(ctx context.Context) ([]domain.WireWorkspace, error) {
	var r struct {
		Workspaces []domain.WireWorkspace `json:"workspaces"`
	}
	err := c.Call(ctx, "feedSnapshots", "GET", "/api/workspaces", nil, &r)
	return r.Workspaces, err
}

func (c *Client) Feed(ctx context.Context, ws, floor string) (domain.WireFeed, error) {
	var r domain.WireFeed
	if !domain.ValidRelayID(ws) {
		return r, errors.New("workspace inválido")
	}
	if floor == "" {
		floor = "ground"
	}
	if !domain.ValidRelayID(floor) {
		return r, errors.New("andar inválido")
	}
	err := c.Call(ctx, "feedSnapshots", "GET", "/api/workspaces/"+ws+"/feed?floor="+floor, nil, &r)
	return r, err
}

func (c *Client) Presets(ctx context.Context) ([]domain.AgentPreset, error) {
	var r struct {
		Presets []domain.AgentPreset `json:"presets"`
	}
	err := c.Call(ctx, "canvasWrites", "GET", "/api/agent-presets", nil, &r)
	return r.Presets, err
}

func (c *Client) socket(ctx context.Context, id string) (*websocket.Conn, error) {
	if !domain.ValidRelayID(id) {
		return nil, errors.New("terminal inválido")
	}
	conn, _, err := websocket.Dial(ctx, strings.Replace(c.base, "https://", "wss://", 1)+"/api/terminals/"+id+"/stream", &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: http.Header{"Authorization": []string{"Bearer " + c.token}}})
	if err != nil {
		return nil, errors.New("stream Wire indisponível")
	}
	conn.SetReadLimit(1 << 20)
	return conn, nil
}

func (c *Client) Input(ctx context.Context, id string, data []byte) error {
	info, err := c.Info(ctx)
	if err != nil {
		return err
	}
	if info.Role != "owner" || !info.Has("terminalInputBytes") {
		return errors.New("entrada de teclas não autorizada pelo Wire")
	}
	s, err := c.socket(ctx, id)
	if err != nil {
		return err
	}
	defer s.CloseNow()
	b, _ := json.Marshal(map[string]string{"type": "inputBytes", "data": base64.StdEncoding.EncodeToString(data)})
	return s.Write(ctx, websocket.MessageText, b)
}

func (c *Client) Screen(ctx context.Context, id string) (string, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return "", err
	}
	if !info.Has("terminalStreaming") {
		return "", errors.New("host sem terminalStreaming")
	}
	s, err := c.socket(ctx, id)
	if err != nil {
		return "", err
	}
	defer s.CloseNow()
	for {
		kind, b, e := s.Read(ctx)
		if e != nil {
			return "", e
		}
		if kind == websocket.MessageBinary {
			return string(b), nil
		}
	}
}

func (c *Client) Upload(ctx context.Context, id, name, mime string, data []byte) (string, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return "", err
	}
	if !info.Has("attachmentStaging") || !info.Has("promptSegments") || info.Role != "owner" {
		return "", errors.New("host não permite anexos")
	}
	if !domain.ValidRelayID(id) || len(data) > 8<<20 {
		return "", errors.New("anexo inválido ou maior que 8 MiB")
	}
	var r struct {
		ID string `json:"attachmentId"`
	}
	err = c.request(ctx, "POST", "/api/terminals/"+id+"/attachments?filename="+url.QueryEscape(name), bytes.NewReader(data), mime, &r)
	return r.ID, err
}
