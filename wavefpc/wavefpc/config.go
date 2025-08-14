package wavefpc

type Quorum struct{ N, F int }

func (q Quorum) Threshold() int { return 2*q.F + 1 }

type Config struct {
    Quorum            Quorum
    Epoch             uint64
    VoteLimitPerBlock int    // proposer budget (e.g., 128-512)
    VotePrefix        []byte // domain sep for BLS/PQ proofs
}
