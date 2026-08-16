// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// The recovery hint is this node's own applied head — a local guess about what the
// network finalized, not authority over it. A cert at the hint's height seeds even
// when it names a different canonical, because the alternative is stranding a node
// whose head sits on a branch the network did not finalize.
func TestSeedStandsAtTheHintWithADifferentCanonical(t *testing.T) {
	head, certified := ids.GenerateTestID(), ids.GenerateTestID()
	led := FinalityLedger{}.withHint(head, 1_000_000)

	next, plan, err := Finalize(led, Cert{Block: certified, Height: 1_000_000, Canonical: certified}, mapDAG{})
	if err != nil {
		t.Fatalf("a cert at the hint's own height must seed: %v", err)
	}
	if h, set := next.Height(); !set || h != 1_000_000 {
		t.Fatalf("seeded height = %d set=%v, want 1000000 set", h, set)
	}
	if len(plan.Accept) != 1 || plan.Accept[0] != certified {
		t.Fatalf("plan accepts %v, want exactly the certified block", plan.Accept)
	}
}
