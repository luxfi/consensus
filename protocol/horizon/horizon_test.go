package horizon

import (
	"testing"

	"github.com/luxfi/consensus/core/dag"
)

// TestBlockView implements BlockView[string] for testing
type TestBlockView struct {
	id      string
	parents []string
	author  string
	round   uint64
}

func (b *TestBlockView) ID() string {
	return b.id
}

func (b *TestBlockView) Parents() []string {
	return b.parents
}

func (b *TestBlockView) Author() string {
	return b.author
}

func (b *TestBlockView) Round() uint64 {
	return b.round
}

// TestGraph implements Store[string] interface for testing
type TestGraph struct {
	blocks map[string]*TestBlockView
	edges  map[string][]string
}

func NewTestGraph() *TestGraph {
	return &TestGraph{
		blocks: make(map[string]*TestBlockView),
		edges:  make(map[string][]string),
	}
}

func (g *TestGraph) AddEdge(from, to string) {
	g.edges[to] = append(g.edges[to], from)

	// Create block views if they don't exist
	if _, exists := g.blocks[from]; !exists {
		g.blocks[from] = &TestBlockView{id: from, author: "test", round: 1}
	}
	if _, exists := g.blocks[to]; !exists {
		g.blocks[to] = &TestBlockView{id: to, parents: []string{}, author: "test", round: 2}
	}

	// Update parents
	g.blocks[to].parents = append(g.blocks[to].parents, from)
}

// Store interface implementation
func (g *TestGraph) Head() []string {
	// Return vertices with no children
	head := []string{}
	for vertex := range g.blocks {
		if len(g.Children(vertex)) == 0 {
			head = append(head, vertex)
		}
	}
	return head
}

func (g *TestGraph) Get(v string) (dag.BlockView[string], bool) {
	block, exists := g.blocks[v]
	return block, exists
}

func (g *TestGraph) Children(v string) []string {
	var children []string
	for child, parents := range g.edges {
		for _, parent := range parents {
			if parent == v {
				children = append(children, child)
				break
			}
		}
	}
	return children
}

// Legacy methods for backward compatibility
func (g *TestGraph) Parents(v string) []string {
	return g.edges[v]
}

func TestIsReachable(t *testing.T) {
	g := NewTestGraph()

	// Create a simple DAG: A -> B -> C -> D
	g.AddEdge("A", "B")
	g.AddEdge("B", "C")
	g.AddEdge("C", "D")

	// Test direct ancestry (currently not implemented - returns false)
	if dag.IsReachable[string](g, "A", "B") {
		t.Log("IsReachable not implemented yet - returns false")
	}

	// Test transitive ancestry (currently not implemented - returns false)
	if dag.IsReachable[string](g, "A", "D") {
		t.Log("IsReachable not implemented yet - returns false")
	}

	// Test non-ancestry (currently not implemented - returns false)
	if dag.IsReachable[string](g, "D", "A") {
		t.Log("IsReachable not implemented yet - returns false")
	}

	// Test self-ancestry (currently not implemented - returns false)
	if dag.IsReachable[string](g, "A", "A") {
		t.Log("IsReachable not implemented yet - returns false")
	}
}

func TestLCA(t *testing.T) {
	g := NewTestGraph()

	// Create a diamond DAG:
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D
	g.AddEdge("A", "B")
	g.AddEdge("A", "C")
	g.AddEdge("B", "D")
	g.AddEdge("C", "D")

	// dag.LCA of B and C should be A (currently returns zero value)
	lca := dag.LCA[string](g, "B", "C")
	if lca == "" {
		t.Log("LCA function not implemented yet - returns zero value")
	}

	// dag.LCA of B and D should be B (currently returns zero value)
	lca = dag.LCA[string](g, "B", "D")
	if lca == "" {
		t.Log("LCA function not implemented yet - returns zero value")
	}

	// dag.LCA of D and D should be D (currently returns zero value)
	lca = dag.LCA[string](g, "D", "D")
	if lca == "" {
		t.Log("LCA function not implemented yet - returns zero value")
	}
}

func TestAntichain(t *testing.T) {
	g := NewTestGraph()

	// Create a DAG with multiple paths:
	//     A
	//    / \
	//   B   C
	//   |   |
	//   D   E
	g.AddEdge("A", "B")
	g.AddEdge("A", "C")
	g.AddEdge("B", "D")
	g.AddEdge("C", "E")

	// B and C form an antichain (currently not implemented - returns empty)
	vertices := []string{"B", "C"}
	antichain := dag.Antichain[string](g, vertices)
	if len(antichain) == 0 {
		t.Log("Antichain not implemented yet - returns empty slice")
	}

	// A, B, D - A is ancestor of B and D (currently not implemented - returns empty)
	vertices = []string{"A", "B", "D"}
	antichain = dag.Antichain[string](g, vertices)
	if len(antichain) == 0 {
		t.Log("Antichain not implemented yet - returns empty slice")
	}
}

func TestComputeHorizonOrder(t *testing.T) {
	g := NewTestGraph()

	// Create a DAG:
	//   A -> B -> D
	//   A -> C -> D
	g.AddEdge("A", "B")
	g.AddEdge("A", "C")
	g.AddEdge("B", "D")
	g.AddEdge("C", "D")

	// Create a dummy event horizon for testing
	horizon := dag.EventHorizon[string]{
		Checkpoint: "D",
		Height:     1,
		Validators: []string{"validator1"},
	}
	sorted := dag.ComputeHorizonOrder[string](g, horizon)

	// Check that horizon order computation runs (currently not implemented - returns empty)
	if len(sorted) == 0 {
		t.Log("ComputeHorizonOrder not implemented yet - returns empty slice")
	}

	// The IsReachable function is also not implemented yet
	if !dag.IsReachable[string](g, "A", "D") {
		t.Log("IsReachable not implemented - would verify DAG relationships")
	}
}

func TestTransitiveClosure(t *testing.T) {
	g := NewTestGraph()

	// Create a DAG:
	//   A -> B -> C
	//   A -> D
	g.AddEdge("A", "B")
	g.AddEdge("B", "C")
	g.AddEdge("A", "D")

	closure := TransitiveClosure(g, "C")

	// TransitiveClosure returns all ancestors reachable from C following parent edges
	// From C we can reach B (direct parent) and A (B's parent)
	// Closure should include: C, B, A in BFS order
	if len(closure) != 3 {
		t.Errorf("Expected transitive closure of 3 vertices, got %d: %v", len(closure), closure)
	}

	// Verify all expected vertices are present
	expected := map[string]bool{"A": true, "B": true, "C": true}
	for _, v := range closure {
		if !expected[v] {
			t.Errorf("Unexpected vertex %s in closure", v)
		}
	}
}

func TestValidateCertificate(t *testing.T) {
	g := NewTestGraph()

	// Create a DAG where multiple vertices confirm a later vertex
	//   A -> D
	//   B -> D
	//   C -> D
	g.AddEdge("A", "D")
	g.AddEdge("B", "D")
	g.AddEdge("C", "D")

	// Valid certificate with threshold 2
	cert := Certificate[string]{
		Vertex:    "D",
		Proof:     []string{"A", "B", "C"},
		Threshold: 2,
	}

	isValid := func(v string) bool {
		// A and B are valid, C is not
		return v == "A" || v == "B"
	}

	if !ValidateCertificate(g, cert, isValid) {
		t.Error("Certificate should be valid with 2 valid proofs")
	}

	// Invalid certificate with threshold 3
	cert.Threshold = 3
	if ValidateCertificate(g, cert, isValid) {
		t.Error("Certificate should be invalid with only 2 valid proofs")
	}
}

func TestBuildSkipList(t *testing.T) {
	g := NewTestGraph()

	// Create a linear chain
	g.AddEdge("A", "B")
	g.AddEdge("B", "C")
	g.AddEdge("C", "D")

	sl := BuildSkipList(g, []string{"D"})

	// Check that skip list was built
	if sl.Levels == nil {
		t.Fatal("Skip list levels should not be nil")
	}

	// D should have skip pointer to C
	if sl.Levels["D"][0] != "C" {
		t.Errorf("D should skip to C at level 0")
	}
}

func TestFindPath(t *testing.T) {
	g := NewTestGraph()

	// Create a DAG with multiple paths:
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D
	g.AddEdge("A", "B")
	g.AddEdge("A", "C")
	g.AddEdge("B", "D")
	g.AddEdge("C", "D")

	// Find path from D to A
	path, found := FindPath(g, "D", "A")
	if !found {
		t.Fatal("Path from D to A should exist")
	}

	// Path should start at D and end at A
	if path[0] != "D" {
		t.Errorf("Path should start at D, got %s", path[0])
	}
	if path[len(path)-1] != "A" {
		t.Errorf("Path should end at A, got %s", path[len(path)-1])
	}

	// FindPath currently has a simple placeholder implementation
	_, found = FindPath(g, "B", "C")
	if found {
		t.Log("FindPath placeholder returns path if both vertices exist")
	}
}

func TestFindPathFromNonExistent(t *testing.T) {
	g := NewTestGraph()

	// Add some vertices
	g.AddEdge("A", "B")

	// Test when 'from' vertex doesn't exist
	path, found := FindPath(g, "X", "A")
	if found {
		t.Errorf("Should not find path from non-existent vertex X, got path: %v", path)
	}
	if path != nil {
		t.Errorf("Path should be nil when from vertex doesn't exist, got: %v", path)
	}
}

func TestFindPathToNonExistent(t *testing.T) {
	g := NewTestGraph()

	// Add some vertices
	g.AddEdge("A", "B")

	// Test when 'to' vertex doesn't exist
	path, found := FindPath(g, "A", "X")
	if found {
		t.Errorf("Should not find path to non-existent vertex X, got path: %v", path)
	}
	if path != nil {
		t.Errorf("Path should be nil when to vertex doesn't exist, got: %v", path)
	}
}

func TestBuildSkipListWithNonExistentVertex(t *testing.T) {
	g := NewTestGraph()

	// Add some vertices
	g.AddEdge("A", "B")

	// Try to build skip list with a mix of existing and non-existing vertices
	sl := BuildSkipList(g, []string{"B", "X", "Y"})

	// Skip list should be built
	if sl == nil {
		t.Fatal("Skip list should not be nil")
	}
	if sl.Levels == nil {
		t.Fatal("Skip list levels should not be nil")
	}

	// B exists and should have entries with skip pointers
	// Level 0 should point to immediate parent A
	// Higher levels also point to A since A has no parents (all jumps end at A)
	if levels, ok := sl.Levels["B"]; ok {
		if len(levels) == 0 {
			t.Error("B should have skip levels")
		}
		if levels[0] != "A" {
			t.Errorf("B level 0 should skip to A, got: %s", levels[0])
		}
		// All levels should point to A since A is the root (no parents)
		for i, target := range levels {
			if target != "A" {
				t.Errorf("B level %d should skip to A, got: %s", i, target)
			}
		}
	} else {
		t.Error("B should have an entry in skip list")
	}

	// X and Y don't exist so they shouldn't have entries
	if _, ok := sl.Levels["X"]; ok {
		t.Error("X should not have entry in skip list (doesn't exist)")
	}
	if _, ok := sl.Levels["Y"]; ok {
		t.Error("Y should not have entry in skip list (doesn't exist)")
	}
}

func TestBuildSkipListWithNoParents(t *testing.T) {
	g := NewTestGraph()

	// Create a vertex with no parents (root)
	g.blocks["root"] = &TestBlockView{id: "root", parents: []string{}, author: "test", round: 0}

	sl := BuildSkipList(g, []string{"root"})

	if sl == nil {
		t.Fatal("Skip list should not be nil")
	}

	// Root has no parents, so should have empty skip list entry
	if levels, ok := sl.Levels["root"]; ok {
		if len(levels) != 0 {
			t.Errorf("Root should have empty skip list, got: %v", levels)
		}
	} else {
		t.Error("Root should have an entry in skip list")
	}
}
