// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_mainnet_committee_test.go — the 2026-07-09 mainnet C-Chain finality-freeze
// regression: view-change must size its BFT committee from the LIVE validator set, not the
// oversized Snowman SAMPLE preset.
//
// EXACT mainnet condition reproduced: MainnetParams presets the Snowman sample at K=21/α=15
// (sized for a ≥21-validator network), but only 5 validators are live. The harness leaves
// cfg.Validators nil, so the construction-time bftCommittee clamp is skipped and consensus.K()
// stays 21 / Alpha() stays 15 — identical to the mainnet node whose current-map validator Manager
// read 0 at construction (the same empty set that gives the proposervm windower pChainHeight=0 /
// anyone-can-propose). The round-scoped view-change POL and precommit count DISTINCT validators, so
// α=15 affirmative prevotes are UNREACHABLE from 5 nodes: no POL ever forms → finality FREEZES
// (all agree on the tip, no fork — the live incident signature).
//
// The fix (integration.go viewCommittee): size the view-change committee from the SAME populated
// ⅔-by-stake StakeSource the cert reads — n = ValidatorCount (5), α = bftAlpha(5) = 4 — so a POL
// forms at 4 prevotes → precommit → one ⅔-by-stake cert → finalize. Snowman K=21 is left untouched
// (the chain still boots past the mainnet K≥11 floor); only the round machine's committee shrinks.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// mainnetStormParams5VC is the storm-tuned 5-node harness base with the REAL MainnetParams Snowman
// committee (K=21/α=15) and the round-scoped view-change ON — the exact live-mainnet config on a
// 5-validator set.
func mainnetStormParams5VC() config.Parameters {
	p := stormParams5()
	p.K = 21               // MainnetParams Snowman SAMPLE size (sized for ≥21 validators)
	p.AlphaPreference = 15 // MainnetParams α — 15 DISTINCT prevotes, unreachable from 5 nodes
	p.AlphaConfidence = 15
	p.ViewChange = true
	return p
}

func TestViewChange_MainnetParams_5Validators_CommitteeFromStakeSource(t *testing.T) {
	// A symmetric sibling storm at height 1: ALL five validators each build a distinct competing
	// block on the same parent (the anyone-can-propose condition). The deterministic lowest-canonical
	// winner is the view-change target every honest node prevotes; whether the height finalizes then
	// turns ENTIRELY on the POL threshold α — which is the committee-sizing bug under test.
	buildStorm := func(net *simNet) map[ids.ID]*simBlock {
		blocks := make(map[ids.ID]*simBlock, 5)
		for i := 0; i < 5; i++ {
			blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "mainnet-sib-"+itoa(i))
			blocks[blk.ID()] = blk
			net.build(i, blk)
		}
		return blocks
	}

	// CONTROL — the mainnet freeze reproduced. ValidatorCount unresolved (0), as on the node whose
	// current-map Manager read 0 at construction: viewCommittee cannot shrink the Snowman preset, so
	// the round machine runs at α=15. Five validators can cast at most five distinct prevotes, so no
	// POL(winner) can ever form → NOTHING finalizes. Safety holds (no fork) — this is a pure liveness
	// freeze, exactly the live incident.
	t.Run("frozen_when_validator_count_unresolved", func(t *testing.T) {
		net := newSimNet(t, 5, mainnetStormParams5VC())
		net.vs.setSimulatedValidatorCount(0)
		buildStorm(net)

		finalizedSomething := waitFor(3*time.Second, func() bool {
			return len(net.headsAtHeight(1)) > 0
		})
		if finalizedSomething {
			t.Fatalf("CONTROL: with the view-change committee stuck at α=15 and only 5 validators, "+
				"no POL can form — height 1 MUST freeze, got finalized heads=%v", net.headsAtHeight(1))
		}
	})

	// FIX — committee sized to the live validator set. Default ValidatorCount == len(ids) == 5, so
	// viewCommittee shrinks 21→5 and α = bftAlpha(5) = 4. The storm's lowest-canonical winner now
	// gathers ≥4 prevotes → POL → precommit → one ⅔-by-stake cert → finalize. Must converge ALL five
	// live nodes on a SINGLE head (no double-finalization).
	t.Run("finalizes_when_committee_sized_to_live_set", func(t *testing.T) {
		net := newSimNet(t, 5, mainnetStormParams5VC())
		blocks := buildStorm(net)

		deadline := time.Now().Add(stormTO)
		for time.Now().Before(deadline) {
			heads := net.headsAtHeight(1)
			if len(heads) > 1 {
				t.Fatalf("FIX: DOUBLE-FINALIZATION at height 1 (safety break): %v", heads)
			}
			if len(heads) == 1 {
				for id, c := range heads {
					if c >= net.upCount() {
						if _, ok := blocks[id]; !ok {
							t.Fatalf("FIX: finalized head %s is not one of the built siblings", id)
						}
						return // converged: one head, finalized by every live node
					}
				}
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Fatalf("FIX: view-change committee sized from the StakeSource (n=5, α=4) must converge the "+
			"sibling storm to one finalized head across all 5 nodes (heads=%v, up=%d)",
			net.headsAtHeight(1), net.upCount())
	})
}
