package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"

	application "github.com/lynx/lynx-os/backend/internal/application/identity"
	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
)

func TestPermissionRoleMatrix(t *testing.T) {
	tests := []struct {
		role       domain.Role
		permission domain.Permission
		allowed    bool
	}{
		{domain.RoleOwner, domain.PermissionViewIntegrations, true},
		{domain.RoleAdmin, domain.PermissionViewIntegrations, true},
		{domain.RoleOperator, domain.PermissionViewIntegrations, true},
		{domain.RoleViewer, domain.PermissionViewIntegrations, true},
		{domain.RoleOwner, domain.PermissionManageIntegrations, true},
		{domain.RoleAdmin, domain.PermissionManageIntegrations, true},
		{domain.RoleOperator, domain.PermissionManageIntegrations, false},
		{domain.RoleViewer, domain.PermissionManageIntegrations, false},
		{domain.RoleOwner, domain.PermissionRunOperations, true},
		{domain.RoleAdmin, domain.PermissionRunOperations, true},
		{domain.RoleOperator, domain.PermissionRunOperations, true},
		{domain.RoleViewer, domain.PermissionRunOperations, false},
		{domain.RoleOwner, domain.PermissionManageAdmins, true},
		{domain.RoleAdmin, domain.PermissionManageAdmins, false},
		{domain.RoleOperator, domain.PermissionManageAdmins, false},
		{domain.RoleViewer, domain.PermissionManageAdmins, false},
		{domain.Role("unknown"), domain.PermissionViewIntegrations, false},
		{domain.RoleOwner, domain.Permission("unknown"), false},
	}

	for _, test := range tests {
		t.Run(string(test.role)+"/"+string(test.permission), func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request = withPrincipal(request, principal{User: application.PublicUser{ID: "user", Role: test.role}})
			response := httptest.NewRecorder()

			RequirePermission(test.permission)(next).ServeHTTP(response, request)

			if test.allowed && (!called || response.Code != http.StatusNoContent) {
				t.Fatalf("expected permission to be granted, got status %d", response.Code)
			}
			if !test.allowed && (called || response.Code != http.StatusForbidden) {
				t.Fatalf("expected permission to be denied, got status %d", response.Code)
			}
		})
	}
}

func TestRequirePermissionRejectsMissingPrincipal(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	called := false
	RequirePermission(domain.PermissionViewIntegrations)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})).ServeHTTP(response, request)
	if called || response.Code != http.StatusUnauthorized {
		t.Fatalf("missing principal must be rejected, got status %d", response.Code)
	}
}
