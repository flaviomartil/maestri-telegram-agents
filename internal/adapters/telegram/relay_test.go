package telegram_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/permgps/herdr-telegram-agents/internal/adapters/telegram"
	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func TestRelayConnectPreservesOfflineUpdatesAndInstallsOwnMenu(t *testing.T) {
	api := newFakeAPI(t)
	api.on("getMe", func(url.Values) apiReply {
		return okReply(map[string]any{"id": 42, "is_bot": true, "first_name": "Relay", "username": "relay_bot"})
	})
	api.on("getForumTopicIconStickers", func(url.Values) apiReply { return okReply([]any{}) })
	ctx, cancel := context.WithCancel(ctxT(t))
	defer cancel()
	_, _, err := telegram.ConnectRelay(ctx, domain.Config{BotToken: testToken, ChatID: testChatID, OperatorIDs: []int64{testOperator}}, nil, cancel, bot.WithServerURL(api.server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if v := api.callsOf("deleteWebhook")[0].form.Get("drop_pending_updates"); v == "true" {
		t.Fatal("discarded queued messages during restart")
	}
	menu := api.callsOf("setMyCommands")[0].form.Get("commands")
	if !strings.Contains(menu, "partituras") || strings.Contains(menu, "Herdr") {
		t.Fatal("wrong command menu")
	}
}
