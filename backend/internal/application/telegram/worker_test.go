package telegram

import (
	"context"
	"io"
	"log/slog"
	"testing"

	integration "github.com/lynx/lynx-os/backend/internal/domain/integration"
	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

func TestWorkerCompletesAndDeduplicatesUpdates(t *testing.T) {
	repository := &workerConversationRepository{handled: map[int64]bool{}}
	service := NewService(repository, nil, nil, nil, nil, nil)
	client := &workerClient{}
	worker := NewWorker(client, service, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	update := domain.Update{ID: 42, Message: &domain.Message{Text: "/start", FromID: 7, ChatID: 8, ChatType: "group"}}

	worker.process(context.Background(), update)
	worker.process(context.Background(), update)

	if repository.completed != 1 || repository.offset != 43 {
		t.Fatalf("update should be completed exactly once and advance offset: completed=%d offset=%d", repository.completed, repository.offset)
	}
	if client.sent != 0 {
		t.Fatalf("group message should be ignored without a reply, got %d sends", client.sent)
	}
}

type workerConversationRepository struct {
	handled   map[int64]bool
	completed int
	offset    int64
}

func (r *workerConversationRepository) GetConversation(context.Context, string) (domain.Conversation, integration.SealedSecret, error) {
	return domain.Conversation{}, integration.SealedSecret{}, domain.ErrConversationNotFound
}
func (*workerConversationRepository) SaveConversation(context.Context, domain.Conversation, integration.SealedSecret) error {
	return nil
}
func (*workerConversationRepository) DeleteConversation(context.Context, string) error { return nil }
func (r *workerConversationRepository) GetUpdateOffset(context.Context) (int64, error) {
	return r.offset, nil
}
func (r *workerConversationRepository) BeginUpdate(_ context.Context, id int64, _ string) (bool, error) {
	if r.handled[id] {
		return false, nil
	}
	r.handled[id] = true
	return true, nil
}
func (r *workerConversationRepository) CompleteUpdate(_ context.Context, id int64, _ string, _ bool) error {
	r.completed++
	if id+1 > r.offset {
		r.offset = id + 1
	}
	return nil
}

type workerClient struct{ sent int }

func (*workerClient) Poll(context.Context, int64) ([]domain.Update, error) { return nil, nil }
func (c *workerClient) Send(context.Context, *domain.Reply) (int64, error) {
	c.sent++
	return 1, nil
}
func (*workerClient) AnswerCallback(context.Context, string) error { return nil }
func (*workerClient) DeleteWebhook(context.Context) error          { return nil }
