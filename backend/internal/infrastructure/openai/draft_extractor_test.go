package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

func TestDraftExtractorSendsStrictSchemaAndParsesResponse(t *testing.T) {
	want := domain.Draft{IntegrationName: ptr("easybill"), AccountName: ptr("Escritório Azul")}
	encoded, _ := json.Marshal(want)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Method != http.MethodPost {
			t.Errorf("unexpected endpoint %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("authorization header missing")
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request["store"] != false {
			t.Error("conversation request must opt out of stored responses")
		}
		format := request["text"].(map[string]any)["format"].(map[string]any)
		if format["type"] != "json_schema" || format["strict"] != true {
			t.Error("strict JSON schema response format not enabled")
		}
		response, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": string(encoded)}}}}})
		_, _ = w.Write(response)
	}))
	defer server.Close()
	extractor := NewDraftExtractor("test-key", "gpt-6-luna")
	extractor.endpoint = server.URL + "/v1/responses"
	extractor.client = server.Client()
	got, err := extractor.Extract(context.Background(), "EasyBill, escritório Azul", domain.Draft{}, []domain.System{{ID: "1", Name: "EasyBill", Slug: "easybill"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.IntegrationName == nil || *got.IntegrationName != "easybill" || got.AccountName == nil || *got.AccountName != "Escritório Azul" {
		t.Fatalf("unexpected draft: %#v", got)
	}
}

func TestDraftExtractorRejectsIncompleteResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"incomplete","output":[]}`))
	}))
	defer server.Close()
	e := NewDraftExtractor("test-key", "")
	e.endpoint = server.URL
	e.client = server.Client()
	_, err := e.Extract(context.Background(), "oi", domain.Draft{}, nil)
	if err == nil || !strings.Contains(err.Error(), "complete draft") {
		t.Fatalf("expected incomplete result error, got %v", err)
	}
}

func ptr(value string) *string { return &value }
