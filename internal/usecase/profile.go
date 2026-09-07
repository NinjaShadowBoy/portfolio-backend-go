package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

const minPasswordLength = 8

func (s *UseCases) UpdateProfile(ctx context.Context, userID int64, req dto.ProfileUpdate) (dto.User, error) {
	var name *string
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return dto.User{}, ErrBadRequest
		}
		name = dto.Ptr(trimmed)
	}
	image := req.ImageURL

	user, err := s.repos.Users().UpdateUserProfile(ctx, userID, name, image)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.User{}, ErrNotFound
	}
	if err != nil {
		return dto.User{}, err
	}
	return dto.FromUser(user), nil
}

// ChangePassword sets a new password for a LOCAL account after verifying the
// current one. OAuth accounts (no local password) cannot use this.

func (s *UseCases) ChangePassword(ctx context.Context, userID int64, req dto.PasswordChange) error {
	if len(req.NewPassword) < minPasswordLength {
		return ErrBadRequest
	}
	user, err := s.repos.Users().GetUserByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if user.PasswordHash == nil {
		// No local password to change (OAuth-only account).
		return ErrForbidden
	}
	if !s.passwords.Verify(user.PasswordHash, req.CurrentPassword) {
		return ErrUnauthorized
	}
	hashed, err := s.passwords.Hash(req.NewPassword)
	if err != nil {
		return err
	}
	return s.repos.Users().UpdateUserPassword(ctx, userID, dto.Ptr(hashed))
}

// DeleteAccount removes the caller's account; ratings and comments cascade.

func (s *UseCases) DeleteAccount(ctx context.Context, userID int64) error {
	return s.repos.Users().DeleteUser(ctx, userID)
}
