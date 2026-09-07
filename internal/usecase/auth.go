package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func (s *UseCases) Login(ctx context.Context, email, password string) (dto.LoginResponse, error) {
	user, err := s.repos.Users().GetUserByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.LoginResponse{}, ErrUnauthorized
	}
	if err != nil {
		return dto.LoginResponse{}, err
	}
	if !s.passwords.Verify(user.PasswordHash, password) {
		return dto.LoginResponse{}, ErrUnauthorized
	}

	updated, err := s.repos.Users().UpdateUserLogin(ctx, user.ID, dto.Ptr(time.Now()))
	if err != nil {
		return dto.LoginResponse{}, err
	}

	return s.tokens.GenerateLoginResponse(updated)
}

func (s *UseCases) Register(ctx context.Context, req dto.RegisterRequest) error {
	exists, err := s.repos.Users().UserExistsByEmail(ctx, req.Email)
	if err != nil {
		return err
	}
	if exists {
		return ErrAlreadyExists
	}

	hashed, err := s.passwords.Hash(req.Password)
	if err != nil {
		return err
	}

	_, err = s.repos.Users().CreateUser(ctx, domain.User{Email: req.Email, PasswordHash: dto.Ptr(hashed), Name: req.Name, Provider: "LOCAL"})
	return err
}

func (s *UseCases) upsertOAuthUser(ctx context.Context, provider, providerID, email, displayName, avatarURL string, emailVerified bool) (domain.User, error) {
	profile := domain.User{Email: email, Name: displayName, Provider: provider, ProviderID: dto.Ptr(providerID), ImageURL: nullableStringFrom(avatarURL), EmailVerified: emailVerified}
	user, err := s.repos.Users().GetUserByProvider(ctx, provider, dto.Ptr(providerID))
	if err == nil {
		profile.ID = user.ID
		return s.repos.Users().UpdateUserOAuthProfile(ctx, profile)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, err
	}

	exists, err := s.repos.Users().UserExistsByEmail(ctx, email)
	if err != nil {
		return domain.User{}, err
	}
	if exists {
		user, err = s.repos.Users().GetUserByEmail(ctx, email)
		if err != nil {
			return domain.User{}, err
		}
		profile.ID = user.ID
		return s.repos.Users().UpdateUserOAuthProfile(ctx, profile)
	}

	return s.repos.Users().CreateUser(ctx, profile)
}

func nullableStringFrom(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (s *UseCases) StartOAuthLogin(provider OAuthProvider) (OAuthLoginStart, error) {
	return s.oauth.Start(provider)
}
func (s *UseCases) StartGitHubDeviceLogin(ctx context.Context) (DeviceLoginStart, error) {
	return s.oauth.StartDevice(ctx)
}

func (s *UseCases) OAuthLogin(ctx context.Context, provider OAuthProvider, code, state, verifier string) (dto.LoginResponse, error) {
	profile, err := s.oauth.Exchange(ctx, provider, code, state, verifier)
	if err != nil {
		return dto.LoginResponse{}, err
	}
	return s.completeOAuthLogin(ctx, profile)
}
func (s *UseCases) PollGitHubDeviceLogin(ctx context.Context, code string) (dto.LoginResponse, DeviceAuthStatus, error) {
	profile, status, err := s.oauth.PollDevice(ctx, code)
	if err != nil || status != DeviceAuthComplete {
		return dto.LoginResponse{}, status, err
	}
	response, err := s.completeOAuthLogin(ctx, profile)
	if err != nil {
		return dto.LoginResponse{}, "", err
	}
	return response, status, nil
}
func (s *UseCases) completeOAuthLogin(ctx context.Context, profile domain.OAuthProfile) (dto.LoginResponse, error) {
	user, err := inTransaction(ctx, s, func(tx *UseCases) (domain.User, error) {
		user, err := tx.upsertOAuthUser(ctx, string(profile.Provider), profile.ProviderID, profile.Email, profile.DisplayName, profile.AvatarURL, profile.EmailVerified)
		if err != nil {
			return domain.User{}, err
		}
		return tx.repos.Users().UpdateUserLogin(ctx, user.ID, dto.Ptr(time.Now()))
	})
	if err != nil {
		return dto.LoginResponse{}, err
	}
	return s.tokens.GenerateLoginResponse(user)
}
