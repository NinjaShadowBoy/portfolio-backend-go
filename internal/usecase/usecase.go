package usecase

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
)

var (
	ErrNotFound      = domain.ErrNotFound
	ErrUnauthorized  = domain.ErrUnauthorized
	ErrForbidden     = domain.ErrForbidden
	ErrConflict      = domain.ErrConflict
	ErrBadRequest    = domain.ErrBadRequest
	ErrAlreadyExists = domain.ErrAlreadyExists
)

type OAuthProvider = domain.OAuthProvider
type OAuthLoginStart = domain.OAuthLoginStart
type DeviceLoginStart = domain.DeviceLoginStart
type DeviceAuthStatus = domain.DeviceAuthStatus

const (
	OAuthProviderGoogle = domain.OAuthProviderGoogle
	OAuthProviderGitHub = domain.OAuthProviderGitHub
	DeviceAuthPending   = domain.DeviceAuthPending
	DeviceAuthSlowDown  = domain.DeviceAuthSlowDown
	DeviceAuthComplete  = domain.DeviceAuthComplete
)

type Service interface {
	Login(ctx context.Context, email, password string) (dto.LoginResponse, error)
	Register(ctx context.Context, req dto.RegisterRequest) error
	StartOAuthLogin(provider OAuthProvider) (OAuthLoginStart, error)
	OAuthLogin(ctx context.Context, provider OAuthProvider, code, state, verifier string) (dto.LoginResponse, error)
	StartGitHubDeviceLogin(ctx context.Context) (DeviceLoginStart, error)
	PollGitHubDeviceLogin(ctx context.Context, deviceCode string) (dto.LoginResponse, DeviceAuthStatus, error)
	ListProjects(ctx context.Context) ([]dto.Project, error)
	GetProject(ctx context.Context, id int64) (dto.Project, error)
	CreateProject(ctx context.Context, req dto.ProjectCreate) (dto.Project, error)
	UpdateProject(ctx context.Context, id int64, req dto.ProjectUpdate) (dto.Project, error)
	DeleteProject(ctx context.Context, id int64) error
	ReorderProjects(ctx context.Context, ids []int64) error
	GetProjectRatings(ctx context.Context, projectID int64) ([]dto.Rating, error)
	GetProjectAverageRating(ctx context.Context, projectID int64) (float64, error)
	GetProjectRatingCount(ctx context.Context, projectID int64) (int, error)
	GetProjectRatingDistribution(ctx context.Context, projectID int64) (map[int]int, error)
	ResetProjectRatings(ctx context.Context, projectID int64) error
	ListRatings(ctx context.Context) ([]dto.Rating, error)
	GetRating(ctx context.Context, id int64) (dto.Rating, error)
	GetRatingsByUserID(ctx context.Context, userID int64) ([]dto.Rating, error)
	CreateRating(ctx context.Context, userID int64, req dto.RatingCreate) (dto.Rating, error)
	UpdateRating(ctx context.Context, id int64, userID int64, req dto.RatingUpdate) (dto.Rating, error)
	DeleteRating(ctx context.Context, id int64, userID int64) error
	HasUserRatedProject(ctx context.Context, userID int64, projectID int64) (bool, error)
	SaveProjectPhoto(ctx context.Context, photoURL string, projectID int64) (dto.Photo, error)
	SaveProfilePhoto(ctx context.Context, photoURL string) (dto.Photo, error)
	DeletePhoto(ctx context.Context, id int64) error
	SubmitContact(ctx context.Context, req dto.Contact) (dto.ContactMessage, error)
	ListContactMessages(ctx context.Context) ([]dto.ContactMessage, error)
	ListUnreadContactMessages(ctx context.Context) ([]dto.ContactMessage, error)
	MarkContactRead(ctx context.Context, id int64) (dto.ContactMessage, error)
	MarkContactReplied(ctx context.Context, id int64) (dto.ContactMessage, error)
	DeleteContactMessage(ctx context.Context, id int64) error
	CreateComment(ctx context.Context, userID int64, req dto.CommentCreate) (dto.Comment, error)
	ListProjectComments(ctx context.Context, projectID int64) ([]dto.Comment, error)
	ListArticleComments(ctx context.Context, slug string) ([]dto.Comment, error)
	ListUserComments(ctx context.Context, userID int64) ([]dto.Comment, error)
	UpdateComment(ctx context.Context, id int64, userID int64, req dto.CommentUpdate) (dto.Comment, error)
	DeleteComment(ctx context.Context, id int64, userID int64) error
	CreateAnonymousUser(ctx context.Context) (int64, string, error)
	ResolveAnonToken(ctx context.Context, rawToken string) (int64, bool, error)
	GetIdentity(ctx context.Context, userID int64) (dto.Identity, error)
	ClaimAnonymousActivity(ctx context.Context, anonUserID, realUserID int64) error
	UpdateProfile(ctx context.Context, userID int64, req dto.ProfileUpdate) (dto.User, error)
	ChangePassword(ctx context.Context, userID int64, req dto.PasswordChange) error
	DeleteAccount(ctx context.Context, userID int64) error
	GetAdminStats(ctx context.Context) (dto.AdminStats, error)
	ListUsersForAdmin(ctx context.Context, query dto.AdminUserQuery) (dto.Page[dto.AdminUser], error)
	UpdateUserRole(ctx context.Context, actorID, userID int64, role string) (dto.User, error)
	DeleteUserAsAdmin(ctx context.Context, actorID, userID int64) error
	ListCommentsForAdmin(ctx context.Context, query dto.AdminCommentQuery) (dto.Page[dto.AdminComment], error)
	DeleteCommentAsAdmin(ctx context.Context, id int64) error
}

// Dependencies are supplied by the composition root. Use cases never construct adapters.
type Dependencies struct {
	Repositories ports.Repositories
	UnitOfWork   ports.UnitOfWork
	Tokens       ports.TokenIssuer
	Passwords    ports.PasswordHasher
	OAuth        ports.OAuthGateway
	Emails       ports.EmailSender
}

type UseCases struct {
	repos     ports.Repositories
	uow       ports.UnitOfWork
	tokens    ports.TokenIssuer
	passwords ports.PasswordHasher
	oauth     ports.OAuthGateway
	emails    ports.EmailSender
}

func New(deps Dependencies) Service {
	return &UseCases{repos: deps.Repositories, uow: deps.UnitOfWork, tokens: deps.Tokens, passwords: deps.Passwords, oauth: deps.OAuth, emails: deps.Emails}
}

// inTransaction gives an operation a private copy with transaction-scoped repositories.
// It never mutates the shared use cases or exposes database handles to them.
func inTransaction[T any](ctx context.Context, s *UseCases, work func(*UseCases) (T, error)) (T, error) {
	var result T
	err := s.uow.WithinTx(ctx, func(repos ports.Repositories) error {
		scoped := *s
		scoped.repos = repos
		value, err := work(&scoped)
		result = value
		return err
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}
