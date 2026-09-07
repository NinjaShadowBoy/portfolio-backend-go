package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type AnonymousTokensRepository interface {
	CreateAnonToken(ctx context.Context, tokenHash string, userID int64) error
	GetUserByAnonToken(ctx context.Context, tokenHash string) (domain.User, error)
}
