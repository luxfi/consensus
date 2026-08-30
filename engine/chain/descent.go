// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// descent.go — finalized history addressed by POSITION.
//
// A node that has fallen behind knows exactly one thing about what it needs: its
// own height. It does not know the id of the block above it, and not knowing that
// id IS the condition it is in.
//
// Recovery asked it for that id anyway. requestCatchup takes a missing block id,
// and CertForBlock is keyed by id, so the only ids a behind node can name are the
// ones it has already been handed — which are the tip's, because that is what
// live gossip carries. It asks about the tip, the tip does not extend its head,
// and the height directly above its head is never named by anyone. Observed on
// mainnet as `advanced=0` held across 904 consecutive samples with one block id
// repeating 396 times.
//
// The question is asked the other way here: give me what follows height H.
// Identity becomes the answer instead of the argument.
package chain

import (
	"context"
	"errors"
	"fmt"

	"github.com/luxfi/ids"
)

// ErrNoDescent means this node cannot serve the requested position: it never
// finalized that height, or the height has aged out of the recovery index. It is
// a clean miss — the caller asks another peer — and never a claim that the height
// does not exist.
var ErrNoDescent = errors.New("chain: no finalized history at that position")

// Certified is a finalized block together with the quorum proof of its finality.
//
// Neither half is a value on its own: a block without its cert cannot be
// accepted, and a cert without its block has nothing to accept. They are one
// type so that holding half of one is not expressible.
type Certified struct {
	Block []byte
	Cert  []byte
}

// Run is finalized history from Base upward, contiguous BY CONSTRUCTION —
// Chain[i] is the block at height Base+i.
//
// The type cannot express a hole. That is the point: a gap cannot be served,
// cannot be cached, and cannot be mistaken for progress by a caller counting
// what it received. Contiguity stops being an invariant someone has to check.
type Run struct {
	Base  uint64
	Chain []Certified
}

// Next is the position one past the end, which is what a caller asks for to
// continue. An empty run asks again at the same place rather than skipping one.
func (r Run) Next() uint64 { return r.Base + uint64(len(r.Chain)) }

// Descent serves finalized history from a height. One method, because there is
// one question.
//
// It does not replace CertForBlock, which answers a different question — the
// proof for a block already named — and is right whenever the id is genuinely
// known. Neither subsumes the other.
type Descent interface {
	From(ctx context.Context, height uint64, max int) (Run, error)
}

// From implements Descent over this node's own finalized history.
//
// It walks upward from height and stops at the first position it cannot serve,
// returning what it has. Stopping early is not an error: a short run is honest
// about where this node's knowledge ends, and the caller continues from
// Run.Next() against this peer or another. Returning a longer run with a hole in
// it is the one outcome that must be impossible, and the type makes it so.
func (rt *Runtime) From(ctx context.Context, height uint64, max int) (Run, error) {
	if rt == nil || rt.Transitive == nil || rt.config.VM == nil {
		return Run{}, ErrNoDescent
	}
	if max <= 0 {
		return Run{Base: height}, nil
	}

	// The asker names max, so the reservation cannot be sized from it. A peer that
	// writes MaxInt costs a `makeslice: cap out of range` panic in one packet, and
	// this is the only peer-sized quantity in the package that was not capped at
	// the read — vote_count is checked against the remaining frame, sig_len
	// against the buffer, the Merkle step count at 64, and the served-cert window
	// is a constant.
	//
	// Nothing beyond that window can be served anyway, so a larger request is not
	// a bigger answer; it is only a bigger allocation. The loop below stops at the
	// first height this node cannot answer, so capping the RESERVATION changes no
	// reply — the run is built by appending, not by filling.
	if max > maxServedCerts {
		max = maxServedCerts
	}
	run := Run{Base: height, Chain: make([]Certified, 0, max)}
	for h := height; h < height+uint64(max); h++ {
		id, ok := rt.finalizedAt(h)
		if !ok {
			break
		}
		// The block comes first: the cert is filed under the DECISION its signatures
		// cover, and the block is what names it (canonicalIDOf, inside CertForBlock).
		// Asking by the outer id from the recovery index would miss a cert this node
		// holds under a sibling wrapper of the same inner block.
		blk, err := rt.config.VM.GetBlock(ctx, id)
		if err != nil || blk == nil {
			break
		}
		cert, ok := rt.Transitive.CertForBlock(blk)
		if !ok {
			break
		}
		run.Chain = append(run.Chain, Certified{Block: blk.Bytes(), Cert: cert})
	}

	if len(run.Chain) == 0 {
		return Run{Base: height}, fmt.Errorf("%w: height %d", ErrNoDescent, height)
	}
	return run, nil
}

// finalizedAt names the block this node finalized at h, from the recovery index
// the accept path already maintains. Reading it here rather than keeping a second
// index is deliberate: two indexes of the same fact drift, and the drift is only
// visible during a recovery, which is the moment it must be trusted.
func (rt *Runtime) finalizedAt(h uint64) (ids.ID, bool) {
	t := rt.Transitive
	t.mu.RLock()
	defer t.mu.RUnlock()
	id, ok := t.recoveredAt[h]
	return id, ok
}
