package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

const (
	maxCommentLength     = 2000
	maxArticleSlugLength = 255
)

// commentTypes is the allowed feedback-type set, mirroring the DB CHECK
// constraint added in migration 003. The service validates it up front so a
// bad type yields a clean 400 rather than a DB constraint violation.
var commentTypes = map[string]struct{}{
	"critique":        {},
	"suggestion":      {},
	"feature_request": {},
}

func (s *UseCases) CreateComment(ctx context.Context, userID int64, req dto.CommentCreate) (dto.Comment, error) {
	body, err := validateCommentBody(req.Body)
	if err != nil {
		return dto.Comment{}, err
	}
	if _, ok := commentTypes[req.Type]; !ok {
		return dto.Comment{}, ErrBadRequest
	}

	// Exactly one target must be set: a comment lives on a project OR an
	// article, never both and never neither. The DB has a matching CHECK
	// constraint; validating here returns a clean 400 instead.
	hasProject := req.ProjectID != nil
	slug := ""
	if req.ArticleSlug != nil {
		slug = strings.TrimSpace(*req.ArticleSlug)
	}
	hasArticle := slug != ""
	if hasProject == hasArticle {
		return dto.Comment{}, ErrBadRequest
	}

	var (
		projectArg *int64
		slugArg    *string
	)
	if hasProject {
		if err := s.requireProject(ctx, *req.ProjectID); err != nil {
			return dto.Comment{}, err
		}
		projectArg = dto.Ptr(*req.ProjectID)
	} else {
		// Articles have no DB row, so we cannot existence-check the slug;
		// enforce only a non-empty, sanely-bounded value.
		if len(slug) > maxArticleSlugLength {
			return dto.Comment{}, ErrBadRequest
		}
		slugArg = dto.Ptr(slug)
	}

	comment, err := s.repos.Comments().CreateComment(ctx, body, userID, projectArg, slugArg, req.Type)
	if err != nil {
		return dto.Comment{}, err
	}
	return s.commentDTO(ctx, comment)
}

func (s *UseCases) ListProjectComments(ctx context.Context, projectID int64) ([]dto.Comment, error) {
	rows, err := s.repos.Comments().ListCommentsByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	comments := make([]dto.Comment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, dto.FromComment(domain.Comment{
			ID:          row.ID,
			Body:        row.Body,
			UserID:      row.UserID,
			ProjectID:   row.ProjectID,
			ArticleSlug: row.ArticleSlug,
			Type:        row.Type,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}, row.UserName, row.UserImage))
	}
	return comments, nil
}

func (s *UseCases) ListArticleComments(ctx context.Context, slug string) ([]dto.Comment, error) {
	trimmed := strings.TrimSpace(slug)
	if trimmed == "" {
		return nil, ErrBadRequest
	}
	rows, err := s.repos.Comments().ListCommentsByArticleSlug(ctx, dto.Ptr(trimmed))
	if err != nil {
		return nil, err
	}
	comments := make([]dto.Comment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, dto.FromComment(domain.Comment{
			ID:          row.ID,
			Body:        row.Body,
			UserID:      row.UserID,
			ProjectID:   row.ProjectID,
			ArticleSlug: row.ArticleSlug,
			Type:        row.Type,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}, row.UserName, row.UserImage))
	}
	return comments, nil
}

func (s *UseCases) ListUserComments(ctx context.Context, userID int64) ([]dto.Comment, error) {
	rows, err := s.repos.Comments().ListCommentsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	comments := make([]dto.Comment, 0, len(rows))
	if len(rows) == 0 {
		return comments, nil
	}
	user, err := s.repos.Users().GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		comments = append(comments, dto.FromComment(row, user.Name, user.ImageURL))
	}
	return comments, nil
}

func (s *UseCases) UpdateComment(ctx context.Context, id int64, userID int64, req dto.CommentUpdate) (dto.Comment, error) {
	body, err := validateCommentBody(req.Body)
	if err != nil {
		return dto.Comment{}, err
	}
	existing, err := s.repos.Comments().GetComment(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.Comment{}, ErrNotFound
	}
	if err != nil {
		return dto.Comment{}, err
	}
	if existing.UserID != userID {
		return dto.Comment{}, ErrForbidden
	}
	updated, err := s.repos.Comments().UpdateComment(ctx, id, userID, body)
	if err != nil {
		return dto.Comment{}, err
	}
	return s.commentDTO(ctx, updated)
}

func (s *UseCases) DeleteComment(ctx context.Context, id int64, userID int64) error {
	rows, err := s.repos.Comments().DeleteComment(ctx, id, userID)
	if err != nil {
		return err
	}
	if rows == 0 {
		existing, err := s.repos.Comments().GetComment(ctx, id)
		if errors.Is(err, domain.ErrNotFound) {
			return ErrNotFound
		}
		if err == nil && existing.UserID != userID {
			return ErrForbidden
		}
		return ErrNotFound
	}
	return nil
}

func (s *UseCases) commentDTO(ctx context.Context, comment domain.Comment) (dto.Comment, error) {
	user, err := s.repos.Users().GetUserByID(ctx, comment.UserID)
	if err != nil {
		return dto.Comment{}, err
	}
	return dto.FromComment(comment, user.Name, user.ImageURL), nil
}

func validateCommentBody(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || len(trimmed) > maxCommentLength {
		return "", ErrBadRequest
	}
	return trimmed, nil
}
