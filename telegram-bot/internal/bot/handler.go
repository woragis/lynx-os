package bot

import (
	"bytes"
	"context"
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
	LynxOSURL      string
	InternalToken  string
	TelegramAPIURL string
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.BotToken) == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is required")
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
		client = &http.Client{Timeout: 40 * time.Second}
	}
	if config.TelegramAPIURL == "" {
		config.TelegramAPIURL = "https://api.telegram.org"
	}
	return &Handler{config: config, client: client}
}

// RunPolling recebe atualizações diretamente do Telegram, sem expor uma porta
// pública. O offset só avança depois que o processamento no Lynx OS termina.
func (h *Handler) RunPolling(ctx context.Context) error {
	if err := h.callTelegram(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": false}, nil); err != nil {
		return fmt.Errorf("disable Telegram webhook: %w", err)
	}
	var offset int64
	for {
		var updates struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int64           `json:"update_id"`
				Message  json.RawMessage `json:"message"`
			} `json:"result"`
		}
		if err := h.callTelegram(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 25, "allowed_updates": []string{"message"}}, &updates); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("Telegram polling failed", "error", err)
			timer := time.NewTimer(3 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}
		if !updates.OK {
			slog.Error("Telegram getUpdates returned ok=false")
			continue
		}
		for _, update := range updates.Result {
			body, err := json.Marshal(map[string]any{"update_id": update.UpdateID, "message": update.Message})
			if err != nil {
				offset = update.UpdateID + 1
				continue
			}
			var message struct {
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
			}
			if len(update.Message) > 0 && json.Unmarshal(update.Message, &message) == nil && message.Chat.ID != 0 {
				reply, processErr := h.process(ctx, body)
				if processErr != nil {
					slog.Error("Lynx OS did not process Telegram update", "update_id", update.UpdateID, "error", processErr)
					break
				}
				if reply != nil && strings.TrimSpace(*reply) != "" {
					if err := h.sendMessage(ctx, message.Chat.ID, *reply); err != nil {
						slog.Error("Telegram response could not be delivered", "update_id", update.UpdateID, "error", err)
					}
				}
			}
			offset = update.UpdateID + 1
		}
	}
}

func (h *Handler) callTelegram(ctx context.Context, method string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(h.config.TelegramAPIURL, "/") + "/bot" + h.config.BotToken + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Telegram returned HTTP %d", response.StatusCode)
	}
	if result == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxUpdateBytes)).Decode(result); err != nil {
		return err
	}
	return nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	return mux
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
