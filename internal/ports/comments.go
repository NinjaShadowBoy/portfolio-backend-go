package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type CommentsRepository interface {
	CountCommentsForAdmin(ctx context.Context, filter domain.CommentFilter) (int32, error)
	CreateComment(ctx context.Context, body string, userID int64, projectID *int64, articleSlug *string, type_ string) (domain.Comment, error)
	DeleteComment(ctx context.Context, id int64, userID int64) (int64, error)
	DeleteCommentByID(ctx context.Context, id int64) (int64, error)
	GetComment(ctx context.Context, id int64) (domain.Comment, error)
	ListCommentsForAdmin(ctx context.Context, filter domain.CommentFilter) ([]domain.AdminComment, error)
	ListCommentsByArticleSlug(ctx context.Context, articleSlug *string) ([]domain.CommentWithAuthor, error)
	ListCommentsByProjectID(ctx context.Context, projectID int64) ([]domain.CommentWithAuthor, error)
	ListCommentsByUserID(ctx context.Context, userID int64) ([]domain.Comment, error)
	ReassignCommentsToUser(ctx context.Context, targetUserID int64, sourceUserID int64) error
	UpdateComment(ctx context.Context, id int64, userID int64, body string) (domain.Comment, error)
}
