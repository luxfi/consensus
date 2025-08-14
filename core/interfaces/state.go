package interfaces

// State represents consensus state
type State int

const (
    Bootstrapping State = iota
    NormalOp
)

func (s State) String() string {
    switch s {
    case Bootstrapping:
        return "Bootstrapping"
    case NormalOp:
        return "NormalOp"
    default:
        return "Unknown"
    }
}
