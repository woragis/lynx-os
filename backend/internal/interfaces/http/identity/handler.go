package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"

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
	service      *application.Service
	secureCookie bool
}

func NewHandler(service *application.Service, secureCookie bool) *Handler {
	return &Handler{service: service, secureCookie: secureCookie}
}

func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()
	router.Get("/csrf", h.issueCSRF)
	router.Post("/login", h.login)
	router.Post("/refresh", h.refresh)
	router.Post("/logout", h.logout)
	router.Group(func(protected chi.Router) {
		protected.Use(h.authenticate)
		protected.Use(h.requireCSRF)
		protected.Get("/me", h.me)
		protected.Get("/sessions", h.listSessions)
		protected.Delete("/sessions/{sessionID}", h.revokeSession)
		protected.Post("/sessions/revoke-all", h.revokeAll)
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
	if err := h.service.Logout(r.Context(), principal.User.ID, chi.URLParam(r, "sessionID")); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error", "Could not revoke session"))
		return
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
	if err != nil {
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
