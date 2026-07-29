package email

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
)

type Sender interface {
	SendContactFormNotification(ctx context.Context, name, email, message string) error
	SendContactConfirmation(ctx context.Context, name, email string) error
}

type SMTPSender struct {
	cfg config.EmailConfig
}

type disabledSender struct{}

func NewSender(cfg config.EmailConfig) Sender {
	if !cfg.EmailSendingEnabled {
		return disabledSender{}
	}
	return SMTPSender{cfg: cfg}
}

func (disabledSender) SendContactFormNotification(context.Context, string, string, string) error {
	return nil
}

func (disabledSender) SendContactConfirmation(context.Context, string, string) error {
	return nil
}

func (s SMTPSender) SendContactFormNotification(_ context.Context, name, email, message string) error {
	if strings.TrimSpace(s.cfg.EmailRecipient) == "" {
		return fmt.Errorf("EMAIL_RECIPIENT must not be empty")
	}

	subject := fmt.Sprintf("New Contact Form Submission from %s", name)
	body := fmt.Sprintf(`You have received a new contact form submission.

From: %s
Email: %s

Message:
%s

---
This email was sent from your portfolio website contact form.
`, name, email, message)

	return s.send(s.cfg.EmailRecipient, subject, body)
}

func (s SMTPSender) SendContactConfirmation(_ context.Context, name, email string) error {
	subject := "Thank you for contacting me!"
	body := fmt.Sprintf(`Hi %s,

Thank you for reaching out! I've received your message and will get back to you as soon as possible.

Best regards,
Alex Nelson

---
This is an automated confirmation email.
`, name)

	return s.send(email, subject, body)
}

func (s SMTPSender) send(to, subject, body string) error {
	addr := net.JoinHostPort(s.cfg.SMTPHost, fmt.Sprintf("%d", s.cfg.SMTPPort))
	auth := smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)

	header := strings.Builder{}
	fmt.Fprintf(&header, "From: %s\r\n", s.cfg.SMTPUsername)
	fmt.Fprintf(&header, "To: %s\r\n", to)
	fmt.Fprintf(&header, "Subject: %s\r\n", subject)
	fmt.Fprintf(&header, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	header.WriteString("MIME-Version: 1.0\r\n")
	header.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")

	return smtp.SendMail(addr, auth, s.cfg.SMTPUsername, []string{to}, []byte(header.String()+body))
}
