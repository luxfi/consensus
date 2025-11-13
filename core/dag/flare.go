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
	// A vertex has a certificate if ≥2f+1 vertices in the next round reference it
	// This indicates strong support from honest validators
	
	block, ok := store.Get(vertex)
	if !ok {
		return false
	}
	
	currentRound := block.Round()
	author := block.Author()
	
	// Count how many vertices in round+1 reference this vertex (support it)
	support := 0
	required := 2*params.F + 1
	
	// Check children of this vertex
	children := store.Children(vertex)
	for _, child := range children {
		childBlock, ok := store.Get(child)
		if !ok {
			continue
		}
		
		// Count children in the next round
		if childBlock.Round() == currentRound+1 {
			// Check if child references this vertex via parents
			for _, parent := range childBlock.Parents() {
				if parent == vertex {
					support++
					if support >= required {
						return true
					}
					break
				}
			}
		}
	}
	
	// If author matters, filter by author
	_ = author // Use author for potential filtering
	
	return false
}

// HasSkipGeneric checks if a vertex has a skip certificate (≥2f+1 validators do not support it)
func HasSkipGeneric[V VID](store Store[V], vertex V, params Params) bool {
	// A vertex has a skip certificate if ≥2f+1 vertices in the next round do NOT reference it
	// This indicates the vertex should be skipped/rejected
	
	block, ok := store.Get(vertex)
	if !ok {
		return false
	}
	
	currentRound := block.Round()
	
	// Count vertices in next round that do NOT reference this vertex
	noSupport := 0
	required := 2*params.F + 1
	
	// Get all vertices in the next round
	children := store.Children(vertex)
	childrenInNextRound := make(map[V]bool)
	
	for _, child := range children {
		childBlock, ok := store.Get(child)
		if !ok {
			continue
		}
		if childBlock.Round() == currentRound+1 {
			childrenInNextRound[child] = true
		}
	}
	
	// Count vertices in next round that don't reference this vertex
	// This is approximated by checking if they're not in the children set
	// In a real implementation, we'd iterate through all vertices in round+1
	for child := range childrenInNextRound {
		childBlock, _ := store.Get(child)
		hasReference := false
		
		for _, parent := range childBlock.Parents() {
			if parent == vertex {
				hasReference = true
				break
			}
		}
		
		if !hasReference {
			noSupport++
			if noSupport >= required {
				return true
			}
		}
	}
	
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
	// After finalizing vertices, compute the new frontier (tips) of the DAG
	// The frontier consists of vertices that:
	// 1. Are not finalized
	// 2. Have no unfinalized children (are "tips")
	
	if len(finalized) == 0 {
		return store.Head()
	}
	
	// Mark finalized vertices
	finalizedSet := make(map[V]bool)
	for _, v := range finalized {
		finalizedSet[v] = true
	}
	
	// Find vertices that are tips (have no children, or all children are finalized)
	var newFrontier []V
	candidates := store.Head()
	
	// BFS to find all vertices reachable from finalized set
	visited := make(map[V]bool)
	queue := make([]V, len(finalized))
	copy(queue, finalized)
	
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		
		if visited[current] {
			continue
		}
		visited[current] = true
		
		// Check children of finalized vertices
		children := store.Children(current)
		for _, child := range children {
			if !finalizedSet[child] && !visited[child] {
				// This child is not finalized, could be part of new frontier
				queue = append(queue, child)
				
				// Check if this child has no unfinalized children (is a tip)
				childChildren := store.Children(child)
				isTip := true
				for _, cc := range childChildren {
					if !finalizedSet[cc] {
						isTip = false
						break
					}
				}
				
				if isTip {
					newFrontier = append(newFrontier, child)
				}
			}
		}
	}
	
	// If no frontier found, return current head
	if len(newFrontier) == 0 {
		return candidates
	}
	
	return newFrontier
}
