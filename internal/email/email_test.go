package email

import (
	"context"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
)

func TestDisabledSenderDoesNotSend(t *testing.T) {
	sender := NewSender(config.EmailConfig{EmailSendingEnabled: false})
	if err := sender.SendContactFormNotification(context.Background(), "Name", "sender@example.com", "Message"); err != nil {
		t.Fatalf("SendContactFormNotification() error = %v", err)
	}
	if err := sender.SendContactConfirmation(context.Background(), "Name", "sender@example.com"); err != nil {
		t.Fatalf("SendContactConfirmation() error = %v", err)
	}
}

func TestSMTPSenderRejectsMissingRecipient(t *testing.T) {
	sender := NewSender(config.EmailConfig{
		EmailSendingEnabled: true,
		SMTPHost:            "smtp.gmail.com",
		SMTPPort:            587,
		SMTPUsername:        "sender@example.com",
		SMTPPassword:        "password",
	})
	if err := sender.SendContactFormNotification(context.Background(), "Name", "sender@example.com", "Message"); err == nil {
		t.Fatal("expected missing EMAIL_RECIPIENT error")
	}
}
