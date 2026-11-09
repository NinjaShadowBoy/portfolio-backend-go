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

// Embedder turns texts into vectors, one per text and in the same order.
// The purpose picks the document or the query model.
type Embedder interface {
	Embed(ctx context.Context, texts []string, purpose domain.EmbedPurpose) ([][]float32, error)
}

// KnowledgeCorpus loads the author-written corpus files (CVs, papers, About).
type KnowledgeCorpus interface {
	Load(ctx context.Context) ([]domain.KnowledgeSource, error)
}
