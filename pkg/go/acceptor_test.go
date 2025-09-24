package consensus

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestBasicAcceptor(t *testing.T) {
	tests := []struct {
		name        string
		containerID func() ids.ID
		container   []byte
		wantErr     bool
	}{
		{
			name:        "accept single container",
			containerID: ids.GenerateTestID,
			container:   []byte("test container data"),
			wantErr:     false,
		},
		{
			name:        "accept empty container",
			containerID: ids.GenerateTestID,
			container:   []byte{},
			wantErr:     false,
		},
		{
			name:        "accept nil container",
			containerID: ids.GenerateTestID,
			container:   nil,
			wantErr:     false,
		},
		{
			name: "accept with empty ID",
			containerID: func() ids.ID {
				return ids.Empty
			},
			container: []byte("data with empty id"),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acceptor := NewBasicAcceptor()
			containerID := tt.containerID()
			ctx := context.Background()

			err := acceptor.Accept(ctx, containerID, tt.container)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.container, acceptor.accepted[containerID])
			}
		})
	}
}

func TestBasicAcceptorMultiple(t *testing.T) {
	acceptor := NewBasicAcceptor()
	ctx := context.Background()

	// Accept first container
	id1 := ids.GenerateTestID()
	data1 := []byte("first")
	err := acceptor.Accept(ctx, id1, data1)
	require.NoError(t, err)
	require.Len(t, acceptor.accepted, 1)

	// Accept second container
	id2 := ids.GenerateTestID()
	data2 := []byte("second")
	err = acceptor.Accept(ctx, id2, data2)
	require.NoError(t, err)
	require.Len(t, acceptor.accepted, 2)

	// Verify both are stored correctly
	require.Equal(t, data1, acceptor.accepted[id1])
	require.Equal(t, data2, acceptor.accepted[id2])
}

func TestBasicAcceptorOverwrite(t *testing.T) {
	acceptor := NewBasicAcceptor()
	ctx := context.Background()

	id := ids.GenerateTestID()

	// Accept original
	original := []byte("original")
	err := acceptor.Accept(ctx, id, original)
	require.NoError(t, err)
	require.Equal(t, original, acceptor.accepted[id])

	// Overwrite with new data
	updated := []byte("updated")
	err = acceptor.Accept(ctx, id, updated)
	require.NoError(t, err)
	require.Equal(t, updated, acceptor.accepted[id])
	require.Len(t, acceptor.accepted, 1)
}

func TestBasicAcceptorWithCancelledContext(t *testing.T) {
	acceptor := NewBasicAcceptor()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	id := ids.GenerateTestID()
	data := []byte("data")

	// Should still accept even with cancelled context
	// since current implementation doesn't check context
	err := acceptor.Accept(ctx, id, data)
	require.NoError(t, err)
	require.Equal(t, data, acceptor.accepted[id])
}

func TestNewBasicAcceptor(t *testing.T) {
	acceptor := NewBasicAcceptor()
	require.NotNil(t, acceptor)
	require.NotNil(t, acceptor.accepted)
	require.Empty(t, acceptor.accepted)
}

func TestAcceptorInterface(t *testing.T) {
	// Verify BasicAcceptor implements Acceptor interface
	var _ Acceptor = (*BasicAcceptor)(nil)

	acceptor := NewBasicAcceptor()
	require.Implements(t, (*Acceptor)(nil), acceptor)
}

func BenchmarkAccept(b *testing.B) {
	acceptor := NewBasicAcceptor()
	ctx := context.Background()

	testIDs := make([]ids.ID, b.N)
	data := make([][]byte, b.N)

	for i := 0; i < b.N; i++ {
		testIDs[i] = ids.GenerateTestID()
		data[i] = []byte("benchmark data")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = acceptor.Accept(ctx, testIDs[i], data[i])
	}
}

func BenchmarkNewBasicAcceptor(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewBasicAcceptor()
	}
}