package telegram

import (
	"context"

	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

// UnavailableDraftExtractor keeps menu and identity-link flows available when
// the bot token is configured before an OpenAI key is supplied.
type UnavailableDraftExtractor struct{}

func (UnavailableDraftExtractor) Extract(context.Context, string, domain.Draft, []domain.System) (domain.Draft, error) {
	return domain.Draft{}, ErrAIUnavailable
}
