// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"encoding/json"
)

// =============================================================================
// SEQUENCER TYPE: Abstract any sequencer (native, external, recursive)
// =============================================================================

// SequencerType identifies the type of sequencer
type SequencerType uint8

const (
	// SequencerNative is Lux native consensus
	SequencerNative SequencerType = 0

	// SequencerExternal is an external sequencer (OP Stack, Arbitrum, etc.)
	SequencerExternal SequencerType = 1

	// SequencerRecursive is a child network with parent finality
	SequencerRecursive SequencerType = 2
)

// SequencerIdentity identifies a specific sequencer within a network topology
type SequencerIdentity struct {
	// Type is the sequencer type
	Type SequencerType `json:"sequencer_type"`

	// ChainID is the unique chain identifier
	ChainID uint64 `json:"chain_id"`

	// Domain is the consensus domain
	Domain []byte `json:"domain"`

	// ParentChainID is for recursive networks (nil for root)
	ParentChainID *uint64 `json:"parent_chain_id,omitempty"`

	// ExternalRPC is for external sequencers
	ExternalRPC string `json:"external_rpc,omitempty"`

	// Depth is the recursion depth (0 = root)
	Depth uint32 `json:"depth"`
}

// NativeSequencer creates a native Lux sequencer identity
func NativeSequencer(chainID uint64, domain []byte) *SequencerIdentity {
	return &SequencerIdentity{
		Type:    SequencerNative,
		ChainID: chainID,
		Domain:  domain,
		Depth:   0,
	}
}

// ExternalSequencerIdentity creates an external sequencer identity
func ExternalSequencerIdentity(chainID uint64, domain []byte, rpc string) *SequencerIdentity {
	return &SequencerIdentity{
		Type:        SequencerExternal,
		ChainID:     chainID,
		Domain:      domain,
		ExternalRPC: rpc,
		Depth:       0,
	}
}

// RecursiveSequencerIdentity creates a recursive child sequencer identity
func RecursiveSequencerIdentity(chainID uint64, domain []byte, parentChainID uint64, depth uint32) *SequencerIdentity {
	return &SequencerIdentity{
		Type:          SequencerRecursive,
		ChainID:       chainID,
		Domain:        domain,
		ParentChainID: &parentChainID,
		Depth:         depth,
	}
}

// =============================================================================
// RECURSIVE NETWORK TOPOLOGY: Infinite fractal networks
// =============================================================================

// NetworkNode represents a node in the recursive network topology
type NetworkNode struct {
	// Identity identifies this sequencer
	Identity *SequencerIdentity `json:"identity"`

	// Config is the sequencer configuration
	Config SequencerConfig `json:"config"`

	// Children are child chains
	Children []*NetworkNode `json:"children,omitempty"`
}

// AddChild adds a child chain to this node
func (n *NetworkNode) AddChild(child *NetworkNode) {
	child.Identity.ParentChainID = &n.Identity.ChainID
	child.Identity.Depth = n.Identity.Depth + 1
	n.Children = append(n.Children, child)
}

// Traverse returns all nodes in the network (depth-first)
func (n *NetworkNode) Traverse() []*NetworkNode {
	result := []*NetworkNode{n}
	for _, child := range n.Children {
		result = append(result, child.Traverse()...)
	}
	return result
}

// FindByChainID finds a node by chain ID
func (n *NetworkNode) FindByChainID(chainID uint64) *NetworkNode {
	if n.Identity.ChainID == chainID {
		return n
	}
	for _, child := range n.Children {
		if found := child.FindByChainID(chainID); found != nil {
			return found
		}
	}
	return nil
}

// RecursiveNetwork represents a fractal recursive network topology
//
// Supports infinite depth of child chains, each with their own:
// - Sequencer configuration
// - Finality policy
// - Validator set
//
// Finality flows UP the tree:
// - Child chain finalizes locally
// - Parent chain includes child certificate
// - Root chain provides global finality
type RecursiveNetwork struct {
	Root *NetworkNode `json:"root"`
}

// GetAllChains returns all chains in the network
func (rn *RecursiveNetwork) GetAllChains() []*NetworkNode {
	return rn.Root.Traverse()
}

// GetChain returns a chain by ID
func (rn *RecursiveNetwork) GetChain(chainID uint64) *NetworkNode {
	return rn.Root.FindByChainID(chainID)
}

// GetFinalityPath returns the path from a chain to the root
func (rn *RecursiveNetwork) GetFinalityPath(chainID uint64) []*NetworkNode {
	var path []*NetworkNode
	node := rn.GetChain(chainID)
	for node != nil {
		path = append(path, node)
		if node.Identity.ParentChainID == nil {
			break
		}
		node = rn.GetChain(*node.Identity.ParentChainID)
	}
	return path
}

// AddChain adds a child chain under a parent
func (rn *RecursiveNetwork) AddChain(parentChainID uint64, child *NetworkNode) bool {
	parent := rn.GetChain(parentChainID)
	if parent == nil {
		return false
	}
	parent.AddChild(child)
	return true
}

// =============================================================================
// FACTORY FUNCTIONS
// =============================================================================

// NewSingleChainNetwork creates a simple single-chain network
func NewSingleChainNetwork(chainID uint64, domain []byte, config SequencerConfig) *RecursiveNetwork {
	return &RecursiveNetwork{
		Root: &NetworkNode{
			Identity: NativeSequencer(chainID, domain),
			Config:   config,
		},
	}
}

// NewAIMeshNetwork creates an AI agent mesh network
func NewAIMeshNetwork(chainID uint64, domain []byte, agentCount int) *RecursiveNetwork {
	return &RecursiveNetwork{
		Root: &NetworkNode{
			Identity: NativeSequencer(chainID, domain),
			Config:   AgentMeshConfig(domain, agentCount),
		},
	}
}

// L2Config holds L2 chain configuration
type L2Config struct {
	ChainID uint64
	Domain  []byte
	Config  SequencerConfig
}

// NewRecursiveRollupNetwork creates a recursive rollup network with multiple L2s
func NewRecursiveRollupNetwork(l1ChainID uint64, l1Domain []byte, l2Configs []L2Config) *RecursiveNetwork {
	root := &NetworkNode{
		Identity: NativeSequencer(l1ChainID, l1Domain),
		Config:   BlockchainConfig(l1Domain),
	}

	for _, l2 := range l2Configs {
		l2Node := &NetworkNode{
			Identity: RecursiveSequencerIdentity(l2.ChainID, l2.Domain, l1ChainID, 1),
			Config:   l2.Config,
		}
		root.AddChild(l2Node)
	}

	return &RecursiveNetwork{Root: root}
}

// =============================================================================
// JSON SERIALIZATION
// =============================================================================

// MarshalSequencerIdentity serializes a sequencer identity to JSON
func MarshalSequencerIdentity(s *SequencerIdentity) ([]byte, error) {
	return json.Marshal(s)
}

// UnmarshalSequencerIdentity deserializes a sequencer identity from JSON
func UnmarshalSequencerIdentity(data []byte) (*SequencerIdentity, error) {
	var s SequencerIdentity
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// MarshalRecursiveNetwork serializes a recursive network to JSON
func MarshalRecursiveNetwork(rn *RecursiveNetwork) ([]byte, error) {
	return json.Marshal(rn)
}

// UnmarshalRecursiveNetwork deserializes a recursive network from JSON
func UnmarshalRecursiveNetwork(data []byte) (*RecursiveNetwork, error) {
	var rn RecursiveNetwork
	if err := json.Unmarshal(data, &rn); err != nil {
		return nil, err
	}
	return &rn, nil
}
