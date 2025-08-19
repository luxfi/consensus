// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package benchlist

import "time"

// Config defines benchlist configuration parameters
type Config struct {
	// Threshold for benchlisting
	Threshold int `json:"threshold"`
	// Duration to benchlist a node
	Duration time.Duration `json:"duration"`
	// MinimumFailingDuration is the minimum amount of time a peer must be
	// failing before we will consider benchlisting them
	MinimumFailingDuration time.Duration `json:"minimumFailingDuration"`
	// MaxPortion is the maximum portion of validators that can be benchlisted
	MaxPortion float64 `json:"maxPortion"`
	// Validators is the validator manager
	Validators interface{} `json:"-"`
	// Benchable is the object that can be benched
	Benchable interface{} `json:"-"`
	// BenchlistRegisterer is the metrics registerer
	BenchlistRegisterer interface{} `json:"-"`
}