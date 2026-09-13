package compose

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/permgps/herdr-telegram-agents/internal/adapters/maestri"
	"github.com/permgps/herdr-telegram-agents/internal/adapters/state"
	"github.com/permgps/herdr-telegram-agents/internal/adapters/system"
	"github.com/permgps/herdr-telegram-agents/internal/adapters/telegram"
	"github.com/permgps/herdr-telegram-agents/internal/app"
	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func RelayWire(cfg domain.RelayConfig) (*maestri.Client, error) {
	return maestri.New(cfg.WireURL, cfg.WirePin, cfg.WireToken)
}

func RelayCatalog(cfg domain.RelayConfig) (*maestri.Catalog, error) {
	return maestri.LoadCatalog(filepath.Join(cfg.CatalogDir, maestri.GuideRevision))
}

func RelaySync(ctx context.Context, cfg domain.RelayConfig) error {
	return maestri.SyncCatalog(ctx, cfg.CatalogDir)
}

func RelayWrite(path string, data []byte) error { return state.SaveRelayFile(path, data) }

func RelayExport(ctx context.Context, cfg domain.RelayConfig) ([]byte, error) {
	c, err := RelayCatalog(cfg)
	if err != nil {
		return nil, err
	}
	wire, err := RelayWire(cfg)
	if err != nil {
		return nil, err
	}
	presets, err := wire.Presets(ctx)
	if err != nil {
		return nil, err
	}
	all := []any{}
	for _, a := range c.List() {
		native, e := maestri.NativePartitura(a, presets)
		if e != nil {
			return nil, e
		}
		all = append(all, native)
	}
	return json.MarshalIndent(map[string]any{"formatVersion": 1, "partituras": all}, "", "  ")
}

func RelayPlan(ctx context.Context, cfg domain.RelayConfig, request string, base domain.MaestroPlan) (domain.MaestroPlan, error) {
	wire, err := RelayWire(cfg)
	if err != nil {
		return base, err
	}
	workspaces, err := wire.Workspaces(ctx)
	if err != nil {
		return base, err
	}
	presets, err := wire.Presets(ctx)
	if err != nil {
		return base, err
	}
	allowed := []domain.WireWorkspace{}
	for _, ws := range workspaces {
		if cfg.Allows(ws.ID) && !ws.IsLocked {
			feed, e := wire.Feed(ctx, ws.ID, "ground")
			if e != nil {
				return base, e
			}
			ws.Floors = feed.Floors
			allowed = append(allowed, ws)
		}
	}
	planner := &maestri.Planner{URL: cfg.LLMURL, Model: cfg.LLMModel, Key: cfg.LLMKey}
	return planner.Plan(ctx, request, base, allowed, presets)
}

func relayApp(cfg domain.RelayConfig, wire domain.WireGateway, tg domain.TelegramGateway) (*app.Relay, error) {
	store, err := state.NewRelay(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	relay, err := app.NewRelay(cfg, wire, tg, store)
	if err != nil {
		return nil, err
	}
	if _, e := os.Stat(filepath.Join(cfg.CatalogDir, maestri.GuideRevision)); e == nil {
		relay.Catalog, err = RelayCatalog(cfg)
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	relay.Planner = &maestri.Planner{URL: cfg.LLMURL, Key: cfg.LLMKey, Model: cfg.LLMModel}
	relay.Native = maestri.NativePartitura
	return relay, nil
}

func RelayApply(ctx context.Context, cfg domain.RelayConfig, id string, plan domain.MaestroPlan) (string, error) {
	release, err := relayLock(cfg)
	if err != nil {
		return "", err
	}
	defer release()
	wire, err := RelayWire(cfg)
	if err != nil {
		return "", err
	}
	relay, err := relayApp(cfg, wire, nil)
	if err != nil {
		return "", err
	}
	result, err := relay.Deploy(ctx, id, plan)
	var required *app.ImportRequired
	if errors.As(err, &required) {
		b, _ := json.MarshalIndent(required.Native, "", "  ")
		path := filepath.Join(cfg.StateDir, id+".maestripartitura")
		if e := RelayWrite(path, b); e != nil {
			return "", e
		}
		return path, err
	}
	return result, err
}

func relayLock(cfg domain.RelayConfig) (func(), error) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	proc := system.NewProcess(cfg.StateDir, log)
	pid := state.NewPidFile(cfg.StateDir, proc.Alive, log)
	if err := pid.Acquire(os.Getpid()); err != nil {
		return nil, err
	}
	return func() { _ = pid.Release() }, nil
}

func RunRelay(ctx context.Context, cfg domain.RelayConfig, log *slog.Logger) error {
	release, err := relayLock(cfg)
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wire, err := RelayWire(cfg)
	if err != nil {
		return err
	}
	if _, err = wire.Info(ctx); err != nil {
		return err
	}
	tg, run, err := telegram.ConnectRelay(ctx, domain.Config{BotToken: cfg.BotToken, ChatID: cfg.ChatID, OperatorIDs: cfg.Operators, ObserverIDs: cfg.Observers}, log, cancel)
	if err != nil {
		return err
	}
	relay, err := relayApp(cfg, wire, tg)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); run(ctx) }()
	err = relay.Run(ctx)
	cancel()
	wg.Wait()
	return err
}
