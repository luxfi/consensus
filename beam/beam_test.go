// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultBuilder(t *testing.T) {
	builder := &DefaultBuilder[string]{}

	parents := [][]byte{{1, 2, 3}, {4, 5, 6}}
	decided := [][]byte{{7, 8, 9}}
	execOwned := []string{"tx1", "tx2", "tx3"}

	block, err := builder.Propose(parents, decided, execOwned)
	require.NoError(t, err)
	require.NotNil(t, block)

	// Check header
	require.Equal(t, parents, block.Header.Parents)
	require.WithinDuration(t, time.Now(), block.Header.Ts, time.Second)
	require.Equal(t, uint64(0), block.Header.Round) // Default is 0
}

func TestProposedBlockStructure(t *testing.T) {
	// Test block structure
	block := &ProposedBlock{
		Header: Header{
			Parents: [][]byte{{1}, {2}},
			Round:   100,
			Ts:      time.Now(),
		},
		Entries: []Entry{
			{Payload: []byte("entry1")},
			{Payload: []byte("entry2")},
		},
		Votes:   nil,
		BLSSig:  []byte("bls-signature"),
		PQSig:   []byte("pq-signature"),
		Binding: []byte("binding-data"),
	}

	require.Len(t, block.Header.Parents, 2)
	require.Equal(t, uint64(100), block.Header.Round)
	require.Len(t, block.Entries, 2)
	require.NotNil(t, block.BLSSig)
	require.NotNil(t, block.PQSig)
	require.NotNil(t, block.Binding)
}

func TestBuilderWithDifferentTypes(t *testing.T) {
	// Test with int type
	intBuilder := &DefaultBuilder[int]{}
	block, err := intBuilder.Propose(nil, nil, []int{1, 2, 3})
	require.NoError(t, err)
	require.NotNil(t, block)

	// Test with struct type
	type TxID struct {
		Hash [32]byte
	}
	structBuilder := &DefaultBuilder[TxID]{}
	block2, err := structBuilder.Propose(nil, nil, []TxID{{Hash: [32]byte{1}}})
	require.NoError(t, err)
	require.NotNil(t, block2)
}