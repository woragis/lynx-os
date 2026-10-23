package telegram

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

type BotClient interface {
	Poll(context.Context, int64) ([]domain.Update, error)
	Send(context.Context, *domain.Reply) (int64, error)
	AnswerCallback(context.Context, string) error
	DeleteWebhook(context.Context) error
}

type Worker struct {
	client  BotClient
	service *Service
	leader  domain.Leader
	logger  *slog.Logger
}

func NewWorker(client BotClient, service *Service, leader domain.Leader, logger *slog.Logger) *Worker {
	return &Worker{client: client, service: service, leader: leader, logger: logger}
}

func (w *Worker) Run(ctx context.Context) {
	for ctx.Err() == nil {
		release, acquired, err := w.leader.TryAcquireLeader(ctx)
		if err != nil {
			w.logger.Error("telegram leader lock failed", "error", err)
			if !wait(ctx, 5*time.Second) {
				return
			}
			continue
		}
		if !acquired {
			if !wait(ctx, 5*time.Second) {
				return
			}
			continue
		}
		w.runLeader(ctx)
		release()
		if ctx.Err() == nil && !wait(ctx, 2*time.Second) {
			return
		}
	}
}

func (w *Worker) runLeader(ctx context.Context) {
	if err := w.client.DeleteWebhook(ctx); err != nil {
		w.logger.Error("telegram webhook cleanup failed", "error", err)
		return
	}
	for ctx.Err() == nil {
		offset, err := w.service.UpdateOffset(ctx)
		if err != nil {
			w.logger.Error("telegram offset lookup failed", "error", err)
			return
		}
		updates, err := w.client.Poll(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.logger.Error("telegram polling failed", "error", err)
			if !wait(ctx, 3*time.Second) {
				return
			}
			continue
		}
		for _, update := range updates {
			if ctx.Err() != nil {
				return
			}
			w.process(ctx, update)
		}
	}
}

func (w *Worker) process(ctx context.Context, update domain.Update) {
	chatID := int64(0)
	telegramID := ""
	callbackID := ""
	if update.Message != nil {
		chatID = update.Message.ChatID
		telegramID = strconv.FormatInt(update.Message.FromID, 10)
	}
	if update.Callback != nil {
		chatID = update.Callback.ChatID
		telegramID = strconv.FormatInt(update.Callback.FromID, 10)
		callbackID = update.Callback.ID
	}
	created, err := w.service.BeginUpdate(ctx, update.ID, strconv.FormatInt(chatID, 10))
	if err != nil {
		w.logger.Error("telegram update claim failed", "error", err)
		return
	}
	if !created {
		return
	}
	if callbackID != "" {
		if err := w.client.AnswerCallback(ctx, callbackID); err != nil {
			w.logger.Warn("telegram callback acknowledgement failed", "error", err)
		}
	}
	reply, handleErr := w.service.HandleUpdate(ctx, update)
	ignored := reply == nil
	if handleErr != nil {
		w.logger.Error("telegram update processing failed", "update_id", update.ID, "error", handleErr)
		reply = &domain.Reply{ChatID: chatID, Text: "Não consegui concluir essa ação agora. Tente novamente em instantes."}
		ignored = false
	}
	if reply != nil {
		if conversation, _, getErr := w.service.conversations.GetConversation(ctx, telegramID); getErr == nil {
			reply.MessageID = conversation.BotMessageID
		}
		messageID, sendErr := w.client.Send(ctx, reply)
		if sendErr != nil {
			w.logger.Error("telegram reply failed", "update_id", update.ID, "error", sendErr)
			return
		}
		if messageID > 0 && telegramID != "" {
			if saveErr := w.service.SetBotMessage(ctx, telegramID, messageID); saveErr != nil {
				w.logger.Warn("telegram message pointer was not saved", "error", saveErr)
			}
		}
	}
	if err := w.service.CompleteUpdate(ctx, update.ID, strconv.FormatInt(chatID, 10), ignored); err != nil {
		w.logger.Error("telegram update completion failed", "update_id", update.ID, "error", err)
	}
}

func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
