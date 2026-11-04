package postgres

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func (r *repositories) GetAdminStats(ctx context.Context) (domain.AdminStats, error) {
	row, err := r.q.GetAdminStats(ctx)
	return domain.AdminStats(row), repositoryError(err)
}
