package bot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxUpdateBytes = 64_000

type Config struct {
	BotToken       string
	WebhookSecret  string
	LynxOSURL      string
	InternalToken  string
	TelegramAPIURL string
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.BotToken) == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	if strings.TrimSpace(c.WebhookSecret) == "" {
		return errors.New("TELEGRAM_WEBHOOK_SECRET is required")
	}
	if len(c.WebhookSecret) > 256 {
		return errors.New("TELEGRAM_WEBHOOK_SECRET must not exceed 256 characters")
	}
	for _, char := range c.WebhookSecret {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '_' && char != '-' {
			return errors.New("TELEGRAM_WEBHOOK_SECRET contains unsupported characters")
		}
	}
	if len(c.InternalToken) < 32 {
		return errors.New("LYNX_TELEGRAM_INTERNAL_TOKEN must have at least 32 characters")
	}
	parsed, err := url.ParseRequestURI(c.LynxOSURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("LYNX_OS_INTERNAL_URL must be an absolute HTTP(S) URL")
	}
	return nil
}

type TelegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

type internalReply struct {
	Reply *string `json:"reply"`
}

type telegramResult struct {
	OK bool `json:"ok"`
}

type Handler struct {
	config Config
	client *http.Client
}

func NewHandler(config Config, client *http.Client) *Handler {
	if client == nil {
		client = &http.Client{Timeout: 50 * time.Second}
	}
	if config.TelegramAPIURL == "" {
		config.TelegramAPIURL = "https://api.telegram.org"
	}
	return &Handler{config: config, client: client}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /webhooks/telegram", h.webhook)
	return mux
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	if !sameSecret(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), h.config.WebhookSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]bool{"ok": false})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpdateBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]bool{"ok": false})
		return
	}
	var update TelegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]bool{"ok": false})
		return
	}
	if update.Message == nil || update.Message.Chat.ID == 0 {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	reply, err := h.process(r.Context(), body)
	if err != nil {
		slog.Error("Lynx OS did not process Telegram update", "update_id", update.UpdateID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]bool{"ok": false})
		return
	}
	if reply != nil && strings.TrimSpace(*reply) != "" {
		if err := h.sendMessage(r.Context(), update.Message.Chat.ID, *reply); err != nil {
			// O domínio já processou a mensagem; responder 200 evita executar o
			// mesmo comando novamente quando o Telegram retentar o webhook.
			slog.Error("Telegram response could not be delivered", "update_id", update.UpdateID, "error", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) process(ctx context.Context, body []byte) (*string, error) {
	endpoint := strings.TrimRight(h.config.LynxOSURL, "/") + "/api/internal/telegram/update"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Lynx OS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.config.InternalToken)
	response, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Lynx OS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Lynx OS returned HTTP %d", response.StatusCode)
	}
	var result internalReply
	if err := json.NewDecoder(io.LimitReader(response.Body, 16_384)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Lynx OS response: %w", err)
	}
	return result.Reply, nil
}

func (h *Handler) sendMessage(ctx context.Context, chatID int64, text string) error {
	payload, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return fmt.Errorf("encode Telegram message: %w", err)
	}
	endpoint := strings.TrimRight(h.config.TelegramAPIURL, "/") + "/bot" + h.config.BotToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create Telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("call Telegram: %w", err)
	}
	defer response.Body.Close()
	var result telegramResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 16_384)).Decode(&result); err != nil {
		return fmt.Errorf("decode Telegram response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !result.OK {
		return fmt.Errorf("Telegram returned HTTP %d", response.StatusCode)
	}
	return nil
}

func sameSecret(actual, expected string) bool {
	if actual == "" || expected == "" {
		return false
	}
	a := sha256.Sum256([]byte(actual))
	e := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(a[:], e[:]) == 1
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
