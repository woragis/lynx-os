package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

func TestExecuteReplaysPersistedResponseWithoutCallingSaaSAgain(t *testing.T) {
	remote := &fakeSaaS{response: domain.Response{ContractVersion: "1", RequestID: "remote-request", Operation: "resource.update", Status: "completed", Data: json.RawMessage(`{"id":"customer-1"}`)}}
	store := &fakeOperationRepository{records: map[string]domain.ProvisioningRecord{}}
	service := NewService(fakeIntegrationRepository{}, store, fakeCipher{}, NewContractService(remote))
	command := domain.Command{Operation: "resource.update", Input: map[string]any{"resourceType": "customer", "accountId": "account-1", "id": "customer-1", "name": "Acme"}}
	first, err := service.Execute(context.Background(), "integration-1", "owner-1", "central-request-1", command, "request-key-001")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Execute(context.Background(), "integration-1", "owner-1", "central-request-2", command, "request-key-001")
	if err != nil {
		t.Fatal(err)
	}
	if remote.executions != 1 {
		t.Fatalf("replayed operation executed remotely %d times", remote.executions)
	}
	if second.RequestID != first.RequestID || !second.Replayed {
		t.Fatalf("expected original replay response, got %#v", second)
	}
	if len(store.audits) != 1 || store.audits[0].ActorUserID != "owner-1" || store.audits[0].RequestID != "central-request-1" || store.audits[0].ResourceType != "customer" || store.audits[0].ResourceID != "customer-1" {
		t.Fatalf("audit was not stored with original actor/request: %#v", store.audits)
	}
}

func TestExecuteRejectsIdempotencyKeyWithDifferentPayload(t *testing.T) {
	remote := &fakeSaaS{response: domain.Response{ContractVersion: "1", Operation: "resource.update", Status: "completed", Data: json.RawMessage(`{}`)}}
	service := NewService(fakeIntegrationRepository{}, &fakeOperationRepository{records: map[string]domain.ProvisioningRecord{}}, fakeCipher{}, NewContractService(remote))
	base := domain.Command{Operation: "resource.update", Input: map[string]any{"resourceType": "customer", "accountId": "account-1", "id": "customer-1", "name": "Acme"}}
	if _, err := service.Execute(context.Background(), "integration-1", "owner-1", "request-1", base, "request-key-001"); err != nil {
		t.Fatal(err)
	}
	base.Input["name"] = "Different name"
	if _, err := service.Execute(context.Background(), "integration-1", "owner-1", "request-2", base, "request-key-001"); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if remote.executions != 1 {
		t.Fatalf("conflicting request reached SaaS %d times", remote.executions)
	}
}

func TestExecuteCanRetryFailedOperationWithSameKey(t *testing.T) {
	remote := &fakeSaaS{response: domain.Response{ContractVersion: "1", Operation: "resource.update", Status: "completed", Data: json.RawMessage(`{"id":"customer-1"}`)}, failures: 1}
	store := &fakeOperationRepository{records: map[string]domain.ProvisioningRecord{}}
	service := NewService(fakeIntegrationRepository{}, store, fakeCipher{}, NewContractService(remote))
	command := domain.Command{Operation: "resource.update", Input: map[string]any{"resourceType": "customer", "accountId": "account-1", "id": "customer-1", "name": "Acme"}}
	if _, err := service.Execute(context.Background(), "integration-1", "owner-1", "request-1", command, "request-key-001"); !errors.Is(err, ErrSaaSExecution) {
		t.Fatalf("expected SaaS failure, got %v", err)
	}
	response, err := service.Execute(context.Background(), "integration-1", "owner-1", "request-2", command, "request-key-001")
	if err != nil || response.Status != "completed" {
		t.Fatalf("retry did not complete: response=%#v err=%v", response, err)
	}
	if remote.executions != 2 || len(store.audits) != 2 || store.audits[0].Result != "failed" || store.audits[1].Result != "completed" {
		t.Fatalf("retry history was not recorded: calls=%d audits=%#v", remote.executions, store.audits)
	}
}

type fakeSaaS struct {
	executions int
	response   domain.Response
	failures   int
}

func (f *fakeSaaS) Discover(context.Context, string, string) (domain.Capabilities, error) {
	return domain.Capabilities{ContractVersion: "1", Resources: map[string]domain.ResourceCapabilities{"customer": {Operations: []string{"update"}}}}, nil
}
func (f *fakeSaaS) Execute(_ context.Context, _, _ string, command domain.Command, _ string) (domain.Response, error) {
	f.executions++
	if f.failures > 0 {
		f.failures--
		return domain.Response{}, errors.New("network timeout")
	}
	response := f.response
	response.Operation = command.Operation
	return response, nil
}

type fakeIntegrationRepository struct{}

func (fakeIntegrationRepository) Create(context.Context, domain.Record, domain.SealedSecret) (domain.Record, error) {
	return domain.Record{}, nil
}
func (fakeIntegrationRepository) List(context.Context) ([]domain.Record, error) { return nil, nil }
func (fakeIntegrationRepository) Get(context.Context, string) (domain.Record, domain.SealedSecret, error) {
	return domain.Record{ID: "integration-1", BaseURL: "https://saas.example", Active: true}, domain.SealedSecret{Ciphertext: "cipher:api-key"}, nil
}
func (fakeIntegrationRepository) ReplaceSecret(context.Context, string, domain.SealedSecret) (domain.Record, error) {
	return domain.Record{}, nil
}
func (fakeIntegrationRepository) Revoke(context.Context, string) (domain.Record, error) {
	return domain.Record{}, nil
}

type fakeCipher struct{}

func (fakeCipher) Encrypt(value []byte) (domain.SealedSecret, error) {
	return domain.SealedSecret{Ciphertext: "cipher:" + string(value), IV: "iv", AuthTag: "tag"}, nil
}
func (fakeCipher) Decrypt(value domain.SealedSecret) ([]byte, error) {
	if len(value.Ciphertext) < 7 || value.Ciphertext[:7] != "cipher:" {
		return nil, fmt.Errorf("invalid ciphertext")
	}
	return []byte(value.Ciphertext[7:]), nil
}

type fakeOperationRepository struct {
	records map[string]domain.ProvisioningRecord
	audits  []domain.AuditEntry
}

func (f *fakeOperationRepository) StartProvisioning(_ context.Context, item domain.ProvisioningRecord) (domain.ProvisioningRecord, bool, error) {
	key := item.IntegrationID + ":" + item.IdempotencyKey
	if existing, ok := f.records[key]; ok {
		if existing.PayloadHash != item.PayloadHash {
			return domain.ProvisioningRecord{}, false, domain.ErrIdempotencyConflict
		}
		if existing.Status == "failed" {
			existing.Status = "pending"
			f.records[key] = existing
			return existing, true, nil
		}
		return existing, false, nil
	}
	f.records[key] = item
	return item, true, nil
}
func (f *fakeOperationRepository) CompleteProvisioning(_ context.Context, id string, response domain.SealedSecret, audit domain.AuditEntry) error {
	for key, item := range f.records {
		if item.ID == id {
			item.Status = "completed"
			item.Response = &response
			f.records[key] = item
			f.audits = append(f.audits, audit)
			return nil
		}
	}
	return domain.ErrIntegrationNotFound
}
func (f *fakeOperationRepository) FailProvisioning(_ context.Context, id string, audit domain.AuditEntry) error {
	for key, item := range f.records {
		if item.ID == id {
			item.Status = "failed"
			f.records[key] = item
			f.audits = append(f.audits, audit)
			return nil
		}
	}
	return domain.ErrIntegrationNotFound
}
func (f *fakeOperationRepository) WriteAudit(_ context.Context, audit domain.AuditEntry) error {
	f.audits = append(f.audits, audit)
	return nil
}
