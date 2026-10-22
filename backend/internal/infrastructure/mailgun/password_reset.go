package mailgun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
)

type PasswordResetMailer struct {
	apiKey    string
	domain    string
	from      string
	publicURL string
	client    *http.Client
}

func NewPasswordResetMailer(apiKey, domainName, from, publicURL string) (*PasswordResetMailer, error) {
	apiKey = strings.TrimSpace(apiKey)
	domainName = strings.TrimSpace(domainName)
	from = strings.TrimSpace(from)
	parsed, err := url.Parse(publicURL)
	if apiKey == "" || domainName == "" || strings.ContainsAny(domainName, "/\\?#") || from == "" || err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1"))) {
		return nil, errors.New("Mailgun and public URL configuration is invalid")
	}
	return &PasswordResetMailer{apiKey: apiKey, domain: domainName, from: from, publicURL: strings.TrimRight(publicURL, "/"), client: &http.Client{Timeout: 12 * time.Second}}, nil
}

func (m *PasswordResetMailer) SendPasswordReset(ctx context.Context, email, token string, expiresAt time.Time) error {
	link := m.publicURL + "/reset-password?token=" + url.QueryEscape(token)
	return m.send(ctx, email, "Redefinição de senha do Lynx OS", fmt.Sprintf("Use este link para redefinir sua senha: %s\n\nO link expira em 15 minutos. Se você não solicitou a redefinição, ignore esta mensagem.", link))
}

func (m *PasswordResetMailer) send(ctx context.Context, email, subject, body string) error {
	form := url.Values{}
	form.Set("from", m.from)
	form.Set("to", email)
	form.Set("subject", subject)
	form.Set("text", body)
	endpoint := "https://api.mailgun.net/v3/" + url.PathEscape(m.domain) + "/messages"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("could not create password reset email")
	}
	request.SetBasicAuth("api", m.apiKey)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := m.client.Do(request)
	if err != nil {
		return errors.New("password reset email delivery failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 16<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("password reset email delivery failed")
	}
	return nil
}

var _ domain.PasswordResetMailer = (*PasswordResetMailer)(nil)
