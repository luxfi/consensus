package dag

import (
    "context"
    "testing"
    
    "github.com/luxfi/consensus/core/dag"
)

// mockMeta implements dag.Meta for testing
type mockMeta struct {
    id      dag.VertexID
    author  string
    round   uint64
    parents []dag.VertexID
}

func (m mockMeta) ID() dag.VertexID     { return m.id }
func (m mockMeta) Author() string       { return m.author }
func (m mockMeta) Round() uint64        { return m.round }
func (m mockMeta) Parents() []dag.VertexID { return m.parents }

// mockView implements dag.View for testing
type mockView struct {
    vertices map[dag.VertexID]dag.Meta
}

func (v mockView) Get(id dag.VertexID) (dag.Meta, bool) {
    m, ok := v.vertices[id]
    return m, ok
}

func (v mockView) ByRound(round uint64) []dag.Meta {
    var result []dag.Meta
    for _, m := range v.vertices {
        if m.Round() == round {
            result = append(result, m)
        }
    }
    return result
}

func (v mockView) Supports(from dag.VertexID, author string, round uint64) bool {
    // Simple mock implementation
    return true
}

func TestNew(t *testing.T) {
    e := New(5, 1) // 5 nodes, 1 fault
    if e == nil {
        t.Fatal("New returned nil")
    }
}

func TestEngine_Tick(t *testing.T) {
    e := New(5, 1)
    ctx := context.Background()
    
    // Test tick with various views and proposers
    view := mockView{
        vertices: make(map[dag.VertexID]dag.Meta),
    }
    
    proposers := []dag.Meta{
        mockMeta{id: dag.VertexID{1}, author: "proposer1", round: 1},
        mockMeta{id: dag.VertexID{2}, author: "proposer2", round: 1},
        mockMeta{id: dag.VertexID{3}, author: "proposer3", round: 1},
    }
    
    e.Tick(ctx, view, proposers)
    // Engine processes internally
}

func TestEngine_MultipleViews(t *testing.T) {
    e := New(5, 1)
    ctx := context.Background()
    
    // Test multiple views/rounds
    for round := uint64(1); round < 10; round++ {
        view := mockView{
            vertices: make(map[dag.VertexID]dag.Meta),
        }
        proposers := []dag.Meta{
            mockMeta{id: dag.VertexID{byte(round)}, author: "proposer", round: round},
        }
        e.Tick(ctx, view, proposers)
    }
}

func TestEngine_FaultTolerance(t *testing.T) {
    tests := []struct {
        name string
        n    int
        f    int
    }{
        {"3 nodes 1 fault", 3, 1},
        {"5 nodes 1 fault", 5, 1},
        {"5 nodes 2 faults", 5, 2},
        {"7 nodes 2 faults", 7, 2},
        {"21 nodes 7 faults", 21, 7},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            e := New(tt.n, tt.f)
            if e == nil {
                t.Fatal("New returned nil")
            }
            
            ctx := context.Background()
            view := mockView{vertices: make(map[dag.VertexID]dag.Meta)}
            proposers := []dag.Meta{mockMeta{id: dag.VertexID{1}, author: "test", round: 1}}
            e.Tick(ctx, view, proposers)
        })
    }
}

func BenchmarkEngine_Tick(b *testing.B) {
    e := New(5, 1)
    ctx := context.Background()
    view := mockView{vertices: make(map[dag.VertexID]dag.Meta)}
    proposers := []dag.Meta{mockMeta{id: dag.VertexID{1}, author: "bench", round: 1}}
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        e.Tick(ctx, view, proposers)
    }
}

func BenchmarkEngine_LargeNetwork(b *testing.B) {
    e := New(21, 7)
    ctx := context.Background()
    view := mockView{vertices: make(map[dag.VertexID]dag.Meta)}
    
    // Many proposers
    proposers := make([]dag.Meta, 21)
    for i := range proposers {
        proposers[i] = mockMeta{
            id:     dag.VertexID{byte(i)},
            author: string(rune(i)),
            round:  1,
        }
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        e.Tick(ctx, view, proposers)
    }
}