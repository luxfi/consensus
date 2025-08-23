package flare

import "github.com/luxfi/consensus/core/dag"

type Decision int

const (
	DecideUndecided Decision = iota
	DecideCommit
	DecideSkip
)

// Cert: >=2f+1 in r+1 support proposer(author,round). Skip: >=2f+1 in r+1 not supporting.
func HasCertificate(v dag.View, proposer dag.Meta, p dag.Params) bool {
	r1 := proposer.Round() + 1
	next := v.ByRound(r1)
	support := 0
	for _, m := range next {
		if v.Supports(m.ID(), proposer.Author(), proposer.Round()) {
			support++
			if support >= 2*p.F+1 {
				return true
			}
		}
	}
	return false
}

func HasSkip(v dag.View, proposer dag.Meta, p dag.Params) bool {
	r1 := proposer.Round() + 1
	next := v.ByRound(r1)
	nos := 0
	for _, m := range next {
		if !v.Supports(m.ID(), proposer.Author(), proposer.Round()) {
			nos++
			if nos >= 2*p.F+1 {
				return true
			}
		}
	}
	return false
}

type Flare struct{ p dag.Params }

func NewFlare(p dag.Params) *Flare { return &Flare{p: p} }

func (f *Flare) Classify(v dag.View, proposer dag.Meta) Decision {
	switch {
	case HasCertificate(v, proposer, f.p):
		return DecideCommit
	case HasSkip(v, proposer, f.p):
		return DecideSkip
	default:
		return DecideUndecided
	}
}
