package service

import (
	"errors"
	"testing"
)

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *ValidationError
		want    string
	}{
		{
			name: "field and message",
			err: &ValidationError{
				Field:   "message",
				Message: "cannot be empty",
			},
			want: "validation error on field message: cannot be empty",
		},
		{
			name: "empty field",
			err: &ValidationError{
				Field:   "",
				Message: "invalid",
			},
			want: "validation error on field : invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("ValidationError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		msg     string
		wantNil bool
		wantMsg string
	}{
		{
			name:    "nil error",
			err:     nil,
			msg:     "context",
			wantNil: true,
		},
		{
			name:    "wrapped error",
			err:     errors.New("original error"),
			msg:     "context",
			wantNil: false,
			wantMsg: "context: original error",
		},
		{
			name:    "empty message",
			err:     errors.New("original error"),
			msg:     "",
			wantNil: false,
			wantMsg: ": original error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapError(tt.err, tt.msg)
			if tt.wantNil {
				if got != nil {
					t.Errorf("WrapError() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("WrapError() = nil, want error")
				return
			}
			if got.Error() != tt.wantMsg {
				t.Errorf("WrapError() = %v, want %v", got.Error(), tt.wantMsg)
			}
			// Verify error wrapping
			if !errors.Is(got, tt.err) {
				t.Errorf("WrapError() should wrap original error")
			}
		})
	}
}

func TestErrorConstants(t *testing.T) {
	if ErrInvalidInput == nil {
		t.Error("ErrInvalidInput should not be nil")
	}
	if ErrNotFound == nil {
		t.Error("ErrNotFound should not be nil")
	}
	if ErrExternalService == nil {
		t.Error("ErrExternalService should not be nil")
	}

	// Test error matching
	if !errors.Is(ErrInvalidInput, ErrInvalidInput) {
		t.Error("ErrInvalidInput should match itself")
	}
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Error("ErrNotFound should match itself")
	}
	if !errors.Is(ErrExternalService, ErrExternalService) {
		t.Error("ErrExternalService should match itself")
	}
}

