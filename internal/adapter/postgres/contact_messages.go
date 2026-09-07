package postgres

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func (r *repositories) CreateContactMessage(ctx context.Context, name string, email string, message string) (domain.ContactMessage, error) {
	row, err := r.q.CreateContactMessage(ctx, name, email, message)
	return fromContactMessage(row), repositoryError(err)
}

func (r *repositories) DeleteContactMessage(ctx context.Context, id int64) (int64, error) {
	row, err := r.q.DeleteContactMessage(ctx, id)
	return row, repositoryError(err)
}

func (r *repositories) ListContactMessages(ctx context.Context) ([]domain.ContactMessage, error) {
	row, err := r.q.ListContactMessages(ctx)
	return mapRows(row, fromContactMessage), repositoryError(err)
}

func (r *repositories) ListUnreadContactMessages(ctx context.Context) ([]domain.ContactMessage, error) {
	row, err := r.q.ListUnreadContactMessages(ctx)
	return mapRows(row, fromContactMessage), repositoryError(err)
}

func (r *repositories) MarkContactMessageRead(ctx context.Context, id int64) (domain.ContactMessage, error) {
	row, err := r.q.MarkContactMessageRead(ctx, id)
	return fromContactMessage(row), repositoryError(err)
}

func (r *repositories) MarkContactMessageReplied(ctx context.Context, id int64) (domain.ContactMessage, error) {
	row, err := r.q.MarkContactMessageReplied(ctx, id)
	return fromContactMessage(row), repositoryError(err)
}
