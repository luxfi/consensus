package types

import "time"

type NodeID string
type Topic string

type Probe int
const (
    ProbeGood Probe = iota
    ProbeTimeout
    ProbeBadSig
)

type Decision int
const (
    DecideAccept Decision = iota
    DecideReject
    DecideUndecided
)

type Digest [32]byte

type Round struct {
    Height uint64
    Time   time.Time
}
