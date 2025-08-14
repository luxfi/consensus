// Package integration provides consensus-geth verkle integration
package integration

import (
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/trie"
	"github.com/luxfi/geth/trie/utils"
	"github.com/luxfi/geth/triedb/database"
	"github.com/luxfi/consensus/dag/witness"
)

// VerkleAdapter adapts geth's VerkleTrie for consensus witness validation
type VerkleAdapter struct {
	trie  *trie.VerkleTrie
	cache *witness.Cache
}

// NewVerkleAdapter creates a new adapter bridging geth verkle and consensus
func NewVerkleAdapter(root common.Hash, db database.NodeDatabase, pointCache *utils.PointCache) (*VerkleAdapter, error) {
	vt, err := trie.NewVerkleTrie(root, db, pointCache)
	if err != nil {
		return nil, err
	}
	
	// Create witness cache with soft policy for verkle nodes
	wCache := witness.NewCache(witness.Policy{
		Mode:     witness.Soft,
		MaxBytes: 100 * 1024 * 1024, // 100MB cache
	}, 100000, 1<<30) // 100k entries, 1GB max
	
	return &VerkleAdapter{
		trie:  vt,
		cache: wCache,
	}, nil
}

// ValidateWitness validates a verkle witness against the trie
func (v *VerkleAdapter) ValidateWitness(key []byte, value []byte, proof []byte) bool {
	// Use geth's verkle trie to verify
	storedValue, err := v.trie.Get(key)
	if err != nil {
		return false
	}
	
	// Check value matches
	if len(storedValue) != len(value) {
		return false
	}
	for i := range storedValue {
		if storedValue[i] != value[i] {
			return false
		}
	}
	
	// Cache the witness node
	nodeKey := witness.NodeKey{
		Stem: [32]byte{}, // Fill from key
	}
	copy(nodeKey.Stem[:], key[:min(32, len(key))])
	v.cache.PutNode(nodeKey, proof)
	
	return true
}

// GetCachedNode retrieves a cached verkle node
func (v *VerkleAdapter) GetCachedNode(key witness.NodeKey) ([]byte, bool) {
	return v.cache.GetNode(key)
}

// Hash returns the root hash of the verkle trie
func (v *VerkleAdapter) Hash() common.Hash {
	return v.trie.Hash()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
