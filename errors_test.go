package consensus

import (
	"errors"
	"testing"
)

// Common consensus errors
var (
	ErrTimeout          = errors.New("consensus timeout")
	ErrInvalidVote      = errors.New("invalid vote")
	ErrNoQuorum         = errors.New("no quorum")
	ErrConflict         = errors.New("conflicting decisions")
	ErrNotFinalized     = errors.New("block not finalized")
	ErrAlreadyDecided   = errors.New("already decided")
	ErrInvalidState     = errors.New("invalid state transition")
	ErrNetworkPartition = errors.New("network partition detected")
)

func TestErrorTypes(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{ErrTimeout, "consensus timeout"},
		{ErrInvalidVote, "invalid vote"},
		{ErrNoQuorum, "no quorum"},
		{ErrConflict, "conflicting decisions"},
		{ErrNotFinalized, "block not finalized"},
		{ErrAlreadyDecided, "already decided"},
		{ErrInvalidState, "invalid state transition"},
		{ErrNetworkPartition, "network partition detected"},
	}

	for _, test := range tests {
		if test.err.Error() != test.expected {
			t.Errorf("expected error %q, got %q", test.expected, test.err.Error())
		}
	}
}

func TestErrorWrapping(t *testing.T) {
	base := ErrTimeout
	wrapped := WrapError(base, "block 123")
	
	if !errors.Is(wrapped, base) {
		t.Error("wrapped error should match base error")
	}
	
	expected := "block 123: consensus timeout"
	if wrapped.Error() != expected {
		t.Errorf("expected %q, got %q", expected, wrapped.Error())
	}
}

func TestErrorChaining(t *testing.T) {
	err1 := ErrNoQuorum
	err2 := WrapError(err1, "round 1")
	err3 := WrapError(err2, "validator set A")
	
	if !errors.Is(err3, ErrNoQuorum) {
		t.Error("chained error should match original")
	}
	
	expected := "validator set A: round 1: no quorum"
	if err3.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err3.Error())
	}
}

func TestIsRetryable(t *testing.T) {
	retryable := []error{
		ErrTimeout,
		ErrNoQuorum,
		ErrNetworkPartition,
	}
	
	notRetryable := []error{
		ErrInvalidVote,
		ErrConflict,
		ErrAlreadyDecided,
		ErrInvalidState,
	}
	
	for _, err := range retryable {
		if !IsRetryable(err) {
			t.Errorf("%v should be retryable", err)
		}
	}
	
	for _, err := range notRetryable {
		if IsRetryable(err) {
			t.Errorf("%v should not be retryable", err)
		}
	}
}

// Helper functions
func WrapError(err error, context string) error {
	return &WrappedError{
		context: context,
		err:     err,
	}
}

type WrappedError struct {
	context string
	err     error
}

func (w *WrappedError) Error() string {
	return w.context + ": " + w.err.Error()
}

func (w *WrappedError) Unwrap() error {
	return w.err
}

func IsRetryable(err error) bool {
	return errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrNoQuorum) ||
		errors.Is(err, ErrNetworkPartition)
}