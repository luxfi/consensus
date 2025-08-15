package wavefpc

// Committee maps authors to indices in [0,N).
type Committee interface {
	Size() int
	// author is your node identity bytes (whatever your block exposes).
	IndexOf(author []byte) (ValidatorIndex, bool)
}

// Classifier returns "owned" inputs for a tx and conflict relation.
type Classifier interface {
	OwnedInputs(tx TxRef) []ObjectID
	Conflicts(a, b TxRef) bool
}

// DAGTap answers fast ancestry queries (accepted anchor coverage).
type DAGTap interface {
	InAncestry(blockID []byte, needleTx TxRef) bool
}

// PQEngine is optional. Use it to kick Ringtail (or other PQ) in parallel.
type PQEngine interface {
	Submit(tx TxRef, voters []ValidatorIndex)
	HasPQ(tx TxRef) bool
}

// CandidateSource feeds the proposer with eligible txs (owned, non-conflicting).
type CandidateSource interface {
	Eligible(max int) []TxRef
}

// ObservedBlock is a tiny adapter so this package stays engine-agnostic.
type ObservedBlock struct {
	ID       []byte
	Author   []byte  // author identity bytes (Committee.IndexOf maps this)
	FPCVotes []TxRef // vote payload
	EpochBit bool    // epoch closing marker
}
