package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type ContactsRepository interface {
	CreateContactMessage(ctx context.Context, name string, email string, message string) (domain.ContactMessage, error)
	DeleteContactMessage(ctx context.Context, id int64) (int64, error)
	ListContactMessages(ctx context.Context) ([]domain.ContactMessage, error)
	ListUnreadContactMessages(ctx context.Context) ([]domain.ContactMessage, error)
	MarkContactMessageRead(ctx context.Context, id int64) (domain.ContactMessage, error)
	MarkContactMessageReplied(ctx context.Context, id int64) (domain.ContactMessage, error)
}
