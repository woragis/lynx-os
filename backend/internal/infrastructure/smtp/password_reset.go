package smtp

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	netsmtp "net/smtp"
	"net/url"
	"strings"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
)

// PasswordResetMailer sends development messages to a local SMTP capture such
// as Mailpit. It deliberately accepts only loopback or the compose service name.
type PasswordResetMailer struct {
	address   string
	from      mail.Address
	publicURL string
}

func NewPasswordResetMailer(host string, port int, from, publicURL string) (*PasswordResetMailer, error) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" && host != "mailpit" {
		return nil, errors.New("development SMTP host must be local")
	}
	if port < 1 || port > 65535 {
		return nil, errors.New("development SMTP port is invalid")
	}
	fromAddress, err := mail.ParseAddress(strings.TrimSpace(from))
	if err != nil || strings.ContainsAny(fromAddress.Address, "\r\n") {
		return nil, errors.New("development SMTP sender is invalid")
	}
	parsedURL, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || parsedURL.Hostname() == "" || parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" ||
		(parsedURL.Scheme != "https" && !(parsedURL.Scheme == "http" && (parsedURL.Hostname() == "localhost" || parsedURL.Hostname() == "127.0.0.1" || parsedURL.Hostname() == "[::1]" || parsedURL.Hostname() == "::1"))) {
		return nil, errors.New("development public URL is invalid")
	}
	return &PasswordResetMailer{
		address: net.JoinHostPort(host, fmt.Sprint(port)), from: *fromAddress,
		publicURL: strings.TrimRight(strings.TrimSpace(publicURL), "/"),
	}, nil
}

func (m *PasswordResetMailer) SendPasswordReset(ctx context.Context, email, token string, expiresAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	to, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || to.Address != strings.TrimSpace(email) || strings.ContainsAny(to.Address, "\r\n") {
		return errors.New("password reset recipient is invalid")
	}
	link := m.publicURL + "/reset-password?token=" + url.QueryEscape(token)
	subject := mime.QEncoding.Encode("UTF-8", "Redefinição de senha do Lynx OS")
	body := fmt.Sprintf("Use este link para redefinir sua senha: %s\n\nO link expira em %s. Se você não solicitou a redefinição, ignore esta mensagem.\n",
		link, expiresAt.Local().Format("02/01/2006 15:04 MST"))
	body = strings.ReplaceAll(body, "\n", "\r\n")
	message := strings.Join([]string{
		"From: " + m.from.String(),
		"To: " + to.String(),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n")
	if err := netsmtp.SendMail(m.address, nil, m.from.Address, []string{to.Address}, []byte(message)); err != nil {
		return errors.New("development password reset message could not be captured")
	}
	return nil
}

var _ domain.PasswordResetMailer = (*PasswordResetMailer)(nil)
