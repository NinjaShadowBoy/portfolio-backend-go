package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

const maxCommentLength = 2000

func (s *service) CreateComment(ctx context.Context, userID int64, req dto.CommentCreate) (dto.Comment, error) {
	body, err := validateCommentBody(req.Body)
	if err != nil {
		return dto.Comment{}, err
	}
	if err := s.requireProject(ctx, req.ProjectID); err != nil {
		return dto.Comment{}, err
	}
	comment, err := s.Q.CreateComment(ctx, body, userID, req.ProjectID)
	if err != nil {
		return dto.Comment{}, err
	}
	return s.commentDTO(ctx, comment)
}

func (s *service) ListProjectComments(ctx context.Context, projectID int64) ([]dto.Comment, error) {
	rows, err := s.Q.ListCommentsByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	comments := make([]dto.Comment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, dto.Comment{
			ID:        row.ID,
			UserID:    row.UserID,
			UserName:  row.UserName,
			UserImage: dto.NullString(row.UserImage),
			ProjectID: row.ProjectID,
			Body:      row.Body,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
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
		ID:        comment.ID,
		UserID:    comment.UserID,
		UserName:  user.Name,
		UserImage: dto.NullString(user.ImageUrl),
		ProjectID: comment.ProjectID,
		Body:      comment.Body,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	}, nil
}

func validateCommentBody(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || len(trimmed) > maxCommentLength {
		return "", ErrBadRequest
	}
	return trimmed, nil
}
