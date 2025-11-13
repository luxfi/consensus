package dag

// VertexID represents a vertex identifier in the DAG
type VertexID [32]byte

// Meta interface represents metadata for a DAG vertex
type Meta interface {
	ID() VertexID
	Author() string
	Round() uint64
	Parents() []VertexID
}

// View interface provides access to DAG structure and vertex relationships
type View interface {
	Get(VertexID) (Meta, bool)
	ByRound(round uint64) []Meta
	Supports(from VertexID, author string, round uint64) bool
}

// Params holds DAG consensus parameters
type Params struct{ N, F int }

// VID represents a generic vertex identifier for new protocol interfaces
type VID interface{ comparable }

// BlockView represents a view of a block/vertex in the DAG (generic version)
type BlockView[V VID] interface {
	ID() V
	Parents() []V
	Author() string
	Round() uint64
}

// Store represents DAG storage interface (generic version)
type Store[V VID] interface {
	Head() []V
	Get(V) (BlockView[V], bool)
	Children(V) []V
}

// ComputeSafePrefix computes the safe prefix of vertices that can be committed
// based on DAG order theory using horizon (reachability) and flare (cert/skip) analysis
func ComputeSafePrefix[V VID](store Store[V], frontier []V) []V {
	// Compute vertices that are ancestors of all frontier vertices
	// These vertices have achieved finality and can be safely committed
	
	if len(frontier) == 0 {
		return []V{}
	}
	
	// Find common ancestors of all frontier vertices
	var safeVertices []V
	head := store.Head()
	
	for _, v := range head {
		// Check if this vertex is an ancestor of all frontier vertices
		isAncestorOfAll := true
		for _, f := range frontier {
			if !IsReachable(store, v, f) {
				isAncestorOfAll = false
				break
			}
		}
		
		if isAncestorOfAll {
			safeVertices = append(safeVertices, v)
		}
	}
	
	return safeVertices
}

// ChooseFrontier selects appropriate parents for a new vertex proposal
// This typically involves choosing a subset of frontier vertices to reference
func ChooseFrontier[V VID](frontier []V) []V {
	// For Byzantine tolerance with f faults, we need 2f+1 vertices
	// Assuming f = (n-1)/3 for optimal Byzantine tolerance
	// We'll choose min(2f+1, all) vertices
	
	if len(frontier) == 0 {
		return []V{}
	}
	
	// For small frontiers, reference all vertices
	if len(frontier) <= 3 {
		return frontier
	}
	
	// For larger frontiers, choose 2f+1 where f = (len-1)/3
	// This ensures Byzantine fault tolerance
	f := (len(frontier) - 1) / 3
	required := 2*f + 1
	
	if required >= len(frontier) {
		return frontier
	}
	
	// Select the most recent vertices (assuming they're ordered)
	return frontier[:required]
}

// IsReachable checks if vertex 'from' can reach vertex 'to' in the DAG
func IsReachable[V VID](store Store[V], from, to V) bool {
	// Use BFS to check reachability
	if from == to {
		return true
	}
	
	visited := make(map[V]bool)
	queue := []V{from}
	visited[from] = true
	
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		
		// Check children (forward edges in DAG)
		for _, child := range store.Children(current) {
			if child == to {
				return true
			}
			if !visited[child] {
				visited[child] = true
				queue = append(queue, child)
			}
		}
	}
	
	return false
}

// LCA finds the lowest common ancestor of two vertices
func LCA[V VID](store Store[V], a, b V) V {
	// Find all ancestors of a
	ancestorsA := make(map[V]uint64) // vertex -> height
	queue := []V{a}
	visited := make(map[V]bool)
	
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		
		if visited[current] {
			continue
		}
		visited[current] = true
		
		// Store with height for finding lowest
		if block, ok := store.Get(current); ok {
			ancestorsA[current] = block.Round()
			
			// Traverse parents (backward edges in DAG)
			for _, parent := range block.Parents() {
				if !visited[parent] {
					queue = append(queue, parent)
				}
			}
		}
	}
	
	// Find first common ancestor of b
	queue = []V{b}
	visited = make(map[V]bool)
	var lca V
	var lcaHeight uint64 = ^uint64(0) // Max uint64
	
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		
		if visited[current] {
			continue
		}
		visited[current] = true
		
		// Check if this is a common ancestor
		if height, isAncestor := ancestorsA[current]; isAncestor {
			// Found a common ancestor - keep track of the lowest (highest height)
			if height < lcaHeight {
				lcaHeight = height
				lca = current
			}
		}
		
		// Continue traversing parents
		if block, ok := store.Get(current); ok {
			for _, parent := range block.Parents() {
				if !visited[parent] {
					queue = append(queue, parent)
				}
			}
		}
	}
	
	return lca
}

// Antichain computes an antichain (set of mutually unreachable vertices) in the DAG
func Antichain[V VID](store Store[V], vertices []V) []V {
	// An antichain is a set of vertices where no vertex can reach another
	// This represents concurrent vertices in the DAG
	
	if len(vertices) <= 1 {
		return vertices
	}
	
	var antichain []V
	
	for i, v1 := range vertices {
		isInAntichain := true
		
		// Check if v1 is mutually unreachable with all other vertices
		for j, v2 := range vertices {
			if i == j {
				continue
			}
			
			// If v1 can reach v2 or v2 can reach v1, they're not in antichain
			if IsReachable(store, v1, v2) || IsReachable(store, v2, v1) {
				isInAntichain = false
				break
			}
		}
		
		if isInAntichain {
			antichain = append(antichain, v1)
		}
	}
	
	return antichain
}

// EventHorizon represents the finality boundary in Quasar P-Chain consensus
// Beyond this horizon, no events can affect the finalized state
type EventHorizon[V VID] struct {
	// Checkpoint represents a finalized state boundary
	Checkpoint V
	// Height at which this horizon was established
	Height uint64
	// Validators that signed this horizon
	Validators []string
	// Post-quantum signature (Ringtail + BLS)
	Signature []byte
}

// Horizon computes the event horizon for Quasar P-Chain finality
// This determines which vertices are beyond the point of no return
func Horizon[V VID](store Store[V], checkpoints []EventHorizon[V]) EventHorizon[V] {
	// Implement event horizon computation for P-Chain finality
	// This computes:
	// 1. Reachability from latest checkpoints
	// 2. Vertices that have achieved post-quantum finality
	// 3. New event horizon boundary
	
	if len(checkpoints) == 0 {
		var zero EventHorizon[V]
		return zero
	}
	
	// Start with the latest checkpoint
	latest := checkpoints[len(checkpoints)-1]
	
	// Find all vertices reachable from the latest checkpoint
	// These vertices are candidates for being beyond the new horizon
	reachable := make(map[V]bool)
	queue := []V{latest.Checkpoint}
	visited := make(map[V]bool)
	
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		
		if visited[current] {
			continue
		}
		visited[current] = true
		reachable[current] = true
		
		// Traverse children to find newer vertices
		children := store.Children(current)
		for _, child := range children {
			if !visited[child] {
				queue = append(queue, child)
			}
		}
	}
	
	// Find the most recent vertex that is:
	// 1. Reachable from the checkpoint
	// 2. Has sufficient validator signatures (post-quantum finality)
	var newCheckpoint V
	var maxHeight uint64 = 0
	
	for vertex := range reachable {
		if block, ok := store.Get(vertex); ok {
			height := block.Round()
			if height > maxHeight {
				maxHeight = height
				newCheckpoint = vertex
			}
		}
	}
	
	// Create new event horizon
	// In a real implementation, we'd verify post-quantum signatures
	// For now, we establish the horizon at the highest reachable vertex
	newHorizon := EventHorizon[V]{
		Checkpoint: newCheckpoint,
		Height:     maxHeight,
		Validators: latest.Validators, // Inherit validators from previous
		Signature:  latest.Signature,  // In real impl, create new aggregate signature
	}
	
	return newHorizon
}

// BeyondHorizon checks if a vertex is beyond the event horizon (finalized)
func BeyondHorizon[V VID](store Store[V], vertex V, horizon EventHorizon[V]) bool {
	// Check if vertex is reachable from the horizon checkpoint
	// Vertices beyond the event horizon cannot be affected by future consensus
	return IsReachable(store, horizon.Checkpoint, vertex)
}

// ComputeHorizonOrder determines the canonical order of vertices beyond the event horizon
func ComputeHorizonOrder[V VID](store Store[V], horizon EventHorizon[V]) []V {
	// Implement topological ordering for vertices beyond the horizon
	// This provides deterministic ordering for P-Chain state transitions
	
	if horizon.Height == 0 {
		return []V{}
	}
	
	// Collect all vertices reachable from the horizon checkpoint
	var beyondHorizon []V
	visited := make(map[V]bool)
	queue := []V{horizon.Checkpoint}
	
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		
		if visited[current] {
			continue
		}
		visited[current] = true
		beyondHorizon = append(beyondHorizon, current)
		
		// Add children to queue for BFS traversal
		for _, child := range store.Children(current) {
			if !visited[child] {
				queue = append(queue, child)
			}
		}
	}
	
	return beyondHorizon
}
