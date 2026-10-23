package sqliteimport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	integration "github.com/lynx/lynx-os/backend/internal/domain/integration"
	"github.com/lynx/lynx-os/backend/internal/infrastructure/cryptox"
)

func TestPreviewNormalizesLegacySecretsAndArchivesTelegramDraftEncrypted(t *testing.T) {
	cipher := testCipher(t)
	key, err := cipher.Encrypt([]byte("legacy-api-key"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := padLegacy(key)
	dataset := Dataset{Integrations: []LegacyIntegration{{ID: "i-1", Name: "EasyBill", Slug: "easybill", BaseURL: "https://easybill.test", Ciphertext: legacy.Ciphertext, IV: legacy.IV, AuthTag: legacy.AuthTag, Active: true}}, TelegramConversations: []LegacyTelegram{{ChatID: "876916572", State: "COLLECTING", DraftJSON: `{"customerName":"Private Name"}`, UpdatedAt: time.Now().UTC()}}}
	source := &fakeSource{data: dataset}
	target := &fakeTarget{}
	importer := New(source, target, cipher)
	fingerprint, counts, err := importer.Preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != "source-fingerprint" || counts != (Counts{Integrations: 1, TelegramConversations: 1}) {
		t.Fatalf("unexpected preview: %s %#v", fingerprint, counts)
	}
	if target.imported {
		t.Fatal("dry-run wrote data")
	}
	if source.data.Integrations[0].Ciphertext != legacy.Ciphertext {
		t.Fatal("source dataset was mutated")
	}
	prepared, err := source.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := importer.normalize(&prepared); err != nil {
		t.Fatal(err)
	}
	plain, err := cipher.Decrypt(prepared.Integrations[0].secret())
	if err != nil || string(plain) != "legacy-api-key" {
		t.Fatal("legacy encrypted key was not normalized")
	}
	telegramPlain, err := cipher.Decrypt(prepared.TelegramConversations[0].EncryptedDraft)
	if err != nil {
		t.Fatal(err)
	}
	if string(telegramPlain) == "" || contains(string(telegramPlain), "876916572") == false {
		t.Fatal("legacy Telegram record is not preserved inside encrypted archive payload")
	}
}

func TestNormalizeOperationConvertsPayloadAndRetryStatus(t *testing.T) {
	cipher := testCipher(t)
	oldPayload := integration.Command{Operation: "resource.update", Input: map[string]any{"resourceType": "customer", "resourceId": "c-1", "accountId": "a-1", "changes": map[string]any{"tradeName": "Novo"}}}
	raw, _ := json.Marshal(oldPayload)
	sealed, err := cipher.Encrypt(raw)
	if err != nil {
		t.Fatal(err)
	}
	rawResponse, _ := json.Marshal(integration.Response{ContractVersion: "1", RequestID: "remote-1", Operation: oldPayload.Operation, Status: "completed", Data: json.RawMessage(`{"id":"c-1"}`)})
	response, err := cipher.Encrypt(rawResponse)
	if err != nil {
		t.Fatal(err)
	}
	item := LegacyOperation{ID: "op-1", IdempotencyKey: "key-001", IntegrationID: "i-1", PayloadCiphertext: base64.StdEncoding.EncodeToString([]byte(decodeRaw(sealed.Ciphertext))), PayloadIV: base64.StdEncoding.EncodeToString([]byte(decodeRaw(sealed.IV))), PayloadAuthTag: base64.StdEncoding.EncodeToString([]byte(decodeRaw(sealed.AuthTag))), ResponseCiphertext: base64.StdEncoding.EncodeToString([]byte(decodeRaw(response.Ciphertext))), ResponseIV: base64.StdEncoding.EncodeToString([]byte(decodeRaw(response.IV))), ResponseAuthTag: base64.StdEncoding.EncodeToString([]byte(decodeRaw(response.AuthTag))), Status: "RETRYABLE"}
	dataset := Dataset{Operations: []LegacyOperation{item}}
	importer := New(&fakeSource{data: dataset}, &fakeTarget{}, cipher)
	if err := importer.normalize(&dataset); err != nil {
		t.Fatal(err)
	}
	if dataset.Operations[0].Status != "failed" || dataset.Operations[0].Operation != "resource.update" || dataset.Operations[0].PayloadHash == "" {
		t.Fatalf("operation metadata was not normalized: %#v", dataset.Operations[0])
	}
	plain, err := cipher.Decrypt(integration.SealedSecret{Ciphertext: dataset.Operations[0].PayloadCiphertext, IV: dataset.Operations[0].PayloadIV, AuthTag: dataset.Operations[0].PayloadAuthTag})
	if err != nil {
		t.Fatal(err)
	}
	var got integration.Command
	if err := json.Unmarshal(plain, &got); err != nil {
		t.Fatal(err)
	}
	if got.ContractVersion != "1" || got.Operation != "resource.update" {
		t.Fatalf("unexpected normalized command %#v", got)
	}
}

func TestPreviewRequiresEmptyTarget(t *testing.T) {
	target := &fakeTarget{counts: Counts{Integrations: 1}}
	importer := New(&fakeSource{}, target, testCipher(t))
	if _, _, err := importer.Preview(context.Background()); !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("expected non-empty target rejection, got %v", err)
	}
}

type fakeSource struct{ data Dataset }

func (s *fakeSource) Integrity(context.Context) error { return nil }
func (s *fakeSource) Fingerprint() (string, error)    { return "source-fingerprint", nil }
func (s *fakeSource) Read(context.Context) (Dataset, error) {
	raw, _ := json.Marshal(s.data)
	var copy Dataset
	_ = json.Unmarshal(raw, &copy)
	return copy, nil
}

type fakeTarget struct {
	counts   Counts
	imported bool
}

func (t *fakeTarget) Counts(context.Context) (Counts, error) { return t.counts, nil }
func (t *fakeTarget) Import(context.Context, Dataset, string, Counts) error {
	t.imported = true
	return nil
}
func testCipher(t *testing.T) *cryptox.AESGCM {
	t.Helper()
	cipher, err := cryptox.NewAESGCM(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}
func padLegacy(value integration.SealedSecret) integration.SealedSecret {
	return integration.SealedSecret{Ciphertext: base64.StdEncoding.EncodeToString([]byte(decodeRaw(value.Ciphertext))), IV: base64.StdEncoding.EncodeToString([]byte(decodeRaw(value.IV))), AuthTag: base64.StdEncoding.EncodeToString([]byte(decodeRaw(value.AuthTag)))}
}
func decodeRaw(value string) string {
	raw, _ := base64.RawStdEncoding.DecodeString(value)
	return string(raw)
}
func (item LegacyIntegration) secret() integration.SealedSecret {
	return integration.SealedSecret{Ciphertext: item.Ciphertext, IV: item.IV, AuthTag: item.AuthTag}
}
func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
