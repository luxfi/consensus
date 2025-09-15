package dag

// Decision represents the outcome of cert/skip analysis
type Decision int

const (
	DecideUndecided Decision = iota
	DecideCommit             // Certificate found - vertex should be committed
	DecideSkip               // Skip certificate found - vertex should be skipped
)

// Cert: >=2f+1 in r+1 support proposer(author,round). Skip: >=2f+1 in r+1 not supporting.
func HasCertificate(v View, proposer Meta, p Params) bool {
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

func HasSkip(v View, proposer Meta, p Params) bool {
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

type Flare struct{ p Params }

func NewFlare(p Params) *Flare { return &Flare{p: p} }

func (f *Flare) Classify(v View, proposer Meta) Decision {
	switch {
	case HasCertificate(v, proposer, f.p):
		return DecideCommit
	case HasSkip(v, proposer, f.p):
		return DecideSkip
	default:
		return DecideUndecided
	}
}

// Generic versions for new protocol interfaces

// HasCertificateGeneric checks if a vertex has a certificate (≥2f+1 validators support it)
func HasCertificateGeneric[V VID](store Store[V], vertex V, params Params) bool {
	// TODO: Implement certificate detection
	// A vertex has a certificate if ≥2f+1 vertices in the next round reference it
	// This indicates strong support from honest validators
	return false
}

// HasSkipGeneric checks if a vertex has a skip certificate (≥2f+1 validators do not support it)
func HasSkipGeneric[V VID](store Store[V], vertex V, params Params) bool {
	// TODO: Implement skip detection
	// A vertex has a skip certificate if ≥2f+1 vertices in the next round do NOT reference it
	// This indicates the vertex should be skipped/rejected
	return false
}

// ClassifyGeneric determines the status of a vertex based on cert/skip analysis
func ClassifyGeneric[V VID](store Store[V], vertex V, params Params) Decision {
	switch {
	case HasCertificateGeneric(store, vertex, params):
		return DecideCommit
	case HasSkipGeneric(store, vertex, params):
		return DecideSkip
	default:
		return DecideUndecided
	}
}

// ComputeFinalizableSet returns vertices that can be finalized based on cert/skip analysis
func ComputeFinalizableSet[V VID](store Store[V], candidates []V, params Params) []V {
	var finalizable []V
	for _, v := range candidates {
		if ClassifyGeneric(store, v, params) == DecideCommit {
			finalizable = append(finalizable, v)
		}
	}
	return finalizable
}

// UpdateDAGFrontier computes the new frontier after finalizing a set of vertices
func UpdateDAGFrontier[V VID](store Store[V], finalized []V) []V {
	// TODO: Implement frontier update logic
	// After finalizing vertices, compute the new frontier (tips) of the DAG
	return []V{}
}
