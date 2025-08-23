package horizon

type VertexID [32]byte

type Meta interface {
    ID() VertexID
    Author() string
    Round() uint64
    Parents() []VertexID
}

type View interface {
    Get(VertexID) (Meta, bool)
    ByRound(round uint64) []Meta
    Supports(from VertexID, author string, round uint64) bool
}

type Params struct{ N, F int }
