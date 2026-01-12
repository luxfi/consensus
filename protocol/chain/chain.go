package chain

import "github.com/luxfi/consensus/engine/chain/block"

// Block is the canonical chain block interface.
// This aliases the engine/chain/block.Block to avoid duplicate interfaces.
type Block = block.Block
