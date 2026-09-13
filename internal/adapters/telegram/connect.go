package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

// Connect builds the daemon's Telegram side from the saved config: bot
// client, token check (dropping pending updates, since the daemon owns
// polling from here on), icon pack, command menu, serial queue and gateway. The returned
// run function polls and serves the queue until its context ends. fatal is
// invoked by the poller on 401/409. Extra opts are for tests.
func Connect(ctx context.Context, cfg domain.Config, log *slog.Logger, fatal context.CancelFunc, opts ...bot.Option) (*Gateway, func(context.Context), error) {
	return connect(ctx, cfg, log, fatal, false, opts...)
}

func ConnectRelay(ctx context.Context, cfg domain.Config, log *slog.Logger, fatal context.CancelFunc, opts ...bot.Option) (*Gateway, func(context.Context), error) {
	opts = append(opts, bot.WithNotAsyncHandlers())
	return connect(ctx, cfg, log, fatal, true, opts...)
}

func connect(ctx context.Context, cfg domain.Config, log *slog.Logger, fatal context.CancelFunc, relay bool, opts ...bot.Option) (*Gateway, func(context.Context), error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	api, err := NewBot(cfg.BotToken, log, fatal, opts...)
	if err != nil {
		return nil, nil, err
	}
	identity, err := check(ctx, api, log, !relay)
	if err != nil {
		return nil, nil, fmt.Errorf("telegram check: %w", err)
	}
	icons, err := LoadIcons(ctx, api, log)
	if err != nil {
		return nil, nil, fmt.Errorf("telegram icons: %w", err)
	}
	// A missing menu is a cosmetic loss; RegisterCommands already logged it.
	queueConfig := DefaultQueueConfig()
	if relay {
		queueConfig.MaxTries = 1
		commands := []models.BotCommand{
			{Command: "status", Description: "Workspaces, andares e conexões"},
			{Command: "agents", Description: "Escolher um agente deste andar"},
			{Command: "screen", Description: "Prévia do terminal escolhido"},
			{Command: "partituras", Description: "Buscar exemplos por descrição"},
			{Command: "apply", Description: "Aplicar exemplo pelo ID neste andar"},
			{Command: "adapt", Description: "Adaptar exemplo com IA"},
			{Command: "create", Description: "Criar equipe pela descrição"},
			{Command: "preview", Description: "Gerar plano sem executar"},
			{Command: "resume", Description: "Retomar criação registrada"},
			{Command: "focus", Description: "Focar agente no Maestri"},
			{Command: "stop", Description: "Enviar Escape ao agente"},
			{Command: "interrupt", Description: "Enviar Ctrl+C ao agente"},
			{Command: "close", Description: "Encerrar processo com confirmação"},
			{Command: "mute", Description: "Silenciar notificações deste andar"},
			{Command: "unmute", Description: "Ativar notificações deste andar"},
			{Command: "help", Description: "Ajuda do Maestri Relay"},
		}
		if _, err = api.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: commands, Scope: &models.BotCommandScopeChat{ChatID: cfg.ChatID}}); err != nil {
			return nil, nil, fmt.Errorf("menu Relay: %w", translate(err))
		}
	} else {
		_ = RegisterCommands(ctx, api, cfg.ChatID, log)
	}
	queue := NewQueue(log, queueConfig)
	gw := NewGateway(api, Config{ChatID: cfg.ChatID, Operators: cfg.OperatorIDs, Observers: cfg.ObserverIDs, Icons: icons, BotID: identity.ID, NoticeDelay: NoticeDelay}, queue, log)
	run := func(ctx context.Context) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			Poll(ctx, api, log)
		}()
		go func() {
			defer wg.Done()
			gw.Run(ctx)
		}()
		wg.Wait()
	}
	log.Info("telegram connected", slog.Int64("bot_id", identity.ID), slog.String("username", identity.Username), slog.Int64("chat_id", cfg.ChatID))
	return gw, run, nil
}
