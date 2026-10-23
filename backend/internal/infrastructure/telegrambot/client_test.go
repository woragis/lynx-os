package telegrambot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

func TestPollMapsTelegramMessageAndUsesOffset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getUpdates") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("offset") != "28" {
			t.Errorf("unexpected offset %s", r.URL.Query().Get("offset"))
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":27,"message":{"text":"/start","from":{"id":876},"chat":{"id":876,"type":"private"}}}]}`))
	}))
	defer server.Close()
	c := NewClient("test-token")
	c.api = server.URL
	updates, err := c.Poll(context.Background(), 28)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || updates[0].ID != 27 || updates[0].Message == nil || updates[0].Message.Text != "/start" || updates[0].Message.ChatType != "private" {
		t.Fatalf("unexpected mapped updates: %#v", updates)
	}
}

func TestSendRendersInlineKeyboard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["chat_id"].(float64) != 123 {
			t.Error("chat id missing")
		}
		markup := body["reply_markup"].(map[string]any)
		rows := markup["inline_keyboard"].([]any)
		button := rows[0].([]any)[0].(map[string]any)
		if button["callback_data"] != "menu:new" {
			t.Error("callback data missing")
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":9}}`))
	}))
	defer server.Close()
	c := NewClient("test-token")
	c.api = server.URL
	id, err := c.Send(context.Background(), &domain.Reply{ChatID: 123, Text: "Menu", Buttons: [][]domain.Button{{{Text: "Novo", Data: "menu:new"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if id != 9 {
		t.Fatalf("message id = %d", id)
	}
}
