package saasclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

func TestContractV1ClientDiscoversAndExecutesWithIdempotency(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != contractPath || r.Header.Get("X-Lynx-API-Key") != "secret-test-key" {
			t.Fatalf("unexpected request path or authentication: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"contractVersion":"1","service":{"name":"Test","slug":"test"},"operations":{},"resources":{}}`))
			return
		}
		if r.Header.Get("Idempotency-Key") != "test-operation-key" {
			t.Fatalf("idempotency key missing: %q", r.Header.Get("Idempotency-Key"))
		}
		var command domain.Command
		if err := json.NewDecoder(r.Body).Decode(&command); err != nil || command.Operation != "bootstrap" {
			t.Fatalf("unexpected command %#v, error %v", command, err)
		}
		_, _ = w.Write([]byte(`{"contractVersion":"1","requestId":"remote-1","operation":"bootstrap","status":"completed","data":{"account":{"id":"account-1"}}}`))
	}))
	defer server.Close()
	client := NewContractV1Client(server.Client())
	capabilities, err := client.Discover(context.Background(), server.URL, "secret-test-key")
	if err != nil || capabilities.ContractVersion != "1" {
		t.Fatalf("discover failed: capabilities=%#v error=%v", capabilities, err)
	}
	response, err := client.Execute(context.Background(), server.URL, "secret-test-key", domain.Command{Operation: "bootstrap", Input: map[string]any{"name": "Test"}}, "test-operation-key")
	if err != nil || response.Status != "completed" || requests != 2 {
		t.Fatalf("execute failed: response=%#v requests=%d error=%v", response, requests, err)
	}
}

func TestContractV1ClientRejectsUnsupportedVersionAndRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"contractVersion":"2"}`))
	}))
	defer server.Close()
	if _, err := NewContractV1Client(server.Client()).Discover(context.Background(), server.URL, "key"); err == nil {
		t.Fatal("expected unsupported contract version")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/collect", http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := NewContractV1Client(redirect.Client()).Discover(context.Background(), redirect.URL, "must-not-forward"); err == nil {
		t.Fatal("expected redirect to be rejected")
	}
}
