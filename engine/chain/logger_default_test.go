// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// countingLogger is a live logger: it records that it was written to, and says
// so via IsZero, which is what the call sites read before they log. Everything
// it does not count, it delegates to the silent logger.
type countingLogger struct {
	log.Logger
	n atomic.Int64
}

func newCountingLogger() *countingLogger { return &countingLogger{Logger: log.Noop()} }

func (c *countingLogger) Debug(string, ...interface{}) { c.n.Add(1) }
func (c *countingLogger) IsZero() bool                 { return false }
func (c *countingLogger) count() int64                 { return c.n.Load() }

// refusingVM parses nothing. It stands in for the VM's answer to a malformed
// block, which is the shape that carries HandleIncomingBlock to its logger.
type refusingVM struct{}

var errRefused = errors.New("refused")

func (refusingVM) BuildBlock(context.Context) (block.Block, error) {
	return nil, errRefused
}
func (refusingVM) GetBlock(context.Context, ids.ID) (block.Block, error) {
	return nil, errRefused
}
func (refusingVM) ParseBlock(context.Context, []byte) (block.Block, error) {
	return nil, errRefused
}
func (refusingVM) LastAccepted(context.Context) (ids.ID, error) { return ids.Empty, nil }
func (refusingVM) SetPreference(context.Context, ids.ID) error  { return nil }

// A Runtime built with no Logger must survive every remote-input path.
//
// NewRuntime answers an unset Logger with log.Noop(), so the logger a handler
// reaches for is always present. The three handlers below read it on their
// first line, before any validation, which is why the answer belongs in the
// constructor rather than at the 24 sites that read the logger without asking
// whether it is there: forgetting one of those turns a malformed packet into a
// crash, and there is no way to notice the omission until it happens.
//
// Reverting the default in NewRuntime makes this test crash, not fail.
func TestZeroLoggerRuntimeSurvivesMalformedRemoteInput(t *testing.T) {
	rt := NewRuntime(NetworkConfig{VM: refusingVM{}})

	garbage := [][]byte{
		nil,
		{},
		{0x00},
		make([]byte, ids.NodeIDLen),          // truncated: no length prefix
		make([]byte, ids.NodeIDLen+4),        // length prefix present, zero-length signature
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // short, high bytes
		make([]byte, 4096),                   // long, all zero
	}

	ctx := context.Background()
	for _, b := range garbage {
		// Every remote-input path, on every shape. No panic is the assertion.
		rt.HandleIncomingVote(ids.Empty, b)
		rt.HandleIncomingCert(b)
		_, _ = rt.HandleIncomingBlock(ctx, b, ids.EmptyNodeID)
	}
}

// The default holds whatever else the caller supplies or omits, since a caller
// that omits the logger tends to omit more than the logger.
func TestNewRuntimeAlwaysLeavesALogger(t *testing.T) {
	k := config.DefaultParams()
	for name, cfg := range map[string]NetworkConfig{
		"empty":      {},
		"vm only":    {VM: refusingVM{}},
		"params":     {Params: &k},
		"chain id":   {ChainID: ids.GenerateTestID()},
		"explicitly": {Logger: nil},
	} {
		if got := NewRuntime(cfg).config.Logger; got == nil {
			t.Errorf("%s: Logger is nil", name)
		} else if !got.IsZero() {
			t.Errorf("%s: default logger should be the silent one, got a live logger", name)
		}
	}
}

// A logger the caller DID supply is the one the Runtime keeps. The default
// fills an absence; it does not overwrite a choice.
func TestNewRuntimeKeepsTheSuppliedLogger(t *testing.T) {
	supplied := newCountingLogger()
	rt := NewRuntime(NetworkConfig{Logger: supplied})

	if rt.config.Logger != supplied {
		t.Fatal("NewRuntime replaced a supplied logger")
	}

	// And it is reached: a malformed vote is a logged event, not a silent one.
	rt.HandleIncomingVote(ids.Empty, []byte{0x00})
	if supplied.count() == 0 {
		t.Error("supplied logger was never written to on the malformed-vote path")
	}
}
