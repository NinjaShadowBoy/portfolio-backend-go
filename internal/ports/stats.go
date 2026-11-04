package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type StatsRepository interface {
	GetAdminStats(ctx context.Context) (domain.AdminStats, error)
}
