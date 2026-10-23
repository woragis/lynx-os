package telegram

import (
	"context"
	"errors"
	"testing"

	domain "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

func TestMergeDraftKeepsPriorValuesAndUsesCorrections(t *testing.T) {
	oldName, oldEmail, newName := "Conta antiga", "old@lynx.test", "Conta nova"
	got := mergeDraft(domain.Draft{AccountName: &newName}, domain.Draft{AccountName: &oldName, UserEmail: &oldEmail})
	if value(got.AccountName) != "Conta nova" || value(got.UserEmail) != "old@lynx.test" {
		t.Fatalf("unexpected merged draft %#v", got)
	}
}

func TestUnavailableExtractorKeepsAIRequirementExplicit(t *testing.T) {
	_, err := (UnavailableDraftExtractor{}).Extract(context.Background(), "empresa exemplo", domain.Draft{}, nil)
	if !errors.Is(err, ErrAIUnavailable) {
		t.Fatalf("expected AI unavailable error, got %v", err)
	}
}

func TestNormalizeDraftRestrictsEnumeratedFields(t *testing.T) {
	valid, invalid := "recorrente", "qualquer coisa"
	draft := domain.Draft{RelationshipType: &valid, CustomerStatus: &invalid}
	normalizeDraft(&draft)
	if value(draft.RelationshipType) != "Recorrente" || draft.CustomerStatus != nil {
		t.Fatalf("unexpected normalized values: %#v", draft)
	}
}

func TestMissingFieldsIdentifiesRequiredCompanyClientAndUserData(t *testing.T) {
	fields := missingFields(domain.Draft{})
	if len(fields) != 11 {
		t.Fatalf("expected 11 missing fields, got %d: %v", len(fields), fields)
	}
}
