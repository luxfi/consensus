// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"sync"
	"time"
	
	"github.com/luxfi/ids"
	"github.com/luxfi/crypto/corona"
)

// Config holds Quasar configuration
type Config struct {
	ChainID       ids.ID
	QPubKey       []byte
	QThreshold    int
	QuasarTimeout time.Duration
}

type quasar struct {
	cfg       Config
	selfPre   corona.Precomp
	pkGroup   []byte
	threshold int
	// fast lock-free buffer of shares per height
	shareBuf sync.Map // height uint64 -> []*corona.Share
}

// newQuasar creates a new Quasar instance
func newQuasar(sk []byte, cfg Config) (*quasar, error) {
	pre, err := corona.Precompute(sk)
	if err != nil {
		return nil, err
	}
	
	return &quasar{
		cfg:       cfg,
		selfPre:   pre,
		pkGroup:   cfg.QPubKey,
		threshold: cfg.QThreshold,
	}, nil
}

// sign is called by proposer thread right after BLS agg finished
func (q *quasar) sign(height uint64, blkHash []byte) (corona.Share, error) {
	share, err := corona.QuickSign(q.selfPre, blkHash)
	if err != nil {
		return nil, err
	}
	// gossip "RTSH|height|shareBytes"
	return share, nil
}

// onShare is called by mempool-gossip handler
func (q *quasar) onShare(height uint64, shareBytes []byte) (ready bool, cert []byte) {
	val, _ := q.shareBuf.LoadOrStore(height, &[]corona.Share{})
	ptr := val.(*[]corona.Share)
	*ptr = append(*ptr, corona.Share(shareBytes))

	// hot path: exit early until threshold reached
	if len(*ptr) < q.threshold {
		return false, nil
	}

	c, err := corona.Aggregate(*ptr)
	if err != nil {
		return false, nil
	}
	
	// Clean up
	q.shareBuf.Delete(height)
	
	return true, c
}

// verify checks a Quasar certificate
func (q *quasar) verify(msg []byte, cert []byte) bool {
	return corona.Verify(q.pkGroup, msg, cert)
}