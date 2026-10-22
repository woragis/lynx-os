package health

import "context"

type Checker interface {
	Check(context.Context) error
}

type Dependency struct {
	Name    string
	Checker Checker
}
