package core

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
)

// ErrNotFound is returned by engines when an item no longer exists.
var ErrNotFound = errors.New("not found")

// ErrUnsupported is returned by adapters for operations outside their Caps.
var ErrUnsupported = errors.New("unsupported by this adapter")

type retryableError struct{ err error }

func (e retryableError) Error() string { return e.err.Error() }
func (e retryableError) Unwrap() error { return e.err }

// Retryable marks an error as transient (network, 5xx, expired link) so a job
// failing with it can be retried.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return retryableError{err}
}

// IsRetryable reports whether a job failure may be retried. Explicitly marked
// errors, network errors and timeouts qualify.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var r retryableError
	if errors.As(err, &r) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF)
}
