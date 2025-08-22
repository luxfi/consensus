package dag

import (
	"testing"
)

type testVertex struct {
	id      VertexID
	author  string
	round   uint64
	parents []VertexID
}

func (v *testVertex) ID() VertexID         { return v.id }
func (v *testVertex) Author() string       { return v.author }
func (v *testVertex) Round() uint64        { return v.round }
func (v *testVertex) Parents() []VertexID { return v.parents }

type testView struct {
	vertices map[VertexID]Meta
	byRound  map[uint64][]Meta
}

func newTestView() *testView {
	return &testView{
		vertices: make(map[VertexID]Meta),
		byRound:  make(map[uint64][]Meta),
	}
}

func (v *testView) add(m Meta) {
	v.vertices[m.ID()] = m
	v.byRound[m.Round()] = append(v.byRound[m.Round()], m)
}

func (v *testView) Get(id VertexID) (Meta, bool) {
	m, ok := v.vertices[id]
	return m, ok
}

func (v *testView) ByRound(round uint64) []Meta {
	return v.byRound[round]
}

func (v *testView) Supports(from VertexID, author string, round uint64) bool {
	fromV, ok := v.Get(from)
	if !ok {
		return false
	}
	
	// Check if from vertex has parent from author at round
	for _, parentID := range fromV.Parents() {
		if parent, ok := v.Get(parentID); ok {
			if parent.Author() == author && parent.Round() == round {
				return true
			}
		}
	}
	return false
}

func TestHasCertificate(t *testing.T) {
	v := newTestView()
	p := Params{N: 4, F: 1} // Need 2f+1 = 3 for certificate

	// Create proposer at round 0
	proposer := &testVertex{
		id:     VertexID{1},
		author: "A",
		round:  0,
	}
	v.add(proposer)

	// Create 3 vertices at round 1 supporting proposer
	for i := 0; i < 3; i++ {
		v.add(&testVertex{
			id:      VertexID{byte(i + 2)},
			author:  string(rune('B' + i)),
			round:   1,
			parents: []VertexID{proposer.ID()},
		})
	}

	// Should have certificate
	if !HasCertificate(v, proposer, p) {
		t.Error("expected certificate with 3 supporters")
	}

	// Test with only 2 supporters (not enough)
	v2 := newTestView()
	v2.add(proposer)
	for i := 0; i < 2; i++ {
		v2.add(&testVertex{
			id:      VertexID{byte(i + 2)},
			author:  string(rune('B' + i)),
			round:   1,
			parents: []VertexID{proposer.ID()},
		})
	}

	if HasCertificate(v2, proposer, p) {
		t.Error("should not have certificate with only 2 supporters")
	}
}

func TestHasSkip(t *testing.T) {
	v := newTestView()
	p := Params{N: 4, F: 1} // Need 2f+1 = 3 for skip

	proposer := &testVertex{
		id:     VertexID{1},
		author: "A",
		round:  0,
	}
	v.add(proposer)

	// Create 3 vertices at round 1 NOT supporting proposer
	for i := 0; i < 3; i++ {
		v.add(&testVertex{
			id:      VertexID{byte(i + 2)},
			author:  string(rune('B' + i)),
			round:   1,
			parents: []VertexID{}, // No parents = not supporting
		})
	}

	// Should have skip certificate
	if !HasSkip(v, proposer, p) {
		t.Error("expected skip with 3 non-supporters")
	}
}

func TestFlareClassify(t *testing.T) {
	f := NewFlare(Params{N: 4, F: 1})
	v := newTestView()

	proposer := &testVertex{
		id:     VertexID{1},
		author: "A",
		round:  0,
	}
	v.add(proposer)

	// Initially undecided
	if f.Classify(v, proposer) != DecideUndecided {
		t.Error("expected undecided with no votes")
	}

	// Add supporters for certificate
	for i := 0; i < 3; i++ {
		v.add(&testVertex{
			id:      VertexID{byte(i + 2)},
			author:  string(rune('B' + i)),
			round:   1,
			parents: []VertexID{proposer.ID()},
		})
	}

	if f.Classify(v, proposer) != DecideCommit {
		t.Error("expected commit with certificate")
	}
}