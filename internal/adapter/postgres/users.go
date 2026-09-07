package postgres

import (
	"context"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func (r *repositories) CountUsersByNamePrefix(ctx context.Context, name string) (int32, error) {
	row, err := r.q.CountUsersByNamePrefix(ctx, name)
	return row, repositoryError(err)
}

func (r *repositories) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	row, err := r.q.CreateUser(ctx, user.Email, nullString(user.PasswordHash), user.Name, user.Provider, nullString(user.ProviderID), nullString(user.ImageURL), user.EmailVerified, nullTime(user.LastLoginAt))
	return fromUser(row), repositoryError(err)
}

func (r *repositories) DeleteUser(ctx context.Context, id int64) error {
	return repositoryError(r.q.DeleteUser(ctx, id))
}

func (r *repositories) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	return fromUser(row), repositoryError(err)
}

func (r *repositories) GetUserByID(ctx context.Context, id int64) (domain.User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	return fromUser(row), repositoryError(err)
}

func (r *repositories) GetUserByProvider(ctx context.Context, provider string, providerID *string) (domain.User, error) {
	row, err := r.q.GetUserByProvider(ctx, provider, nullString(providerID))
	return fromUser(row), repositoryError(err)
}

func (r *repositories) GetUsersByIDs(ctx context.Context, ids []int64) ([]domain.User, error) {
	row, err := r.q.GetUsersByIDs(ctx, ids)
	return mapRows(row, fromUser), repositoryError(err)
}

func (r *repositories) UpdateUserLogin(ctx context.Context, id int64, lastLoginAt *time.Time) (domain.User, error) {
	row, err := r.q.UpdateUserLogin(ctx, id, nullTime(lastLoginAt))
	return fromUser(row), repositoryError(err)
}

func (r *repositories) UpdateUserOAuthProfile(ctx context.Context, user domain.User) (domain.User, error) {
	row, err := r.q.UpdateUserOAuthProfile(ctx, user.ID, user.Name, user.Provider, nullString(user.ProviderID), nullString(user.ImageURL), user.EmailVerified)
	return fromUser(row), repositoryError(err)
}

func (r *repositories) UpdateUserPassword(ctx context.Context, id int64, passwordHash *string) error {
	return repositoryError(r.q.UpdateUserPassword(ctx, id, nullString(passwordHash)))
}

func (r *repositories) UpdateUserProfile(ctx context.Context, id int64, name *string, imageURL *string) (domain.User, error) {
	row, err := r.q.UpdateUserProfile(ctx, id, nullString(name), nullString(imageURL))
	return fromUser(row), repositoryError(err)
}

func (r *repositories) UserExistsByEmail(ctx context.Context, email string) (bool, error) {
	row, err := r.q.UserExistsByEmail(ctx, email)
	return row, repositoryError(err)
}
