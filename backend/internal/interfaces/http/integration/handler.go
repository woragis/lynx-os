package integration

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	application "github.com/lynx/lynx-os/backend/internal/application/integration"
	identitydomain "github.com/lynx/lynx-os/backend/internal/domain/identity"
	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
	identityhttp "github.com/lynx/lynx-os/backend/internal/interfaces/http/identity"
)

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()
	router.With(identityhttp.RequirePermission(identitydomain.PermissionViewIntegrations)).Get("/", h.list)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionViewIntegrations)).Get("/{id}/capabilities", h.capabilities)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionViewIntegrations)).Get("/{id}/operations", h.operations)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionViewIntegrations)).Get("/{id}/operations/{key}", h.operation)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionManageIntegrations)).Post("/", h.create)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionManageIntegrations)).Put("/{id}", h.update)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionManageIntegrations)).Patch("/{id}/active", h.setActive)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionManageIntegrations)).Put("/{id}/api-key", h.rotateKey)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionRunOperations)).Post("/{id}/operations", h.execute)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionRunOperations)).Post("/{id}/operations/{key}/retry", h.retryOperation)
	router.With(identityhttp.RequirePermission(identitydomain.PermissionManageIntegrations)).Delete("/{id}", h.revoke)
	return router
}

func (h *Handler) capabilities(w http.ResponseWriter, r *http.Request) {
	capabilities, err := h.service.Capabilities(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, domain.ErrIntegrationNotFound) {
		writeJSON(w, http.StatusNotFound, apiError("integration_not_found", "Integration was not found"))
		return
	}
	if errors.Is(err, application.ErrIntegrationRevoked) {
		writeJSON(w, http.StatusConflict, apiError("integration_inactive", "Integration is inactive or revoked"))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiError("saas_unavailable", "Could not load SaaS capabilities"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": capabilities})
}

type createRequest struct {
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	BaseURL string `json:"baseUrl"`
	AppURL  string `json:"appUrl"`
	APIKey  string `json:"apiKey"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input createRequest
	if !decodeBody(w, r, &input) {
		return
	}
	item, err := h.service.Create(r.Context(), application.CreateInput{Name: input.Name, Slug: input.Slug, BaseURL: input.BaseURL, AppURL: input.AppURL, APIKey: input.APIKey})
	if err != nil {
		if errors.Is(err, application.ErrInvalidIntegration) {
			writeJSON(w, http.StatusBadRequest, apiError("invalid_integration", "Integration fields are invalid"))
			return
		}
		if errors.Is(err, application.ErrSaaSValidation) {
			writeJSON(w, http.StatusBadGateway, apiError("saas_unavailable", "SaaS contract validation failed"))
			return
		}
		writeJSON(w, http.StatusInternalServerError, apiError("internal_error", "Could not save integration"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": item})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var input createRequest
	if !decodeBody(w, r, &input) {
		return
	}
	item, err := h.service.Update(r.Context(), chi.URLParam(r, "id"), application.CreateInput{Name: input.Name, Slug: input.Slug, BaseURL: input.BaseURL, AppURL: input.AppURL})
	if err != nil {
		h.writeIntegrationError(w, err, "Could not update integration")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}
func (h *Handler) setActive(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Active bool `json:"active"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	item, err := h.service.SetActive(r.Context(), chi.URLParam(r, "id"), input.Active)
	if err != nil {
		h.writeIntegrationError(w, err, "Could not change integration state")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) operations(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	items, err := h.service.ListOperations(r.Context(), chi.URLParam(r, "id"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError("internal_error", "Could not list operations"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}
func (h *Handler) operation(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetOperation(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "key"))
	if errors.Is(err, domain.ErrIntegrationNotFound) {
		writeJSON(w, http.StatusNotFound, apiError("operation_not_found", "Operation was not found"))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError("internal_error", "Could not read operation"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}
func (h *Handler) retryOperation(w http.ResponseWriter, r *http.Request) {
	actor, _, _ := identityhttp.CurrentPrincipal(r)
	response, err := h.service.RetryOperation(r.Context(), chi.URLParam(r, "id"), actor.ID, middleware.GetReqID(r.Context()), chi.URLParam(r, "key"))
	if err != nil {
		if errors.Is(err, domain.ErrIntegrationNotFound) {
			writeJSON(w, http.StatusNotFound, apiError("operation_not_found", "Operation was not found"))
			return
		}
		if errors.Is(err, domain.ErrOperationInProgress) {
			writeJSON(w, http.StatusConflict, apiError("operation_in_progress", "Operation is still in progress"))
			return
		}
		writeJSON(w, http.StatusBadGateway, apiError("retry_failed", "Could not retry operation"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": response})
}

func (h *Handler) writeIntegrationError(w http.ResponseWriter, err error, message string) {
	if errors.Is(err, domain.ErrIntegrationNotFound) {
		writeJSON(w, http.StatusNotFound, apiError("integration_not_found", "Integration was not found"))
		return
	}
	if errors.Is(err, application.ErrIntegrationRevoked) {
		writeJSON(w, http.StatusConflict, apiError("integration_revoked", "Integration is revoked"))
		return
	}
	if errors.Is(err, application.ErrInvalidIntegration) {
		writeJSON(w, http.StatusBadRequest, apiError("invalid_integration", "Integration fields are invalid"))
		return
	}
	if errors.Is(err, application.ErrSaaSValidation) {
		writeJSON(w, http.StatusBadGateway, apiError("saas_unavailable", "SaaS contract validation failed"))
		return
	}
	writeJSON(w, http.StatusInternalServerError, apiError("internal_error", message))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError("internal_error", "Could not list integrations"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) rotateKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		APIKey string `json:"apiKey"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	item, err := h.service.RotateKey(r.Context(), chi.URLParam(r, "id"), input.APIKey)
	if err != nil {
		if errors.Is(err, domain.ErrIntegrationNotFound) {
			writeJSON(w, http.StatusNotFound, apiError("integration_not_found", "Integration was not found"))
			return
		}
		if errors.Is(err, application.ErrIntegrationRevoked) {
			writeJSON(w, http.StatusConflict, apiError("integration_revoked", "Revoked integrations cannot rotate keys"))
			return
		}
		if errors.Is(err, application.ErrInvalidIntegration) {
			writeJSON(w, http.StatusBadRequest, apiError("invalid_api_key", "API key is required"))
			return
		}
		if errors.Is(err, application.ErrSaaSValidation) {
			writeJSON(w, http.StatusBadGateway, apiError("saas_unavailable", "SaaS contract validation failed"))
			return
		}
		writeJSON(w, http.StatusInternalServerError, apiError("internal_error", "Could not rotate integration key"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Revoke(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, domain.ErrIntegrationNotFound) {
		writeJSON(w, http.StatusNotFound, apiError("integration_not_found", "Integration was not found"))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError("internal_error", "Could not revoke integration"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) execute(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Operation string         `json:"operation"`
		Input     map[string]any `json:"input"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	actor, _, _ := identityhttp.CurrentPrincipal(r)
	response, err := h.service.Execute(r.Context(), chi.URLParam(r, "id"), actor.ID, middleware.GetReqID(r.Context()), domain.Command{
		Operation: input.Operation,
		Input:     input.Input,
	}, r.Header.Get("Idempotency-Key"))
	if err != nil {
		if errors.Is(err, domain.ErrIntegrationNotFound) {
			writeJSON(w, http.StatusNotFound, apiError("integration_not_found", "Integration was not found"))
			return
		}
		if errors.Is(err, application.ErrIntegrationRevoked) {
			writeJSON(w, http.StatusConflict, apiError("integration_inactive", "Integration is inactive or revoked"))
			return
		}
		if errors.Is(err, application.ErrInvalidOperation) {
			writeJSON(w, http.StatusUnprocessableEntity, apiError("invalid_operation", "Operation is invalid or not advertised by the SaaS"))
			return
		}
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			writeJSON(w, http.StatusConflict, apiError("idempotency_conflict", "Idempotency key was already used with a different request"))
			return
		}
		if errors.Is(err, domain.ErrOperationInProgress) {
			writeJSON(w, http.StatusConflict, apiError("operation_in_progress", "Operation with this idempotency key is still in progress"))
			return
		}
		writeJSON(w, http.StatusBadGateway, apiError("saas_operation_failed", "SaaS operation failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": response})
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid_request", "Request body is invalid"))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, apiError("invalid_request", "Request body must contain one JSON object"))
		return false
	}
	return true
}

func apiError(code, message string) map[string]any {
	return map[string]any{"error": map[string]string{"code": code, "message": message}}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
