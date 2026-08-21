// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// missing_ancestor_test.go — a parent this node does not hold and a cert that did
// not verify are different facts, and collapsing them into one error is what made
// the condition unrecoverable.
package chain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// failingBlock verifies with whatever error the test gives it. The live failure
// arrives from deep inside the outer-parent resolution as a bare
// database.ErrNotFound that names nothing, which is exactly why the error the
// operator sees has to name the parent itself.
type failingBlock struct {
	id       ids.ID
	parentID ids.ID
	height   uint64
	err      error
}

func (b *failingBlock) ID() ids.ID                   { return b.id }
func (b *failingBlock) Parent() ids.ID               { return b.parentID }
func (b *failingBlock) ParentID() ids.ID             { return b.parentID }
func (b *failingBlock) Height() uint64               { return b.height }
func (b *failingBlock) Timestamp() time.Time         { return time.Unix(0, 0) }
func (b *failingBlock) Status() uint8                { return 0 }
func (b *failingBlock) Bytes() []byte                { return []byte{1} }
func (b *failingBlock) Verify(context.Context) error { return b.err }
func (b *failingBlock) Accept(context.Context) error { return nil }
func (b *failingBlock) Reject(context.Context) error { return nil }

// TestMissingAncestorIsNotARejectedCert is the property the live wedge turned on.
//
// A validator holding byte-identical execution to its peers, but a different
// WRAPPER of the parent height, fails Verify on the outer-parent lookup with
// database.ErrNotFound. Reported as ErrCatchupCertRejected, the only remedy the
// requester has is to ask a different peer — the one move that cannot help, since
// every peer serves the same envelope. Named for what it is, the remedy is to ask
// for the parent, which any peer can serve.
func TestMissingAncestorIsNotARejectedCert(t *testing.T) {
	parent := ids.GenerateTestID()
	blk := &failingBlock{
		id:       ids.GenerateTestID(),
		parentID: parent,
		height:   51757,
		err:      database.ErrNotFound,
	}

	err := classifyVerifyFailure(blk, blk.err)

	if !errors.Is(err, ErrCatchupMissingAncestor) {
		t.Fatalf("a parent we do not hold must report as a missing ancestor, got %v", err)
	}
	if errors.Is(err, ErrCatchupCertRejected) {
		t.Fatal("a missing ancestor must NOT also read as a rejected cert — the two have " +
			"opposite remedies, and sharing one error value is what made this unrecoverable")
	}
	if !strings.Contains(err.Error(), parent.String()) {
		t.Fatalf("the error must NAME the parent to fetch; got %q", err.Error())
	}
}

// TestRealVerifyFailureStillRejects is the safety half. Only a missing ancestor
// gets the new name; anything else about a block remains a rejection, so this
// cannot be used to launder a bad block into a retry loop.
func TestRealVerifyFailureStillRejects(t *testing.T) {
	blk := &failingBlock{
		id:       ids.GenerateTestID(),
		parentID: ids.GenerateTestID(),
		height:   9,
		err:      errors.New("invalid proposer signature"),
	}

	err := classifyVerifyFailure(blk, blk.err)

	if !errors.Is(err, ErrCatchupCertRejected) {
		t.Fatalf("a genuinely bad block must still be rejected, got %v", err)
	}
	if errors.Is(err, ErrCatchupMissingAncestor) {
		t.Fatal("a bad block must not be reported as a fetchable ancestor")
	}
}

// TestWrappedNotFoundIsStillAMissingAncestor: the live error arrives wrapped
// through several layers (proposervm → ZAP → the inner VM), so the classifier
// must match on the sentinel rather than on the top-level value.
func TestWrappedNotFoundIsStillAMissingAncestor(t *testing.T) {
	blk := &failingBlock{
		id:       ids.GenerateTestID(),
		parentID: ids.GenerateTestID(),
		height:   4,
		err:      errors.Join(errors.New("get outer parent"), database.ErrNotFound),
	}
	if err := classifyVerifyFailure(blk, blk.err); !errors.Is(err, ErrCatchupMissingAncestor) {
		t.Fatalf("a wrapped not-found must still classify as a missing ancestor, got %v", err)
	}
}
