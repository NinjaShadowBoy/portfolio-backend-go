package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

const (
	defaultAdminPageSize = 50
	maxAdminPageSize     = 200
	maxAdminSearchLength = 100
	anonymousProvider    = "ANON"
)

var userRoles = map[string]struct{}{"USER": {}, "ADMIN": {}}

var commentTargets = map[string]struct{}{"project": {}, "article": {}}

func (s *UseCases) GetAdminStats(ctx context.Context) (dto.AdminStats, error) {
	stats, err := s.repos.Stats().GetAdminStats(ctx)
	if err != nil {
		return dto.AdminStats{}, err
	}
	return dto.FromAdminStats(stats), nil
}

func (s *UseCases) ListUsersForAdmin(ctx context.Context, query dto.AdminUserQuery) (dto.Page[dto.AdminUser], error) {
	limit, offset, err := adminPage(query.Limit, query.Offset)
	if err != nil {
		return dto.Page[dto.AdminUser]{}, err
	}
	filter := domain.UserFilter{IncludeAnonymous: query.IncludeAnonymous, Limit: limit, Offset: offset}
	if search := strings.TrimSpace(query.Search); search != "" {
		if len(search) > maxAdminSearchLength {
			return dto.Page[dto.AdminUser]{}, ErrBadRequest
		}
		filter.Search = dto.Ptr(search)
	}

	total, err := s.repos.Users().CountUsersForAdmin(ctx, filter)
	if err != nil {
		return dto.Page[dto.AdminUser]{}, err
	}
	rows, err := s.repos.Users().ListUsersForAdmin(ctx, filter)
	if err != nil {
		return dto.Page[dto.AdminUser]{}, err
	}
	users := make([]dto.AdminUser, 0, len(rows))
	for _, row := range rows {
		users = append(users, dto.FromAdminUser(row))
	}
	return dto.Page[dto.AdminUser]{Items: users, Total: int(total), Limit: int(limit), Offset: int(offset)}, nil
}

// UpdateUserRole changes another account's role. An admin cannot change their
// own role, so the last admin can never demote themselves out of the panel.
// The new role applies at the user's next login: roles travel in the JWT.
func (s *UseCases) UpdateUserRole(ctx context.Context, actorID, userID int64, role string) (dto.User, error) {
	if _, ok := userRoles[role]; !ok {
		return dto.User{}, ErrBadRequest
	}
	if actorID == userID {
		return dto.User{}, fmt.Errorf("you cannot change your own role: %w", ErrBadRequest)
	}
	user, err := s.repos.Users().GetUserByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.User{}, ErrNotFound
	}
	if err != nil {
		return dto.User{}, err
	}
	if user.Provider == anonymousProvider {
		return dto.User{}, fmt.Errorf("anonymous identities cannot be given a role: %w", ErrBadRequest)
	}
	updated, err := s.repos.Users().UpdateUserRole(ctx, userID, role)
	if err != nil {
		return dto.User{}, err
	}
	return dto.FromUser(updated), nil
}

// DeleteUserAsAdmin removes another account; its ratings, comments and anon
// tokens cascade. Admins delete their own account through DELETE /users/me.
func (s *UseCases) DeleteUserAsAdmin(ctx context.Context, actorID, userID int64) error {
	if actorID == userID {
		return fmt.Errorf("use account settings to delete your own account: %w", ErrBadRequest)
	}
	if _, err := s.repos.Users().GetUserByID(ctx, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.repos.Users().DeleteUser(ctx, userID)
}

func (s *UseCases) ListCommentsForAdmin(ctx context.Context, query dto.AdminCommentQuery) (dto.Page[dto.AdminComment], error) {
	limit, offset, err := adminPage(query.Limit, query.Offset)
	if err != nil {
		return dto.Page[dto.AdminComment]{}, err
	}
	filter := domain.CommentFilter{Limit: limit, Offset: offset}
	if query.Type != "" {
		if _, ok := commentTypes[query.Type]; !ok {
			return dto.Page[dto.AdminComment]{}, ErrBadRequest
		}
		filter.Type = dto.Ptr(query.Type)
	}
	if query.Target != "" {
		if _, ok := commentTargets[query.Target]; !ok {
			return dto.Page[dto.AdminComment]{}, ErrBadRequest
		}
		filter.Target = dto.Ptr(query.Target)
	}

	total, err := s.repos.Comments().CountCommentsForAdmin(ctx, filter)
	if err != nil {
		return dto.Page[dto.AdminComment]{}, err
	}
	rows, err := s.repos.Comments().ListCommentsForAdmin(ctx, filter)
	if err != nil {
		return dto.Page[dto.AdminComment]{}, err
	}
	comments := make([]dto.AdminComment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, dto.FromAdminComment(row))
	}
	return dto.Page[dto.AdminComment]{Items: comments, Total: int(total), Limit: int(limit), Offset: int(offset)}, nil
}

// DeleteCommentAsAdmin removes any comment regardless of its author.
func (s *UseCases) DeleteCommentAsAdmin(ctx context.Context, id int64) error {
	rows, err := s.repos.Comments().DeleteCommentByID(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// adminPage applies the default page size, caps it, and rejects negatives.
func adminPage(limit, offset int) (int32, int32, error) {
	if offset < 0 || limit < 0 {
		return 0, 0, ErrBadRequest
	}
	if limit == 0 {
		limit = defaultAdminPageSize
	}
	if limit > maxAdminPageSize {
		limit = maxAdminPageSize
	}
	return int32(limit), int32(offset), nil
}
