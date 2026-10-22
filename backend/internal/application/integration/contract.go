package integration

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

type ContractService struct {
	client domain.SaaSContract
}

func NewContractService(client domain.SaaSContract) *ContractService {
	return &ContractService{client: client}
}

func (s *ContractService) Discover(ctx context.Context, baseURL, apiKey string) (domain.Capabilities, error) {
	capabilities, err := s.client.Discover(ctx, baseURL, apiKey)
	if err != nil {
		return domain.Capabilities{}, err
	}
	if capabilities.ContractVersion != "1" {
		return domain.Capabilities{}, domain.ErrUnsupportedContractVersion
	}
	return capabilities, nil
}

func (s *ContractService) Execute(ctx context.Context, baseURL, apiKey string, command domain.Command, idempotencyKey string) (domain.Response, error) {
	if strings.TrimSpace(command.Operation) == "" || command.Input == nil {
		return domain.Response{}, domain.ErrInvalidCommand
	}
	capabilities, err := s.Discover(ctx, baseURL, apiKey)
	if err != nil {
		return domain.Response{}, err
	}
	readOnly, err := supports(capabilities, command)
	if err != nil {
		return domain.Response{}, err
	}
	if !readOnly && !validIdempotencyKey(idempotencyKey) {
		return domain.Response{}, domain.ErrIdempotencyKeyRequired
	}

	command.ContractVersion = "1"
	response, err := s.client.Execute(ctx, baseURL, apiKey, command, idempotencyKey)
	if err != nil {
		return domain.Response{}, err
	}
	if response.ContractVersion != "1" || response.Operation != command.Operation {
		return domain.Response{}, domain.ErrUnsupportedContractVersion
	}
	return response, nil
}

func supports(capabilities domain.Capabilities, command domain.Command) (bool, error) {
	if command.Operation == "bootstrap" {
		if _, ok := capabilities.Operations[command.Operation]; !ok {
			return false, domain.ErrOperationUnsupported
		}
		return false, nil
	}
	if !strings.HasPrefix(command.Operation, "resource.") {
		return false, domain.ErrOperationUnsupported
	}
	resourceType, ok := command.Input["resourceType"].(string)
	if !ok || resourceType == "" {
		return false, domain.ErrInvalidCommand
	}
	resource, ok := capabilities.Resources[resourceType]
	if !ok {
		return false, domain.ErrOperationUnsupported
	}
	name := strings.TrimPrefix(command.Operation, "resource.")
	for _, operation := range resource.Operations {
		if operation == name {
			return name == "list" || name == "get", nil
		}
	}
	return false, fmt.Errorf("%w: %s", domain.ErrOperationUnsupported, command.Operation)
}

func validIdempotencyKey(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
}
