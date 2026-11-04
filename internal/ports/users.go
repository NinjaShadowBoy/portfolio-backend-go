package ports

import (
	"context"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type UsersRepository interface {
	CountUsersByNamePrefix(ctx context.Context, name string) (int32, error)
	CountUsersForAdmin(ctx context.Context, filter domain.UserFilter) (int32, error)
	CreateUser(ctx context.Context, user domain.User) (domain.User, error)
	DeleteUser(ctx context.Context, id int64) error
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	GetUserByID(ctx context.Context, id int64) (domain.User, error)
	GetUserByProvider(ctx context.Context, provider string, providerID *string) (domain.User, error)
	GetUsersByIDs(ctx context.Context, ids []int64) ([]domain.User, error)
	ListUsersForAdmin(ctx context.Context, filter domain.UserFilter) ([]domain.AdminUser, error)
	UpdateUserLogin(ctx context.Context, id int64, lastLoginAt *time.Time) (domain.User, error)
	UpdateUserOAuthProfile(ctx context.Context, user domain.User) (domain.User, error)
	UpdateUserPassword(ctx context.Context, id int64, passwordHash *string) error
	UpdateUserProfile(ctx context.Context, id int64, name *string, imageURL *string) (domain.User, error)
	UpdateUserRole(ctx context.Context, id int64, role string) (domain.User, error)
	UserExistsByEmail(ctx context.Context, email string) (bool, error)
}
