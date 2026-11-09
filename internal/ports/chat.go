package ports

import (
	"context"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

// ChatModel is the language model behind the site assistant.
type ChatModel interface {
	// Stream runs one model turn. onText receives text deltas as they arrive;
	// an error from onText aborts the turn. The result holds the full text,
	// the tool calls and the token usage, which is set even when the turn
	// fails part way, as far as it is known.
	Stream(ctx context.Context, req domain.ChatTurnRequest, onText func(string) error) (domain.ChatTurnResult, error)
}

// ChatUsageRepository keeps the daily chat counters (no transcripts).
type ChatUsageRepository interface {
	// AddUsage adds delta to the (day, subject) row, creating it if needed.
	AddUsage(ctx context.Context, day time.Time, subject string, delta domain.ChatUsageCounts) error
	// GetUsage returns the (day, subject) row; zero counts when there is none.
	GetUsage(ctx context.Context, day time.Time, subject string) (domain.ChatUsageCounts, error)
	// ListDailyUsage returns the global ('*') row of every day in [from, to],
	// ordered by day.
	ListDailyUsage(ctx context.Context, from, to time.Time) ([]domain.ChatUsageDay, error)
	// ListSubjectUsage returns the per-subject totals of every subject other
	// than '*', best first by requests, at most limit of them.
	ListSubjectUsage(ctx context.Context, from, to time.Time, limit int) ([]domain.ChatUsageSubject, error)
}
