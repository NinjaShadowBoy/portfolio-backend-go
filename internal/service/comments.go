package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
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

func (s *service) CreateComment(ctx context.Context, userID int64, req dto.CommentCreate) (dto.Comment, error) {
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
		projectArg sql.NullInt64
		slugArg    sql.NullString
	)
	if hasProject {
		if err := s.requireProject(ctx, *req.ProjectID); err != nil {
			return dto.Comment{}, err
		}
		projectArg = sql.NullInt64{Int64: *req.ProjectID, Valid: true}
	} else {
		// Articles have no DB row, so we cannot existence-check the slug;
		// enforce only a non-empty, sanely-bounded value.
		if len(slug) > maxArticleSlugLength {
			return dto.Comment{}, ErrBadRequest
		}
		slugArg = sql.NullString{String: slug, Valid: true}
	}

	comment, err := s.Q.CreateComment(ctx, body, userID, projectArg, slugArg, req.Type)
	if err != nil {
		return dto.Comment{}, err
	}
	return s.commentDTO(ctx, comment)
}

func (s *service) ListProjectComments(ctx context.Context, projectID int64) ([]dto.Comment, error) {
	rows, err := s.Q.ListCommentsByProjectID(ctx, sql.NullInt64{Int64: projectID, Valid: true})
	if err != nil {
		return nil, err
	}
	comments := make([]dto.Comment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, dto.Comment{
			ID:          row.ID,
			UserID:      row.UserID,
			UserName:    row.UserName,
			UserImage:   dto.NullString(row.UserImage),
			ProjectID:   nullInt64Ptr(row.ProjectID),
			ArticleSlug: dto.NullString(row.ArticleSlug),
			Type:        row.Type,
			Body:        row.Body,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		})
	}
	return comments, nil
}

func (s *service) ListArticleComments(ctx context.Context, slug string) ([]dto.Comment, error) {
	trimmed := strings.TrimSpace(slug)
	if trimmed == "" {
		return nil, ErrBadRequest
	}
	rows, err := s.Q.ListCommentsByArticleSlug(ctx, sql.NullString{String: trimmed, Valid: true})
	if err != nil {
		return nil, err
	}
	comments := make([]dto.Comment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, dto.Comment{
			ID:          row.ID,
			UserID:      row.UserID,
			UserName:    row.UserName,
			UserImage:   dto.NullString(row.UserImage),
			ProjectID:   nullInt64Ptr(row.ProjectID),
			ArticleSlug: dto.NullString(row.ArticleSlug),
			Type:        row.Type,
			Body:        row.Body,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		})
	}
	return comments, nil
}

func (s *service) ListUserComments(ctx context.Context, userID int64) ([]dto.Comment, error) {
	rows, err := s.Q.ListCommentsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	comments := make([]dto.Comment, 0, len(rows))
	for _, row := range rows {
		dtoComment, err := s.commentDTO(ctx, row)
		if err != nil {
			return nil, err
		}
		comments = append(comments, dtoComment)
	}
	return comments, nil
}

func (s *service) UpdateComment(ctx context.Context, id int64, userID int64, req dto.CommentUpdate) (dto.Comment, error) {
	body, err := validateCommentBody(req.Body)
	if err != nil {
		return dto.Comment{}, err
	}
	existing, err := s.Q.GetComment(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.Comment{}, ErrNotFound
	}
	if err != nil {
		return dto.Comment{}, err
	}
	if existing.UserID != userID {
		return dto.Comment{}, ErrForbidden
	}
	updated, err := s.Q.UpdateComment(ctx, id, userID, body)
	if err != nil {
		return dto.Comment{}, err
	}
	return s.commentDTO(ctx, updated)
}

func (s *service) DeleteComment(ctx context.Context, id int64, userID int64) error {
	rows, err := s.Q.DeleteComment(ctx, id, userID)
	if err != nil {
		return err
	}
	if rows == 0 {
		existing, err := s.Q.GetComment(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err == nil && existing.UserID != userID {
			return ErrForbidden
		}
		return ErrNotFound
	}
	return nil
}

func (s *service) commentDTO(ctx context.Context, comment db.Comment) (dto.Comment, error) {
	user, err := s.Q.GetUserByID(ctx, comment.UserID)
	if err != nil {
		return dto.Comment{}, err
	}
	return dto.Comment{
		ID:          comment.ID,
		UserID:      comment.UserID,
		UserName:    user.Name,
		UserImage:   dto.NullString(user.ImageUrl),
		ProjectID:   nullInt64Ptr(comment.ProjectID),
		ArticleSlug: dto.NullString(comment.ArticleSlug),
		Type:        comment.Type,
		Body:        comment.Body,
		CreatedAt:   comment.CreatedAt,
		UpdatedAt:   comment.UpdatedAt,
	}, nil
}

func validateCommentBody(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || len(trimmed) > maxCommentLength {
		return "", ErrBadRequest
	}
	return trimmed, nil
}

// nullInt64Ptr maps a nullable DB integer to the *int64 the DTO layer exposes.
func nullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
