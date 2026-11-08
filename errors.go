package config

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound marks an optional source that is not present.
var ErrNotFound = errors.New("config source not found")

// ErrInvalid marks malformed configuration data.
var ErrInvalid = errors.New("invalid configuration")

// FieldError identifies a configuration failure without including its value.
type FieldError struct {
	Source string
	Path   string
	Op     string
	Err    error
}

func (e *FieldError) Error() string {
	parts := []string{"config"}
	if e.Op != "" {
		parts = append(parts, e.Op)
	}
	if e.Source != "" {
		parts = append(parts, "source="+e.Source)
	}
	if e.Path != "" {
		parts = append(parts, "path="+e.Path)
	}
	if e.Err != nil {
		return strings.Join(parts, " ") + ": " + e.Err.Error()
	}
	return strings.Join(parts, " ")
}

func (e *FieldError) Unwrap() error { return e.Err }

// Errors contains independent configuration failures.
//
//nolint:errname // Errors is the established public aggregate type.
type Errors struct{ list []error }

func (e *Errors) Error() string {
	if len(e.list) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.list))
	for _, err := range e.list {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}

func (e *Errors) Unwrap() []error { return e.list }

func (e *Errors) add(err error) {
	if err == nil {
		return
	}
	nested := &Errors{}
	if errors.As(err, &nested) {
		e.list = append(e.list, nested.list...)
		return
	}
	e.list = append(e.list, err)
}

// Err returns nil when no failures were collected, otherwise it returns e.
func (e *Errors) Err() error {
	if len(e.list) == 0 {
		return nil
	}
	return e
}

func invalid(source, path, op string, err error) error {
	if err == nil {
		err = ErrInvalid
	}
	return &FieldError{Source: source, Path: path, Op: op, Err: fmt.Errorf("%w: %w", ErrInvalid, err)}
}
