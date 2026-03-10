package oss

import (
	"errors"
	"fmt"
)

var (
	ErrConfig         = errors.New("oss config error")
	ErrAuthentication = errors.New("oss authentication error")
	ErrNotFound       = errors.New("oss object not found")
	ErrTransport      = errors.New("oss transport error")
	ErrInternal       = errors.New("oss internal error")
)

// OperationError wraps lower-level errors with a stable classification.
type OperationError struct {
	Op   string
	Key  string
	Kind error
	Err  error
}

func (e *OperationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	switch {
	case e.Key != "" && e.Op != "":
		return fmt.Sprintf("%s %q: %v", e.Op, e.Key, e.Err)
	case e.Op != "":
		return fmt.Sprintf("%s: %v", e.Op, e.Err)
	default:
		return e.Err.Error()
	}
}

func (e *OperationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *OperationError) Is(target error) bool {
	if e == nil {
		return false
	}
	return target == e.Kind || errors.Is(e.Err, target)
}

func wrapOperationError(op, key string, kind error, err error) error {
	if err == nil {
		return nil
	}
	return &OperationError{
		Op:   op,
		Key:  key,
		Kind: kind,
		Err:  err,
	}
}
