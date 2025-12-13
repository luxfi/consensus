package core

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestNewTestDecidable(t *testing.T) {
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	require.NotNil(t, td)
	require.Equal(t, id, td.TestID)
	require.Equal(t, StatusPending, td.TestStatus)
	require.NotNil(t, td.AcceptFunc)
	require.NotNil(t, td.RejectFunc)
}

func TestTestDecidable_ID(t *testing.T) {
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	require.Equal(t, id, td.ID())
}

func TestTestDecidable_ID_Empty(t *testing.T) {
	td := NewTestDecidable(ids.Empty)

	require.Equal(t, ids.Empty, td.ID())
}

func TestTestDecidable_Accept(t *testing.T) {
	ctx := context.Background()
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	require.Equal(t, StatusPending, td.Status())

	err := td.Accept(ctx)
	require.NoError(t, err)
	require.Equal(t, StatusAccepted, td.Status())
}

func TestTestDecidable_Accept_WithCustomFunc(t *testing.T) {
	ctx := context.Background()
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	acceptCalled := false
	td.AcceptFunc = func(ctx context.Context) error {
		acceptCalled = true
		return nil
	}

	err := td.Accept(ctx)
	require.NoError(t, err)
	require.True(t, acceptCalled)
	require.Equal(t, StatusAccepted, td.Status())
}

func TestTestDecidable_Accept_WithError(t *testing.T) {
	ctx := context.Background()
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	expectedErr := errors.New("accept failed")
	td.AcceptFunc = func(ctx context.Context) error {
		return expectedErr
	}

	err := td.Accept(ctx)
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	// Status should remain pending when accept fails
	require.Equal(t, StatusPending, td.Status())
}

func TestTestDecidable_Accept_NilFunc(t *testing.T) {
	ctx := context.Background()
	td := &TestDecidable{
		TestID:     ids.GenerateTestID(),
		TestStatus: StatusPending,
		AcceptFunc: nil,
		RejectFunc: nil,
	}

	err := td.Accept(ctx)
	require.NoError(t, err)
	require.Equal(t, StatusAccepted, td.Status())
}

func TestTestDecidable_Reject(t *testing.T) {
	ctx := context.Background()
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	require.Equal(t, StatusPending, td.Status())

	err := td.Reject(ctx)
	require.NoError(t, err)
	require.Equal(t, StatusRejected, td.Status())
}

func TestTestDecidable_Reject_WithCustomFunc(t *testing.T) {
	ctx := context.Background()
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	rejectCalled := false
	td.RejectFunc = func(ctx context.Context) error {
		rejectCalled = true
		return nil
	}

	err := td.Reject(ctx)
	require.NoError(t, err)
	require.True(t, rejectCalled)
	require.Equal(t, StatusRejected, td.Status())
}

func TestTestDecidable_Reject_WithError(t *testing.T) {
	ctx := context.Background()
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	expectedErr := errors.New("reject failed")
	td.RejectFunc = func(ctx context.Context) error {
		return expectedErr
	}

	err := td.Reject(ctx)
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	// Status should remain pending when reject fails
	require.Equal(t, StatusPending, td.Status())
}

func TestTestDecidable_Reject_NilFunc(t *testing.T) {
	ctx := context.Background()
	td := &TestDecidable{
		TestID:     ids.GenerateTestID(),
		TestStatus: StatusPending,
		AcceptFunc: nil,
		RejectFunc: nil,
	}

	err := td.Reject(ctx)
	require.NoError(t, err)
	require.Equal(t, StatusRejected, td.Status())
}

func TestTestDecidable_Status(t *testing.T) {
	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	// Initial status
	require.Equal(t, StatusPending, td.Status())

	// Manually set status
	td.TestStatus = StatusProcessing
	require.Equal(t, StatusProcessing, td.Status())

	td.TestStatus = StatusAccepted
	require.Equal(t, StatusAccepted, td.Status())

	td.TestStatus = StatusRejected
	require.Equal(t, StatusRejected, td.Status())
}

func TestTestDecidable_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	id := ids.GenerateTestID()
	td := NewTestDecidable(id)

	// With default funcs, context cancellation is not checked
	err := td.Accept(ctx)
	require.NoError(t, err)
	require.Equal(t, StatusAccepted, td.Status())
}

func TestTestDecidable_ContextAwareAccept(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	id := ids.GenerateTestID()
	td := NewTestDecidable(id)
	td.AcceptFunc = func(ctx context.Context) error {
		return ctx.Err()
	}

	err := td.Accept(ctx)
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
	require.Equal(t, StatusPending, td.Status())
}

func TestTestDecidable_ContextAwareReject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	id := ids.GenerateTestID()
	td := NewTestDecidable(id)
	td.RejectFunc = func(ctx context.Context) error {
		return ctx.Err()
	}

	err := td.Reject(ctx)
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
	require.Equal(t, StatusPending, td.Status())
}

func TestTestDecidable_Multiple(t *testing.T) {
	ctx := context.Background()

	// Create multiple decidables
	decidables := make([]*TestDecidable, 5)
	for i := 0; i < 5; i++ {
		decidables[i] = NewTestDecidable(ids.GenerateTestID())
	}

	// Accept some, reject others
	require.NoError(t, decidables[0].Accept(ctx))
	require.NoError(t, decidables[1].Reject(ctx))
	require.NoError(t, decidables[2].Accept(ctx))
	require.NoError(t, decidables[3].Reject(ctx))
	// Leave decidables[4] pending

	require.Equal(t, StatusAccepted, decidables[0].Status())
	require.Equal(t, StatusRejected, decidables[1].Status())
	require.Equal(t, StatusAccepted, decidables[2].Status())
	require.Equal(t, StatusRejected, decidables[3].Status())
	require.Equal(t, StatusPending, decidables[4].Status())
}

func TestTestDecidable_DecidableInterface(t *testing.T) {
	// Verify TestDecidable can be used as Decidable interface
	td := NewTestDecidable(ids.GenerateTestID())

	// Should be able to assign to Decidable interface
	var d Decidable = td
	require.NotNil(t, d)
	require.Equal(t, td.TestID, d.ID())
	require.Equal(t, StatusPending, d.Status())
}

// Benchmarks
func BenchmarkNewTestDecidable(b *testing.B) {
	id := ids.GenerateTestID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewTestDecidable(id)
	}
}

func BenchmarkTestDecidable_Accept(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		td := NewTestDecidable(ids.GenerateTestID())
		_ = td.Accept(ctx)
	}
}

func BenchmarkTestDecidable_Reject(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		td := NewTestDecidable(ids.GenerateTestID())
		_ = td.Reject(ctx)
	}
}

func BenchmarkTestDecidable_ID(b *testing.B) {
	td := NewTestDecidable(ids.GenerateTestID())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = td.ID()
	}
}

func BenchmarkTestDecidable_Status(b *testing.B) {
	td := NewTestDecidable(ids.GenerateTestID())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = td.Status()
	}
}
