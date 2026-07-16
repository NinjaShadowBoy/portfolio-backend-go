package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

const minPasswordLength = 8

// UpdateProfile changes the caller's display name and/or avatar. Nil fields are
// left untouched.
func (s *service) UpdateProfile(ctx context.Context, userID int64, req dto.ProfileUpdate) (dto.User, error) {
	var name sql.NullString
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return dto.User{}, ErrBadRequest
		}
		name = sql.NullString{String: trimmed, Valid: true}
	}
	image := nullableString(req.ImageURL)

	user, err := s.Q.UpdateUserProfile(ctx, userID, name, image)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.User{}, ErrNotFound
	}
	if err != nil {
		return dto.User{}, err
	}
	return dto.FromUser(user), nil
}

// ChangePassword sets a new password for a LOCAL account after verifying the
// current one. OAuth accounts (no local password) cannot use this.
func (s *service) ChangePassword(ctx context.Context, userID int64, req dto.PasswordChange) error {
	if len(req.NewPassword) < minPasswordLength {
		return ErrBadRequest
	}
	user, err := s.Q.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !user.Pwd.Valid {
		// No local password to change (OAuth-only account).
		return ErrForbidden
	}
	if !auth.VerifyPassword(user.Pwd, req.CurrentPassword) {
		return ErrUnauthorized
	}
	hashed, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	return s.Q.UpdateUserPassword(ctx, userID, sql.NullString{String: hashed, Valid: true})
}

// DeleteAccount removes the caller's account; ratings and comments cascade.
func (s *service) DeleteAccount(ctx context.Context, userID int64) error {
	return s.Q.DeleteUser(ctx, userID)
}
