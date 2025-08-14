// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package telemetry provides metrics interfaces
package telemetry

type Counter interface{ Add(int64) }
type Gauge interface{ Set(int64) }

func C(name string) Counter { return noCounter{} }
func G(name string) Gauge   { return noGauge{} }

type noCounter struct{}

func (noCounter) Add(int64) {}

type noGauge struct{}

func (noGauge) Set(int64) {}