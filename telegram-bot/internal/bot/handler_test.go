package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	config := Config{
		BotToken:      "123:token",
		WebhookSecret: "webhook-secret",
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

func TestWebhookForwardsToLynxOSAndSendsReply(t *testing.T) {
	var sentText string
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:token/sendMessage" {
			t.Errorf("unexpected Telegram API path: %s", r.URL.Path)
		}
		var message struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			t.Errorf("decode sent message: %v", err)
		}
		if message.ChatID != 42 {
			t.Errorf("unexpected chat id: %d", message.ChatID)
		}
		sentText = message.Text
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer telegram.Close()

	lynx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/internal/telegram/update" {
			t.Errorf("unexpected Lynx OS path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("k", 32) {
			t.Error("internal authorization was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"reply":"Lynx OS pronto"}`))
	}))
	defer lynx.Close()

	config := Config{
		BotToken:       "123:token",
		WebhookSecret:  "webhook-secret",
		LynxOSURL:      lynx.URL,
		InternalToken:  strings.Repeat("k", 32),
		TelegramAPIURL: telegram.URL,
	}
	handler := NewHandler(config, nil).Routes()
	request := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{"update_id":1,"message":{"text":"/start","chat":{"id":42}}}`))
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "webhook-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected webhook status: %d %s", response.Code, response.Body.String())
	}
	if sentText != "Lynx OS pronto" {
		t.Fatalf("unexpected Telegram reply: %q", sentText)
	}
}

func TestWebhookRejectsInvalidSecretAndOversizedBody(t *testing.T) {
	config := Config{BotToken: "token", WebhookSecret: "secret", LynxOSURL: "http://localhost:3000", InternalToken: strings.Repeat("k", 32)}
	handler := NewHandler(config, nil).Routes()

	request := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(fmt.Sprintf(`{"payload":%q}`, strings.Repeat("x", maxUpdateBytes))))
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected payload too large, got %d", response.Code)
	}
}
