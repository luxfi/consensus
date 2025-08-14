// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
	"time"

	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// novaMetrics is a no-op implementation
// Real metrics should be handled by the parent system
type novaMetrics struct {
	lastAcceptedHeight uint64
	lastAcceptedTime   time.Time
	processing         map[ids.ID]time.Time
}

func newMetrics(
	log log.Logger,
	registerer interfaces.Registerer,
	lastAcceptedHeight uint64,
	lastAcceptedTime time.Time,
) (*novaMetrics, error) {
	return &novaMetrics{
		lastAcceptedHeight: lastAcceptedHeight,
		lastAcceptedTime:   lastAcceptedTime,
		processing:         make(map[ids.ID]time.Time),
	}, nil
}

func (m *novaMetrics) Verified(height uint64) {
	// no-op
}

func (m *novaMetrics) Issued(blkID ids.ID, pollNumber uint64) {
	m.processing[blkID] = time.Now()
}

func (m *novaMetrics) Accepted(blkID ids.ID, height uint64, timestamp time.Time, pollNumber uint64, size int) {
	delete(m.processing, blkID)
	m.lastAcceptedHeight = height
	m.lastAcceptedTime = timestamp
}

func (m *novaMetrics) Rejected(blkID ids.ID, pollNumber uint64, size int) {
	delete(m.processing, blkID)
}

func (m *novaMetrics) MeasureAndGetOldestDuration() time.Duration {
	var oldestTime time.Time
	for _, issuedTime := range m.processing {
		if oldestTime.IsZero() || issuedTime.Before(oldestTime) {
			oldestTime = issuedTime
		}
	}
	if oldestTime.IsZero() {
		return 0
	}
	return time.Since(oldestTime)
}

func (m *novaMetrics) SuccessfulPoll() {
	// no-op
}

func (m *novaMetrics) FailedPoll() {
	// no-op
}