package health

import (
	"context"

	domain "github.com/lynx/lynx-os/backend/internal/domain/health"
)

type DependencyStatus struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

type Result struct {
	Ready        bool               `json:"ready"`
	Dependencies []DependencyStatus `json:"dependencies"`
}

type Service struct {
	dependencies []domain.Dependency
}

func NewService(dependencies ...domain.Dependency) *Service {
	return &Service{dependencies: dependencies}
}

func (s *Service) Check(ctx context.Context) Result {
	result := Result{Ready: true, Dependencies: make([]DependencyStatus, 0, len(s.dependencies))}
	for _, dependency := range s.dependencies {
		status := DependencyStatus{Name: dependency.Name}
		status.Ready = dependency.Checker.Check(ctx) == nil
		result.Ready = result.Ready && status.Ready
		result.Dependencies = append(result.Dependencies, status)
	}
	return result
}
