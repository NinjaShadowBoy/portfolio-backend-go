package postgres

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func (r *repositories) CreateComment(ctx context.Context, body string, userID int64, projectID *int64, articleSlug *string, type_ string) (domain.Comment, error) {
	row, err := r.q.CreateComment(ctx, body, userID, nullInt64(projectID), nullString(articleSlug), type_)
	return fromComment(row), repositoryError(err)
}

func (r *repositories) DeleteComment(ctx context.Context, id int64, userID int64) (int64, error) {
	row, err := r.q.DeleteComment(ctx, id, userID)
	return row, repositoryError(err)
}

func (r *repositories) GetComment(ctx context.Context, id int64) (domain.Comment, error) {
	row, err := r.q.GetComment(ctx, id)
	return fromComment(row), repositoryError(err)
}

func (r *repositories) ListCommentsByArticleSlug(ctx context.Context, articleSlug *string) ([]domain.CommentWithAuthor, error) {
	row, err := r.q.ListCommentsByArticleSlug(ctx, nullString(articleSlug))
	return mapRows(row, fromListCommentsByArticleSlugRow), repositoryError(err)
}

func (r *repositories) ListCommentsByProjectID(ctx context.Context, projectID int64) ([]domain.CommentWithAuthor, error) {
	row, err := r.q.ListCommentsByProjectID(ctx, projectID)
	return mapRows(row, fromListCommentsByProjectIDRow), repositoryError(err)
}

func (r *repositories) ListCommentsByUserID(ctx context.Context, userID int64) ([]domain.Comment, error) {
	row, err := r.q.ListCommentsByUserID(ctx, userID)
	return mapRows(row, fromComment), repositoryError(err)
}

func (r *repositories) ReassignCommentsToUser(ctx context.Context, targetUserID int64, sourceUserID int64) error {
	return repositoryError(r.q.ReassignCommentsToUser(ctx, targetUserID, sourceUserID))
}

func (r *repositories) UpdateComment(ctx context.Context, id int64, userID int64, body string) (domain.Comment, error) {
	row, err := r.q.UpdateComment(ctx, id, userID, body)
	return fromComment(row), repositoryError(err)
}
