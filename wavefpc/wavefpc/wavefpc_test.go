package wavefpc

import (
	"bytes"
	"testing"
)

type mockCommittee struct {
	n      int
	me     ValidatorIndex
	id2idx map[string]ValidatorIndex
}

func (m mockCommittee) Size() int { return m.n }
func (m mockCommittee) IndexOf(a []byte) (ValidatorIndex, bool) {
	i, ok := m.id2idx[string(a)]
	return i, ok
}

type mockClassifier struct{ owned map[[32]byte][]ObjectID }

func (m mockClassifier) OwnedInputs(tx TxRef) []ObjectID {
	return m.owned[tx]
}
func (m mockClassifier) Conflicts(a, b TxRef) bool { return false }

type mockDAG struct{}

func (mockDAG) InAncestry(_ []byte, _ TxRef) bool { return true }

func TestExecutableAt2fPlus1(t *testing.T) {
	var tx TxRef
	copy(tx[:], bytes.Repeat([]byte{1}, 32))
	o := ObjectID{3: 7}

	cls := mockClassifier{owned: map[[32]byte][]ObjectID{tx: oSlice(o)}}
	comm := mockCommittee{
		n: 4,
		id2idx: map[string]ValidatorIndex{
			"A": 0, "B": 1, "C": 2, "D": 3,
		},
	}
	cfg := Config{Quorum: Quorum{N: 4, F: 1}, VoteLimitPerBlock: 10}

	w := New(cfg, comm, 0, cls, mockDAG{}, nil, nil)

	// 2f+1 = 3 votes
	w.OnBlockObserved(&ObservedBlock{Author: []byte("A"), FPCVotes: []TxRef{tx}})
	w.OnBlockObserved(&ObservedBlock{Author: []byte("B"), FPCVotes: []TxRef{tx}})

	if st, _ := w.Status(tx); st != Pending {
		t.Fatalf("expected Pending after 2 votes")
	}
	w.OnBlockObserved(&ObservedBlock{Author: []byte("C"), FPCVotes: []TxRef{tx}})
	st, _ := w.Status(tx)
	if st != Executable {
		t.Fatalf("expected Executable at 3 votes; got %v", st)
	}
	// anchor -> Final
	w.OnBlockAccepted(&ObservedBlock{ID: []byte("blk"), Author: []byte("C"), FPCVotes: []TxRef{tx}})
	st, _ = w.Status(tx)
	if st != Final {
		t.Fatalf("expected Final after anchor")
	}
}

func oSlice(o ObjectID) []ObjectID { return []ObjectID{o} }
