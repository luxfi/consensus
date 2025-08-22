package horizon

import (
	"testing"
)

// TestGraph implements Graph interface for testing
type TestGraph struct {
	edges map[string][]string
}

func NewTestGraph() *TestGraph {
	return &TestGraph{
		edges: make(map[string][]string),
	}
}

func (g *TestGraph) AddEdge(from, to string) {
	g.edges[to] = append(g.edges[to], from)
}

func (g *TestGraph) Parents(v string) []string {
	return g.edges[v]
}

func TestIsAncestor(t *testing.T) {
	g := NewTestGraph()
	
	// Create a simple DAG: A -> B -> C -> D
	g.AddEdge("A", "B")
	g.AddEdge("B", "C")
	g.AddEdge("C", "D")
	
	// Test direct ancestry
	if !IsAncestor(g, "A", "B") {
		t.Error("A should be ancestor of B")
	}
	
	// Test transitive ancestry
	if !IsAncestor(g, "A", "D") {
		t.Error("A should be ancestor of D")
	}
	
	// Test non-ancestry
	if IsAncestor(g, "D", "A") {
		t.Error("D should not be ancestor of A")
	}
	
	// Test self-ancestry
	if !IsAncestor(g, "A", "A") {
		t.Error("A should be ancestor of itself")
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
	
	// LCA of B and C should be A
	lca, found := LCA(g, "B", "C")
	if !found || lca != "A" {
		t.Errorf("LCA of B and C should be A, got %v (found=%v)", lca, found)
	}
	
	// LCA of B and D should be B
	lca, found = LCA(g, "B", "D")
	if !found || lca != "B" {
		t.Errorf("LCA of B and D should be B, got %v (found=%v)", lca, found)
	}
	
	// LCA of D and D should be D
	lca, found = LCA(g, "D", "D")
	if !found || lca != "D" {
		t.Errorf("LCA of D and D should be D, got %v (found=%v)", lca, found)
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
	
	// B and C form an antichain
	vertices := []string{"B", "C"}
	antichain := Antichain(g, vertices)
	if len(antichain) != 2 {
		t.Errorf("Expected antichain of size 2, got %d", len(antichain))
	}
	
	// A, B, D - A is ancestor of B and D
	vertices = []string{"A", "B", "D"}
	antichain = Antichain(g, vertices)
	if len(antichain) != 1 || antichain[0] != "D" {
		t.Errorf("Expected antichain [D], got %v", antichain)
	}
}

func TestTopologicalSort(t *testing.T) {
	g := NewTestGraph()
	
	// Create a DAG:
	//   A -> B -> D
	//   A -> C -> D
	g.AddEdge("A", "B")
	g.AddEdge("A", "C")
	g.AddEdge("B", "D")
	g.AddEdge("C", "D")
	
	sorted := TopologicalSort(g, []string{"D"})
	
	// Check that vertices are in the sorted list
	found := map[string]bool{}
	for _, v := range sorted {
		found[v] = true
	}
	
	if len(sorted) != 4 {
		t.Errorf("Expected 4 vertices in sorted order, got %d", len(sorted))
	}
	
	// Check all vertices are present
	for _, v := range []string{"A", "B", "C", "D"} {
		if !found[v] {
			t.Errorf("Vertex %s not found in topological sort", v)
		}
	}
	
	// The exact order depends on the traversal, but D should be reachable from all
	if !IsAncestor(g, "A", "D") || !IsAncestor(g, "B", "D") || !IsAncestor(g, "C", "D") {
		t.Error("D should be reachable from A, B, and C")
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
	
	// Closure of C should include A, B, C
	expected := map[string]bool{"A": true, "B": true, "C": true}
	if len(closure) != len(expected) {
		t.Errorf("Expected closure size %d, got %d", len(expected), len(closure))
	}
	
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
	
	// No path from B to C (siblings)
	_, found = FindPath(g, "B", "C")
	if found {
		t.Error("Should not find path between siblings B and C")
	}
}