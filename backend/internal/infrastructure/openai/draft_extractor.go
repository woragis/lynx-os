package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

type DraftExtractor struct {
	key, model string
	client     *http.Client
	endpoint   string
}

func NewDraftExtractor(key, model string) *DraftExtractor {
	if strings.TrimSpace(model) == "" {
		model = "gpt-6-luna"
	}
	return &DraftExtractor{key: key, model: model, client: &http.Client{Timeout: 45 * time.Second}, endpoint: "https://api.openai.com/v1/responses"}
}

func (e *DraftExtractor) Extract(ctx context.Context, message string, previous domain.Draft, systems []domain.System) (domain.Draft, error) {
	// Build the JSON Schema as raw JSON so union types retain their array shape.
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"integrationName":{"type":["string","null"]},"accountName":{"type":["string","null"]},"accountTaxId":{"type":["string","null"]},"accountEmail":{"type":["string","null"]},"accountAddress":{"type":["string","null"]},"customerTradeName":{"type":["string","null"]},"customerLegalName":{"type":["string","null"]},"customerTaxId":{"type":["string","null"]},"customerEmail":{"type":["string","null"]},"customerPhone":{"type":["string","null"]},"customerTaxRegime":{"type":["string","null"]},"relationshipType":{"type":["string","null"]},"customerStatus":{"type":["string","null"]},"userName":{"type":["string","null"]},"userEmail":{"type":["string","null"]},"userPhone":{"type":["string","null"]}},"required":["integrationName","accountName","accountTaxId","accountEmail","accountAddress","customerTradeName","customerLegalName","customerTaxId","customerEmail","customerPhone","customerTaxRegime","relationshipType","customerStatus","userName","userEmail","userPhone"]}`)
	encoded, _ := json.Marshal(previous)
	systemsJSON, _ := json.Marshal(systems)
	input := fmt.Sprintf("Sistemas conectados: %s\nRascunho atual (preserve dados que não foram corrigidos): %s\nNova mensagem do administrador: %s", systemsJSON, encoded, message)
	body := map[string]any{"model": e.model, "store": false, "reasoning": map[string]string{"effort": "low"}, "instructions": "Extraia somente informações explicitamente fornecidas. Não invente valores. Preserve dados anteriores; retorne null para campos que não podem ser determinados pela nova mensagem ou pelo rascunho. Associe a empresa/conta, cliente e usuário nos campos corretos. integrationName deve corresponder a um dos sistemas conectados.", "input": input, "text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "lynx_provisioning_draft", "strict": true, "schema": schema}}}
	data, err := json.Marshal(body)
	if err != nil {
		return domain.Draft{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(data))
	if err != nil {
		return domain.Draft{}, err
	}
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return domain.Draft{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return domain.Draft{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.Draft{}, fmt.Errorf("OpenAI Responses API returned status %d", resp.StatusCode)
	}
	var result struct {
		Status string `json:"status"`
		Error  any    `json:"error"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			} `json:"content"`
		} `json:"output"`
		OutputText string `json:"output_text"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return domain.Draft{}, err
	}
	if result.Error != nil || result.Status == "incomplete" {
		return domain.Draft{}, errors.New("OpenAI did not return a complete draft")
	}
	text := result.OutputText
	if text == "" {
		for _, item := range result.Output {
			for _, part := range item.Content {
				if part.Refusal != "" {
					return domain.Draft{}, errors.New("OpenAI refused draft extraction")
				}
				if part.Type == "output_text" {
					text = part.Text
				}
			}
		}
	}
	if text == "" {
		return domain.Draft{}, errors.New("OpenAI response has no structured output")
	}
	var draft domain.Draft
	if err := json.Unmarshal([]byte(text), &draft); err != nil {
		return domain.Draft{}, err
	}
	return draft, nil
}

var _ domain.DraftExtractor = (*DraftExtractor)(nil)
