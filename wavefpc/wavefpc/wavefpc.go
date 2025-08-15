package wavefpc

import (
	"sync"
	"sync/atomic"
)

type WaveFPC interface {
	OnBlockObserved(b *ObservedBlock)
	OnBlockAccepted(b *ObservedBlock)
	OnEpochCloseStart()
	OnEpochClosed()

	NextVotes(budget int) []TxRef
	Status(tx TxRef) (Status, Proof)

	// MarkMixed: gate execution of mixed txs until Final.
	MarkMixed(tx TxRef)
}

type waveFPC struct {
	cfg       Config
	committee Committee
	cls       Classifier
	dag       DAGTap
	pq        PQEngine
	src       CandidateSource

	me ValidatorIndex

	epochPaused atomic.Bool

	mu sync.RWMutex

	// votes[tx] -> voters bitset
	votes map[TxRef]*bitset
	// per-object non-equivocation: (validator||object) -> tx
	votedOn map[[64]byte]TxRef
	// state[tx] -> Pending|Executable|Final
	state map[TxRef]Status
	// mixed[tx] -> true if tx requires Final (owned+shared)
	mixed map[TxRef]bool
}

func New(cfg Config, committee Committee, me ValidatorIndex, cls Classifier, dag DAGTap, pq PQEngine, src CandidateSource) WaveFPC {
	return &waveFPC{
		cfg:       cfg,
		committee: committee,
		cls:       cls,
		dag:       dag,
		pq:        pq,
		src:       src,
		me:        me,

		votes:   make(map[TxRef]*bitset),
		votedOn: make(map[[64]byte]TxRef),
		state:   make(map[TxRef]Status),
		mixed:   make(map[TxRef]bool),
	}
}

func (w *waveFPC) NextVotes(budget int) []TxRef {
	if budget <= 0 || w.epochPaused.Load() {
		return nil
	}
	if w.src == nil {
		return nil
	}
	cands := w.src.Eligible(min(budget, w.cfg.VoteLimitPerBlock))
	if len(cands) == 0 {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	var out []TxRef
	for _, T := range cands {
		owned := w.cls.OwnedInputs(T)
		if len(owned) == 0 {
			continue
		}
		conflicts := false
		for _, o := range owned {
			key := kvKey(w.me, o)
			if _, ok := w.votedOn[key]; ok {
				conflicts = true
				break
			}
		}
		if conflicts {
			continue
		}
		for _, o := range owned {
			w.votedOn[kvKey(w.me, o)] = T
		}
		out = append(out, T)
		if len(out) >= budget {
			break
		}
	}
	return out
}

func (w *waveFPC) OnBlockObserved(b *ObservedBlock) {
	if b == nil || len(b.FPCVotes) == 0 {
		return
	}
	idx, ok := w.committee.IndexOf(b.Author)
	if !ok {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, T := range b.FPCVotes {
		owned := w.cls.OwnedInputs(T)
		if len(owned) == 0 {
			continue
		}
		// per-object non-equivocation by author
		for _, o := range owned {
			key := kvKey(idx, o)
			if prev, ok := w.votedOn[key]; ok && prev != T {
				continue // equivocation on same object doesn't double count
			}
			if !ok {
				w.votedOn[key] = T
			}
		}
		bs := w.votes[T]
		if bs == nil {
			bs = newBitset(w.committee.Size())
			w.votes[T] = bs
		}
		if bs.Set(int(idx)) && bs.Count() >= w.cfg.Quorum.Threshold() {
			if w.state[T] == Pending {
				w.state[T] = Executable
				if w.pq != nil {
					w.pq.Submit(T, votersFromBitset(bs))
				}
			}
		}
	}
	// EpochBit is only tallied on acceptance fence in this simple skeleton.
	_ = b.EpochBit
}

func (w *waveFPC) OnBlockAccepted(b *ObservedBlock) {
	if b == nil || len(b.FPCVotes) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, T := range b.FPCVotes {
		if w.state[T] != Executable {
			continue
		}
		// Path B (anchored): accepted anchor covers the votes
		if w.dag != nil && w.dag.InAncestry(b.ID, T) {
			w.state[T] = Final
		}
	}
}

func (w *waveFPC) OnEpochCloseStart() { w.epochPaused.Store(true) }
func (w *waveFPC) OnEpochClosed()     { w.epochPaused.Store(false) }

func (w *waveFPC) Status(tx TxRef) (Status, Proof) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	st := w.state[tx]
	var proof Proof
	if bs := w.votes[tx]; bs != nil && bs.Count() >= w.cfg.Quorum.Threshold() {
		proof.VoterBitmap = bs.Bitmap()
	}
	// BLSAgg / PQCert are left to the integrator to populate when exposed.
	return st, proof
}

func (w *waveFPC) MarkMixed(tx TxRef) {
	w.mu.Lock()
	w.mixed[tx] = true
	w.mu.Unlock()
}

func kvKey(v ValidatorIndex, o ObjectID) (key [64]byte) {
	copy(key[:2], []byte{byte(v >> 8), byte(v)})
	copy(key[32:], o[:])
	return
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
