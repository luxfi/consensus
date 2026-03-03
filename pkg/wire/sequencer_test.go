// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"encoding/json"
	"testing"
)

func TestNativeSequencer(t *testing.T) {
	domain := []byte("test-chain")
	s := NativeSequencer(42, domain)

	if s.Type != SequencerNative {
		t.Errorf("expected SequencerNative, got %d", s.Type)
	}
	if s.ChainID != 42 {
		t.Errorf("expected chain ID 42, got %d", s.ChainID)
	}
	if string(s.Domain) != "test-chain" {
		t.Errorf("domain mismatch")
	}
	if s.Depth != 0 {
		t.Errorf("native sequencer should have depth 0")
	}
	if s.ParentChainID != nil {
		t.Errorf("native sequencer should have nil parent")
	}
	if s.ExternalRPC != "" {
		t.Errorf("native sequencer should have empty RPC")
	}
}

func TestExternalSequencerIdentity(t *testing.T) {
	s := ExternalSequencerIdentity(10, []byte("op-stack"), "https://rpc.example.com")

	if s.Type != SequencerExternal {
		t.Errorf("expected SequencerExternal, got %d", s.Type)
	}
	if s.ChainID != 10 {
		t.Errorf("expected chain ID 10, got %d", s.ChainID)
	}
	if s.ExternalRPC != "https://rpc.example.com" {
		t.Errorf("RPC mismatch: %s", s.ExternalRPC)
	}
	if s.Depth != 0 {
		t.Errorf("external sequencer should have depth 0")
	}
}

func TestRecursiveSequencerIdentity(t *testing.T) {
	s := RecursiveSequencerIdentity(20, []byte("l2"), 1, 1)

	if s.Type != SequencerRecursive {
		t.Errorf("expected SequencerRecursive, got %d", s.Type)
	}
	if s.ChainID != 20 {
		t.Errorf("expected chain ID 20, got %d", s.ChainID)
	}
	if s.ParentChainID == nil || *s.ParentChainID != 1 {
		t.Errorf("expected parent chain ID 1")
	}
	if s.Depth != 1 {
		t.Errorf("expected depth 1, got %d", s.Depth)
	}
}

func TestNetworkNodeAddChildAndTraverse(t *testing.T) {
	root := &NetworkNode{
		Identity: NativeSequencer(1, []byte("root")),
		Config:   BlockchainConfig([]byte("root")),
	}
	child1 := &NetworkNode{
		Identity: NativeSequencer(2, []byte("child1")),
		Config:   SingleNodeConfig([]byte("child1")),
	}
	child2 := &NetworkNode{
		Identity: NativeSequencer(3, []byte("child2")),
		Config:   SingleNodeConfig([]byte("child2")),
	}

	root.AddChild(child1)
	root.AddChild(child2)

	// child1 should have parent set to root's chain ID
	if child1.Identity.ParentChainID == nil || *child1.Identity.ParentChainID != 1 {
		t.Error("child1 parent should be 1")
	}
	if child1.Identity.Depth != 1 {
		t.Errorf("child1 depth should be 1, got %d", child1.Identity.Depth)
	}

	// Traverse should return all 3 nodes
	all := root.Traverse()
	if len(all) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(all))
	}
}

func TestNetworkNodeFindByChainID(t *testing.T) {
	root := &NetworkNode{
		Identity: NativeSequencer(1, []byte("root")),
		Config:   BlockchainConfig([]byte("root")),
	}
	child := &NetworkNode{
		Identity: NativeSequencer(2, []byte("child")),
		Config:   SingleNodeConfig([]byte("child")),
	}
	grandchild := &NetworkNode{
		Identity: NativeSequencer(3, []byte("grandchild")),
		Config:   SingleNodeConfig([]byte("grandchild")),
	}
	root.AddChild(child)
	child.AddChild(grandchild)

	// Find root
	found := root.FindByChainID(1)
	if found == nil || found.Identity.ChainID != 1 {
		t.Error("should find root")
	}

	// Find grandchild
	found = root.FindByChainID(3)
	if found == nil || found.Identity.ChainID != 3 {
		t.Error("should find grandchild")
	}

	// Not found
	found = root.FindByChainID(999)
	if found != nil {
		t.Error("should not find non-existent chain")
	}
}

func TestRecursiveNetwork(t *testing.T) {
	rn := NewSingleChainNetwork(1, []byte("main"), BlockchainConfig([]byte("main")))

	if rn.Root == nil {
		t.Fatal("root should not be nil")
	}
	if rn.Root.Identity.ChainID != 1 {
		t.Errorf("root chain ID should be 1")
	}

	// GetAllChains
	chains := rn.GetAllChains()
	if len(chains) != 1 {
		t.Errorf("expected 1 chain, got %d", len(chains))
	}

	// GetChain
	chain := rn.GetChain(1)
	if chain == nil {
		t.Error("should find chain 1")
	}
	chain = rn.GetChain(999)
	if chain != nil {
		t.Error("should not find chain 999")
	}

	// AddChain
	child := &NetworkNode{
		Identity: NativeSequencer(2, []byte("l2")),
		Config:   SingleNodeConfig([]byte("l2")),
	}
	ok := rn.AddChain(1, child)
	if !ok {
		t.Error("should add chain under existing parent")
	}
	if len(rn.GetAllChains()) != 2 {
		t.Errorf("expected 2 chains after add")
	}

	// AddChain to non-existent parent
	ok = rn.AddChain(999, &NetworkNode{
		Identity: NativeSequencer(3, []byte("orphan")),
		Config:   SingleNodeConfig([]byte("orphan")),
	})
	if ok {
		t.Error("should not add chain under non-existent parent")
	}
}

func TestGetFinalityPath(t *testing.T) {
	rn := NewSingleChainNetwork(1, []byte("root"), BlockchainConfig([]byte("root")))
	child := &NetworkNode{
		Identity: NativeSequencer(2, []byte("l2")),
		Config:   SingleNodeConfig([]byte("l2")),
	}
	grandchild := &NetworkNode{
		Identity: NativeSequencer(3, []byte("l3")),
		Config:   SingleNodeConfig([]byte("l3")),
	}
	rn.AddChain(1, child)
	rn.AddChain(2, grandchild)

	path := rn.GetFinalityPath(3)
	if len(path) != 3 {
		t.Errorf("expected finality path of length 3, got %d", len(path))
	}
	if path[0].Identity.ChainID != 3 {
		t.Error("path should start at chain 3")
	}
	if path[2].Identity.ChainID != 1 {
		t.Error("path should end at root chain 1")
	}

	// Path from root
	path = rn.GetFinalityPath(1)
	if len(path) != 1 {
		t.Errorf("root path should be length 1, got %d", len(path))
	}

	// Path for non-existent chain
	path = rn.GetFinalityPath(999)
	if len(path) != 0 {
		t.Errorf("non-existent chain should have empty path, got %d", len(path))
	}
}

func TestNewAIMeshNetwork(t *testing.T) {
	rn := NewAIMeshNetwork(42, []byte("mesh"), 5)
	if rn.Root == nil {
		t.Fatal("root should not be nil")
	}
	if rn.Root.Config.K != 5 {
		t.Errorf("expected K=5, got %d", rn.Root.Config.K)
	}
	if rn.Root.Config.SoftPolicy != PolicyQuorum {
		t.Error("AI mesh should use PolicyQuorum")
	}
}

func TestNewRecursiveRollupNetwork(t *testing.T) {
	l2Configs := []L2Config{
		{ChainID: 10, Domain: []byte("l2a"), Config: RollupConfig([]byte("l2a"))},
		{ChainID: 20, Domain: []byte("l2b"), Config: RollupConfig([]byte("l2b"))},
	}
	rn := NewRecursiveRollupNetwork(1, []byte("l1"), l2Configs)

	all := rn.GetAllChains()
	if len(all) != 3 {
		t.Errorf("expected 3 chains (1 L1 + 2 L2), got %d", len(all))
	}

	l2a := rn.GetChain(10)
	if l2a == nil {
		t.Fatal("should find L2 chain 10")
	}
	if l2a.Identity.ParentChainID == nil || *l2a.Identity.ParentChainID != 1 {
		t.Error("L2 should have parent chain ID 1")
	}
	if l2a.Identity.Depth != 1 {
		t.Errorf("L2 depth should be 1, got %d", l2a.Identity.Depth)
	}
}

func TestSequencerIdentitySerialization(t *testing.T) {
	s := RecursiveSequencerIdentity(42, []byte("test"), 1, 3)

	data, err := MarshalSequencerIdentity(s)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	s2, err := UnmarshalSequencerIdentity(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if s2.Type != s.Type {
		t.Error("type mismatch")
	}
	if s2.ChainID != s.ChainID {
		t.Error("chain ID mismatch")
	}
	if s2.Depth != s.Depth {
		t.Error("depth mismatch")
	}
	if s2.ParentChainID == nil || *s2.ParentChainID != *s.ParentChainID {
		t.Error("parent chain ID mismatch")
	}

	// Invalid JSON
	_, err = UnmarshalSequencerIdentity([]byte("invalid"))
	if err == nil {
		t.Error("should fail on invalid JSON")
	}
}

func TestRecursiveNetworkSerialization(t *testing.T) {
	rn := NewRecursiveRollupNetwork(1, []byte("l1"), []L2Config{
		{ChainID: 10, Domain: []byte("l2"), Config: RollupConfig([]byte("l2"))},
	})

	data, err := MarshalRecursiveNetwork(rn)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	rn2, err := UnmarshalRecursiveNetwork(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(rn2.GetAllChains()) != 2 {
		t.Error("expected 2 chains after roundtrip")
	}

	// Invalid JSON
	_, err = UnmarshalRecursiveNetwork([]byte("{bad"))
	if err == nil {
		t.Error("should fail on invalid JSON")
	}
}

func TestSequencerIdentityJSONFields(t *testing.T) {
	s := ExternalSequencerIdentity(10, []byte("test"), "https://rpc.example.com")
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}

	// Verify JSON field names
	for _, field := range []string{"sequencer_type", "chain_id", "domain", "external_rpc", "depth"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing JSON field: %s", field)
		}
	}
}
