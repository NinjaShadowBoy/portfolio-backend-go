package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func (s *UseCases) SubmitContact(ctx context.Context, req dto.Contact) (dto.ContactMessage, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Message) == "" {
		return dto.ContactMessage{}, ErrBadRequest
	}
	message, err := s.repos.Contacts().CreateContactMessage(ctx, req.Name, req.Email, req.Message)
	if err != nil {
		return dto.ContactMessage{}, err
	}

	if s.emails != nil {
		_ = s.emails.SendContactFormNotification(ctx, req.Name, req.Email, req.Message)
		_ = s.emails.SendContactConfirmation(ctx, req.Name, req.Email)
	}

	return contactMessageDTO(message), nil
}

func (s *UseCases) ListContactMessages(ctx context.Context) ([]dto.ContactMessage, error) {
	messages, err := s.repos.Contacts().ListContactMessages(ctx)
	if err != nil {
		return nil, err
	}
	return contactMessagesDTO(messages), nil
}

func (s *UseCases) ListUnreadContactMessages(ctx context.Context) ([]dto.ContactMessage, error) {
	messages, err := s.repos.Contacts().ListUnreadContactMessages(ctx)
	if err != nil {
		return nil, err
	}
	return contactMessagesDTO(messages), nil
}

func (s *UseCases) MarkContactRead(ctx context.Context, id int64) (dto.ContactMessage, error) {
	message, err := s.repos.Contacts().MarkContactMessageRead(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.ContactMessage{}, ErrNotFound
	}
	if err != nil {
		return dto.ContactMessage{}, err
	}
	return contactMessageDTO(message), nil
}

func (s *UseCases) MarkContactReplied(ctx context.Context, id int64) (dto.ContactMessage, error) {
	message, err := s.repos.Contacts().MarkContactMessageReplied(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.ContactMessage{}, ErrNotFound
	}
	if err != nil {
		return dto.ContactMessage{}, err
	}
	return contactMessageDTO(message), nil
}

func (s *UseCases) DeleteContactMessage(ctx context.Context, id int64) error {
	rows, err := s.repos.Contacts().DeleteContactMessage(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func contactMessageDTO(message domain.ContactMessage) dto.ContactMessage {
	return dto.ContactMessage{
		ID:          message.ID,
		Name:        message.Name,
		Email:       message.Email,
		Message:     message.Message,
		SubmittedAt: message.SubmittedAt,
		IsRead:      message.IsRead,
		IsReplied:   message.IsReplied,
	}
}

func contactMessagesDTO(messages []domain.ContactMessage) []dto.ContactMessage {
	items := make([]dto.ContactMessage, 0, len(messages))
	for _, message := range messages {
		items = append(items, contactMessageDTO(message))
	}
	return items
}
