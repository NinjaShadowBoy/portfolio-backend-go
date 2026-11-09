package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
)

var _ ports.ChatUsageRepository = (*repositories)(nil)

func (r *repositories) ChatUsage() ports.ChatUsageRepository { return r }

func (r *repositories) AddUsage(ctx context.Context, day time.Time, subject string, delta domain.ChatUsageCounts) error {
	if delta.Requests < 0 || delta.Requests > math.MaxInt32 {
		return fmt.Errorf("chat usage: request delta %d out of range", delta.Requests)
	}
	return repositoryError(r.q.AddChatUsage(ctx, utcDay(day), subject, int32(delta.Requests), delta.InputTokens, delta.OutputTokens))
}

func (r *repositories) GetUsage(ctx context.Context, day time.Time, subject string) (domain.ChatUsageCounts, error) {
	row, err := r.q.GetChatUsage(ctx, utcDay(day), subject)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ChatUsageCounts{}, nil
	}
	if err != nil {
		return domain.ChatUsageCounts{}, err
	}
	return domain.ChatUsageCounts{Requests: int64(row.Requests), InputTokens: row.InputTokens, OutputTokens: row.OutputTokens}, nil
}

func (r *repositories) ListDailyUsage(ctx context.Context, from, to time.Time) ([]domain.ChatUsageDay, error) {
	rows, err := r.q.ListChatDailyUsage(ctx, utcDay(from), utcDay(to))
	if err != nil {
		return nil, repositoryError(err)
	}
	out := make([]domain.ChatUsageDay, len(rows))
	for i, row := range rows {
		out[i] = domain.ChatUsageDay{
			Day:             utcDay(row.Day),
			ChatUsageCounts: domain.ChatUsageCounts{Requests: int64(row.Requests), InputTokens: row.InputTokens, OutputTokens: row.OutputTokens},
		}
	}
	return out, nil
}

func (r *repositories) ListSubjectUsage(ctx context.Context, from, to time.Time, limit int) ([]domain.ChatUsageSubject, error) {
	rows, err := r.q.ListChatSubjectUsage(ctx, utcDay(from), utcDay(to), int32(limit))
	if err != nil {
		return nil, repositoryError(err)
	}
	out := make([]domain.ChatUsageSubject, len(rows))
	for i, row := range rows {
		out[i] = domain.ChatUsageSubject{
			Subject:         row.Subject,
			ChatUsageCounts: domain.ChatUsageCounts{Requests: row.Requests, InputTokens: row.InputTokens, OutputTokens: row.OutputTokens},
		}
	}
	return out, nil
}

// utcDay is midnight UTC of t's UTC date, so the DATE column always holds the
// UTC day whatever t's location is (the caps reset at midnight UTC).
func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
