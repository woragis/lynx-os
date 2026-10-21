package bot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	config := Config{
		BotToken:      "123:token",
		LynxOSURL:     "http://localhost:3000",
		InternalToken: strings.Repeat("x", 32),
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("valid configuration rejected: %v", err)
	}
	config.LynxOSURL = "file:///etc/passwd"
	if err := config.Validate(); err == nil {
		t.Fatal("non-HTTP backend URL accepted")
	}
}

func TestRunPollingForwardsUpdatesWithoutWebhook(t *testing.T) {
	var polls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var lynx *httptest.Server
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bot123:token/deleteWebhook":
			w.Write([]byte(`{"ok":true,"result":true}`))
		case "/bot123:token/getUpdates":
			if polls.Add(1) == 1 {
				w.Write([]byte(`{"ok":true,"result":[{"update_id":8,"message":{"text":"/start","from":{"id":42},"chat":{"id":42,"type":"private"}}}]}`))
			} else {
				cancel()
				w.Write([]byte(`{"ok":true,"result":[]}`))
			}
		case "/bot123:token/sendMessage":
			w.Write([]byte(`{"ok":true}`))
		default:
			t.Errorf("unexpected Telegram API path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer telegram.Close()
	lynx = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var update map[string]any
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			t.Errorf("decode update: %v", err)
		}
		message := update["message"].(map[string]any)
		if message["text"] != "/start" {
			t.Errorf("message text lost: %#v", message)
		}
		w.Write([]byte(`{"reply":"Lynx OS pronto"}`))
	}))
	defer lynx.Close()
	config := Config{BotToken: "123:token", LynxOSURL: lynx.URL, InternalToken: strings.Repeat("x", 32), TelegramAPIURL: telegram.URL}
	err := NewHandler(config, nil).RunPolling(ctx)
	if err != nil {
		t.Fatalf("polling returned error: %v", err)
	}
	if polls.Load() < 2 {
		t.Fatalf("expected update polling, got %d calls", polls.Load())
	}
}
