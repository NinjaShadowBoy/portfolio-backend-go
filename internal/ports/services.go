package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

type TokenIssuer interface {
	GenerateLoginResponse(domain.User) (dto.LoginResponse, error)
}
type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(*string, string) bool
}
type EmailSender interface {
	SendContactFormNotification(context.Context, string, string, string) error
	SendContactConfirmation(context.Context, string, string) error
}
type OAuthGateway interface {
	Start(domain.OAuthProvider) (domain.OAuthLoginStart, error)
	Exchange(context.Context, domain.OAuthProvider, string, string, string) (domain.OAuthProfile, error)
	StartDevice(context.Context) (domain.DeviceLoginStart, error)
	PollDevice(context.Context, string) (domain.OAuthProfile, domain.DeviceAuthStatus, error)
}
