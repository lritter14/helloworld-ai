package service

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound is returned when a requested resource is not found.
	ErrNotFound = errors.New("not found")
	// ErrExternalService is returned when an external service call fails.
	ErrExternalService = errors.New("external service error")
)

// ValidationError represents a validation error with a field name.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %s: %s", e.Field, e.Message)
}

// WrapError wraps an error with additional context.
func WrapError(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

