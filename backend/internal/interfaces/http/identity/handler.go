package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	application "github.com/lynx/lynx-os/backend/internal/application/identity"
	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
)

const (
	accessCookieName  = "lynx_access"
	refreshCookieName = "lynx_refresh"
	csrfCookieName    = "lynx_csrf"
	refreshCookiePath = "/api/v1/auth"
)

type Handler struct {
	service       *application.Service
	secureCookie  bool
	telegramToken string
}

func NewHandler(service *application.Service, secureCookie bool) *Handler {
	return &Handler{service: service, secureCookie: secureCookie}
}

func NewHandlerWithTelegram(service *application.Service, secureCookie bool, telegramToken string) *Handler {
	return &Handler{service: service, secureCookie: secureCookie, telegramToken: telegramToken}
}

func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()
	router.Get("/csrf", h.issueCSRF)
	router.Post("/login", h.login)
	router.Post("/refresh", h.refresh)
	router.Post("/logout", h.logout)
	router.Post("/password/forgot", h.forgotPassword)
	router.Post("/password/reset", h.resetPassword)
	router.Post("/invitations/accept", h.acceptInvitation)
	router.Post("/telegram/link", h.linkTelegram)
	router.Group(func(protected chi.Router) {
		protected.Use(h.authenticate)
		protected.Use(h.requireCSRF)
		protected.Get("/me", h.me)
		protected.Get("/sessions", h.listSessions)
		protected.Delete("/sessions/{sessionID}", h.revokeSession)
		protected.Post("/sessions/revoke-all", h.revokeAll)
		protected.Post("/telegram/link-code", h.telegramLinkCode)
		protected.With(RequirePermission(domain.PermissionManageAdmins)).Get("/admins", h.listAdmins)
		protected.With(RequirePermission(domain.PermissionManageAdmins)).Post("/admins/invitations", h.inviteAdmin)
		protected.With(RequirePermission(domain.PermissionManageAdmins)).Patch("/admins/{id}", h.updateAdmin)
	})
	return router
}

func (h *Handler) Middleware() func(http.Handler) http.Handler { return h.authenticate }

func (h *Handler) ProtectedMiddleware(next http.Handler) http.Handler {
	return h.authenticate(h.requireCSRF(next))
}

func CurrentPrincipal(r *http.Request) (application.PublicUser, domain.Session, bool) {
	value, ok := r.Context().Value(contextKey{}).(principal)
	return value.User, value.Session, ok
}

func RequireRoles(roles ...domain.Role) func(http.Handler) http.Handler {
	allowed := make(map[domain.Role]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, _, ok := CurrentPrincipal(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized", "Authentication required"))
				return
			}
			if _, ok := allowed[user.Role]; !ok {
				writeJSON(w, http.StatusForbidden, errorBody("forbidden", "Insufficient permission"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequirePermission(permission domain.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, _, ok := CurrentPrincipal(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized", "Authentication required"))
				return
			}
			if !user.Role.Allows(permission) {
				writeJSON(w, http.StatusForbidden, errorBody("forbidden", "Insufficient permission"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *Handler) issueCSRF(w http.ResponseWriter, _ *http.Request) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not initialize request security"))
		return
	}
	token := base64.RawURLEncoding.EncodeToString(value)
	h.setCookie(w, csrfCookieName, token, "/", 30*60, false)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"csrfToken": token}})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, errorBody("csrf_failed", "Request could not be verified"))
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	tokens, err := h.service.Login(r.Context(), input.Email, input.Password, clientIP(r), r.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, errorBody("invalid_credentials", "E-mail ou senha inválidos"))
		case errors.Is(err, domain.ErrTooManyAttempts):
			writeJSON(w, http.StatusTooManyRequests, errorBody("too_many_attempts", "Muitas tentativas. Aguarde antes de tentar novamente."))
		case errors.Is(err, domain.ErrRateLimiterUnavailable):
			writeJSON(w, http.StatusServiceUnavailable, errorBody("auth_unavailable", "Autenticação temporariamente indisponível"))
		default:
			writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Não foi possível iniciar a sessão"))
		}
		return
	}
	h.setSessionCookies(w, tokens)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"user": tokens.User, "expiresIn": tokens.ExpiresIn, "sessionId": tokens.Session.ID}})
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, errorBody("csrf_failed", "Request could not be verified"))
		return
	}
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, errorBody("session_expired", "Sessão expirada"))
		return
	}
	tokens, err := h.service.Refresh(r.Context(), cookie.Value, clientIP(r), r.UserAgent())
	if err != nil {
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, errorBody("session_expired", "Sessão expirada ou revogada"))
		return
	}
	h.setSessionCookies(w, tokens)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"user": tokens.User, "expiresIn": tokens.ExpiresIn, "sessionId": tokens.Session.ID}})
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, errorBody("csrf_failed", "Request could not be verified"))
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if err := h.service.RequestPasswordReset(r.Context(), input.Email, clientIP(r)); err != nil {
		// A resposta é igual exista ou não a conta; o log não inclui o endereço nem o token.
		slog.Error("administrator password reset request failed", "reason", err.Error())
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, errorBody("csrf_failed", "Request could not be verified"))
		return
	}
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if err := h.service.ResetPassword(r.Context(), input.Token, input.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_reset_token", "Link inválido ou expirado. Solicite outro link."))
		return
	}
	h.clearSessionCookies(w)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "password_changed"}})
}

func (h *Handler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, errorBody("csrf_failed", "Request could not be verified"))
		return
	}
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if err := h.service.AcceptInvitation(r.Context(), input.Token, input.Password); err != nil {
		if errors.Is(err, domain.ErrEmailAlreadyUsed) {
			writeJSON(w, http.StatusConflict, errorBody("email_already_used", "Esta conta já foi ativada"))
			return
		}
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_invitation", "Convite inválido ou expirado"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"status": "account_created"}})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, errorBody("csrf_failed", "Request could not be verified"))
		return
	}
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		if err := h.service.LogoutRefresh(r.Context(), cookie.Value); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not end session"))
			return
		}
	}
	h.clearSessionCookies(w)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "signed_out"}})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	principal := principalFromContext(r)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"user": principal.User, "sessionId": principal.Session.ID}})
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	principal := principalFromContext(r)
	sessions, err := h.service.Sessions(r.Context(), principal.User.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not list sessions"))
		return
	}
	data := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		data = append(data, map[string]any{
			"id": session.ID, "createdAt": session.CreatedAt, "lastUsedAt": session.LastUsedAt,
			"expiresAt": session.ExpiresAt, "revokedAt": session.RevokedAt,
			"ipAddress": session.IPAddress, "userAgent": session.UserAgent,
			"current": session.ID == principal.Session.ID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (h *Handler) revokeSession(w http.ResponseWriter, r *http.Request) {
	principal := principalFromContext(r)
	sessionID := chi.URLParam(r, "sessionID")
	if err := h.service.Logout(r.Context(), principal.User.ID, sessionID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not revoke session"))
		return
	}
	if sessionID == principal.Session.ID {
		h.clearSessionCookies(w)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

func (h *Handler) revokeAll(w http.ResponseWriter, r *http.Request) {
	principal := principalFromContext(r)
	if err := h.service.LogoutAll(r.Context(), principal.User.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not revoke sessions"))
		return
	}
	h.clearSessionCookies(w)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

func (h *Handler) listAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.service.Admins(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not list administrators"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": admins})
}

func (h *Handler) inviteAdmin(w http.ResponseWriter, r *http.Request) {
	user, _, _ := CurrentPrincipal(r)
	var input struct {
		Email       string      `json:"email"`
		DisplayName string      `json:"displayName"`
		Role        domain.Role `json:"role"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	token, err := h.service.Invite(r.Context(), user.ID, input.Email, input.DisplayName, input.Role)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrEmailAlreadyUsed):
			writeJSON(w, http.StatusConflict, errorBody("email_already_used", "Já existe uma conta para este e-mail"))
		case errors.Is(err, domain.ErrInvalidRole), errors.Is(err, application.ErrInvalidAccount):
			writeJSON(w, http.StatusBadRequest, errorBody("invalid_invitation", "Confira os dados e o papel solicitado"))
		default:
			writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not create invitation"))
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"token": token, "expiresIn": "24h"}})
}

func (h *Handler) updateAdmin(w http.ResponseWriter, r *http.Request) {
	actor, _, _ := CurrentPrincipal(r)
	var input struct {
		DisplayName string      `json:"displayName"`
		Role        domain.Role `json:"role"`
		Active      *bool       `json:"active"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if input.Active == nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_admin", "Administrator details are invalid"))
		return
	}
	admin, err := h.service.UpdateAdmin(r.Context(), actor.ID, chi.URLParam(r, "id"), input.DisplayName, input.Role, *input.Active)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, errorBody("admin_not_found", "Administrator was not found"))
		case errors.Is(err, domain.ErrCannotModifyOwner), errors.Is(err, domain.ErrCannotDeactivateSelf):
			writeJSON(w, http.StatusConflict, errorBody("admin_change_not_allowed", "This administrator cannot be changed"))
		case errors.Is(err, domain.ErrInvalidRole), errors.Is(err, application.ErrInvalidAccount):
			writeJSON(w, http.StatusBadRequest, errorBody("invalid_admin", "Administrator details are invalid"))
		default:
			writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not update administrator"))
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": admin})
}

func (h *Handler) telegramLinkCode(w http.ResponseWriter, r *http.Request) {
	user, _, _ := CurrentPrincipal(r)
	code, err := h.service.CreateTelegramLinkCode(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not create Telegram link code"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"code": code, "expiresIn": "10m"}})
}

func (h *Handler) linkTelegram(w http.ResponseWriter, r *http.Request) {
	if h.telegramToken == "" || !constantBearer(r, h.telegramToken) {
		writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized", "Authentication required"))
		return
	}
	var input struct {
		TelegramUserID string `json:"telegramUserId"`
		Code           string `json:"code"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if err := h.service.LinkTelegramIdentity(r.Context(), input.TelegramUserID, input.Code); err != nil {
		if errors.Is(err, domain.ErrTelegramAlreadyLinked) {
			writeJSON(w, http.StatusConflict, errorBody("telegram_already_linked", "Esta conta Telegram já está vinculada"))
			return
		}
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_link_code", "Código inválido ou expirado"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "linked"}})
}

func constantBearer(r *http.Request, expected string) bool {
	provided := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(provided) <= len(prefix) || !strings.EqualFold(provided[:len(prefix)], prefix) {
		return false
	}
	value := provided[len(prefix):]
	return len(value) == len(expected) && subtle.ConstantTimeCompare([]byte(value), []byte(expected)) == 1
}

func (h *Handler) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(accessCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized", "Authentication required"))
			return
		}
		user, session, err := h.service.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized", "Authentication required"))
			return
		}
		next.ServeHTTP(w, withPrincipal(r, principal{User: user, Session: session}))
	})
}

func (h *Handler) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.validCSRF(r) {
			writeJSON(w, http.StatusForbidden, errorBody("csrf_failed", "Request could not be verified"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) validCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || len(cookie.Value) < 40 {
		return false
	}
	provided := r.Header.Get("X-CSRF-Token")
	return len(provided) == len(cookie.Value) && subtle.ConstantTimeCompare([]byte(provided), []byte(cookie.Value)) == 1
}

func (h *Handler) setSessionCookies(w http.ResponseWriter, tokens application.Tokens) {
	h.setCookie(w, accessCookieName, tokens.AccessToken, "/", int(tokens.ExpiresIn), true)
	h.setCookie(w, refreshCookieName, tokens.RefreshToken, refreshCookiePath, 14*24*60*60, true)
	csrf := make([]byte, 32)
	if _, err := rand.Read(csrf); err == nil {
		h.setCookie(w, csrfCookieName, base64.RawURLEncoding.EncodeToString(csrf), "/", 14*24*60*60, false)
	}
}

func (h *Handler) clearSessionCookies(w http.ResponseWriter) {
	h.setCookie(w, accessCookieName, "", "/", -1, true)
	h.setCookie(w, refreshCookieName, "", refreshCookiePath, -1, true)
	h.setCookie(w, csrfCookieName, "", "/", -1, false)
}

func (h *Handler) setCookie(w http.ResponseWriter, name, value, path string, maxAge int, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: path, MaxAge: maxAge,
		HttpOnly: httpOnly, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode,
	})
}

type principal struct {
	User    application.PublicUser
	Session domain.Session
}
type contextKey struct{}

func withPrincipal(r *http.Request, value principal) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), contextKey{}, value))
}
func principalFromContext(r *http.Request) principal {
	value, _ := r.Context().Value(contextKey{}).(principal)
	return value
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_request", "Request body is invalid"))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_request", "Request body must contain one JSON object"))
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	return ip
}
func errorBody(code, message string) map[string]any {
	return map[string]any{"error": map[string]string{"code": code, "message": message}}
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
