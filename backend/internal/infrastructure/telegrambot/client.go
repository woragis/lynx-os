package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

type Client struct {
	token string
	http  *http.Client
	api   string
}

func NewClient(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 35 * time.Second}, api: "https://api.telegram.org"}
}

func (c *Client) Poll(ctx context.Context, offset int64) ([]domain.Update, error) {
	values := url.Values{"timeout": {"25"}, "allowed_updates": {`["message","callback_query"]`}}
	if offset > 0 {
		values.Set("offset", strconv.FormatInt(offset, 10))
	}
	var result []struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			Text string `json:"text"`
			From struct {
				ID int64 `json:"id"`
			} `json:"from"`
			Chat struct {
				ID   int64  `json:"id"`
				Type string `json:"type"`
			} `json:"chat"`
		} `json:"message"`
		Callback *struct {
			ID   string `json:"id"`
			Data string `json:"data"`
			From struct {
				ID int64 `json:"id"`
			} `json:"from"`
			Message struct {
				Chat struct {
					ID   int64  `json:"id"`
					Type string `json:"type"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"callback_query"`
	}
	if err := c.call(ctx, "getUpdates", values, nil, &result); err != nil {
		return nil, err
	}
	updates := make([]domain.Update, 0, len(result))
	for _, item := range result {
		u := domain.Update{ID: item.UpdateID}
		if item.Message != nil {
			u.Message = &domain.Message{Text: item.Message.Text, FromID: item.Message.From.ID, ChatID: item.Message.Chat.ID, ChatType: item.Message.Chat.Type}
		}
		if item.Callback != nil {
			u.Callback = &domain.Callback{ID: item.Callback.ID, Data: item.Callback.Data, FromID: item.Callback.From.ID, ChatID: item.Callback.Message.Chat.ID, ChatType: item.Callback.Message.Chat.Type}
		}
		updates = append(updates, u)
	}
	return updates, nil
}

func (c *Client) Send(ctx context.Context, reply *domain.Reply) (int64, error) {
	if reply == nil {
		return 0, nil
	}
	markup := map[string]any{"inline_keyboard": [][]map[string]string{}}
	for _, row := range reply.Buttons {
		buttons := []map[string]string{}
		for _, b := range row {
			buttons = append(buttons, map[string]string{"text": b.Text, "callback_data": b.Data})
		}
		markup["inline_keyboard"] = append(markup["inline_keyboard"].([][]map[string]string), buttons)
	}
	if reply.MessageID != nil {
		body := map[string]any{"chat_id": reply.ChatID, "message_id": *reply.MessageID, "text": reply.Text, "reply_markup": markup}
		var ignored any
		err := c.call(ctx, "editMessageText", nil, body, &ignored)
		if err == nil {
			return *reply.MessageID, nil
		}
	}
	body := map[string]any{"chat_id": reply.ChatID, "text": reply.Text, "reply_markup": markup}
	var result struct {
		MessageID int64 `json:"message_id"`
	}
	if err := c.call(ctx, "sendMessage", nil, body, &result); err != nil {
		return 0, err
	}
	return result.MessageID, nil
}
func (c *Client) AnswerCallback(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	body := map[string]string{"callback_query_id": id}
	var ignored any
	return c.call(ctx, "answerCallbackQuery", nil, body, &ignored)
}
func (c *Client) DeleteWebhook(ctx context.Context) error {
	var ignored any
	return c.call(ctx, "deleteWebhook", url.Values{"drop_pending_updates": {"false"}}, nil, &ignored)
}
func (c *Client) call(ctx context.Context, method string, query url.Values, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	endpoint := c.api + "/bot" + c.token + "/" + method
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&envelope); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.OK {
		return fmt.Errorf("telegram API %s failed (status %d): %s", method, resp.StatusCode, strings.TrimSpace(envelope.Description))
	}
	if out != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}
