package postgres

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func (r *repositories) CreateAnonToken(ctx context.Context, tokenHash string, userID int64) error {
	return repositoryError(r.q.CreateAnonToken(ctx, tokenHash, userID))
}

func (r *repositories) GetUserByAnonToken(ctx context.Context, tokenHash string) (domain.User, error) {
	row, err := r.q.GetUserByAnonToken(ctx, tokenHash)
	return fromUser(row), repositoryError(err)
}
