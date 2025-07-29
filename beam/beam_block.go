// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/photon"
)

// Tracks the state of a beam block
type beamBlock struct {
	t *Topological

	// block that this node contains. For the genesis, this value will be nil
	blk Block

	// shouldFalter is set to true if this node, and all its descendants received
	// less than Alpha votes
	shouldFalter bool

	// sb is the focus instance used to decide which child is the canonical
	// child of this block. If this node has not had a child issued under it,
	// this value will be nil
	sb photon.Consensus

	// children is the set of blocks that have been issued that name this block
	// as their parent. If this node has not had a child issued under it, this value
	// will be nil
	children map[ids.ID]Block
}

func (n *beamBlock) AddChild(child Block) {
	childID := child.ID()

	// if the focus instance is nil, this is the first child. So the instance
	// should be initialized.
	if n.sb == nil {
		n.sb = photon.NewTree(n.t.Factory, n.t.params, childID)
		n.children = make(map[ids.ID]Block)
	} else {
		n.sb.Add(childID)
	}

	n.children[childID] = child
}

func (n *beamBlock) Decided() bool {
	// if the block is nil, then this is the genesis which is defined as
	// accepted
	return n.blk == nil || n.blk.Height() <= n.t.lastAcceptedHeight
}
