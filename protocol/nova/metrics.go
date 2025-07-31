// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

type novaMetrics struct {
	processingBlocks    prometheus.Gauge
	acceptedBlocks      prometheus.Counter
	rejectedBlocks      prometheus.Counter
	prismsFinished       prometheus.Counter
	blockHeight         prometheus.Gauge
	lastAcceptedHeight  uint64
	lastAcceptedTime    time.Time
	processing          map[ids.ID]time.Time
	successfulPrisms     prometheus.Counter
	failedPrisms         prometheus.Counter
}

func newMetrics(
	log log.Logger,
	registerer prometheus.Registerer,
	lastAcceptedHeight uint64,
	lastAcceptedTime time.Time,
) (*novaMetrics, error) {
	m := &novaMetrics{
		processingBlocks: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nova_processing_blocks",
			Help: "Number of blocks currently processing",
		}),
		acceptedBlocks: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nova_accepted_blocks",
			Help: "Number of accepted blocks",
		}),
		rejectedBlocks: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nova_rejected_blocks",
			Help: "Number of rejected blocks",
		}),
		prismsFinished: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nova_prisms_finished",
			Help: "Number of prisms finished",
		}),
		blockHeight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nova_block_height",
			Help: "Current block height",
		}),
		successfulPrisms: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nova_successful_prisms",
			Help: "Number of successful prisms",
		}),
		failedPrisms: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nova_failed_prisms",
			Help: "Number of failed prisms",
		}),
		lastAcceptedHeight: lastAcceptedHeight,
		lastAcceptedTime:   lastAcceptedTime,
		processing:         make(map[ids.ID]time.Time),
	}

	m.blockHeight.Set(float64(lastAcceptedHeight))

	err := registerer.Register(m.processingBlocks)
	if err != nil {
		return nil, err
	}
	err = registerer.Register(m.acceptedBlocks)
	if err != nil {
		return nil, err
	}
	err = registerer.Register(m.rejectedBlocks)
	if err != nil {
		return nil, err
	}
	err = registerer.Register(m.prismsFinished)
	if err != nil {
		return nil, err
	}
	err = registerer.Register(m.blockHeight)
	if err != nil {
		return nil, err
	}
	err = registerer.Register(m.successfulPrisms)
	if err != nil {
		return nil, err
	}
	err = registerer.Register(m.failedPrisms)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *novaMetrics) Verified(height uint64) {
	// Track that a block was verified
}

func (m *novaMetrics) Issued(blkID ids.ID, pollNumber uint64) {
	m.processing[blkID] = time.Now()
	m.processingBlocks.Set(float64(len(m.processing)))
}

func (m *novaMetrics) Accepted(blkID ids.ID, height uint64, timestamp time.Time, pollNumber uint64, size int) {
	delete(m.processing, blkID)
	m.processingBlocks.Set(float64(len(m.processing)))
	m.acceptedBlocks.Inc()
	m.blockHeight.Set(float64(height))
	m.lastAcceptedHeight = height
	m.lastAcceptedTime = timestamp
}

func (m *novaMetrics) Rejected(blkID ids.ID, pollNumber uint64, size int) {
	delete(m.processing, blkID)
	m.processingBlocks.Set(float64(len(m.processing)))
	m.rejectedBlocks.Inc()
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
	m.successfulPrisms.Inc()
	m.prismsFinished.Inc()
}

func (m *novaMetrics) FailedPoll() {
	m.failedPrisms.Inc()
	m.prismsFinished.Inc()
}