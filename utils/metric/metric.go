// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metric

import (
	"errors"
	"sync"

	"github.com/luxfi/metrics"
)

// ErrMetricNotFound is returned when a metric is not found
var ErrMetricNotFound = errors.New("metric not found")

// Averager tracks a running average
type Averager interface {
	Observe(value float64)
	Read() float64
}

// averager implements an average tracker using internal state
type averager struct {
	mu    sync.RWMutex
	sum   float64
	count int64
}

// NewAverager returns a new Averager
func NewAverager() Averager {
	return &averager{}
}

// Observe adds a value to the average
func (a *averager) Observe(value float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sum += value
	a.count++
}

// Read returns the current average
func (a *averager) Read() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.count == 0 {
		return 0
	}
	return a.sum / float64(a.count)
}

// Counter tracks a count
type Counter interface {
	Inc()
	Add(delta int64)
	Read() int64
}

// counter wraps a luxfi/metrics Counter
type counter struct {
	ctr metrics.Counter
}

// NewCounter returns a new Counter
func NewCounter() Counter {
	m := metrics.New("consensus")
	return &counter{
		ctr: m.NewCounter("counter", "A counter metric"),
	}
}

// Inc increments the counter by 1
func (c *counter) Inc() {
	c.ctr.Inc()
}

// Add adds delta to the counter
func (c *counter) Add(delta int64) {
	c.ctr.Add(float64(delta))
}

// Read returns the current count
func (c *counter) Read() int64 {
	return int64(c.ctr.Get())
}

// Gauge tracks a value that can go up or down
type Gauge interface {
	Set(value float64)
	Add(delta float64)
	Read() float64
}

// gauge wraps a luxfi/metrics Gauge
type gauge struct {
	g metrics.Gauge
}

// NewGauge returns a new Gauge
func NewGauge() Gauge {
	m := metrics.New("consensus")
	return &gauge{
		g: m.NewGauge("gauge", "A gauge metric"),
	}
}

// Set sets the gauge to a specific value
func (g *gauge) Set(value float64) {
	g.g.Set(value)
}

// Add adds delta to the gauge
func (g *gauge) Add(delta float64) {
	g.g.Add(delta)
}

// Read returns the current value
func (g *gauge) Read() float64 {
	return g.g.Get()
}

// Registry is a collection of metrics
type Registry interface {
	NewCounter(name string) Counter
	NewGauge(name string) Gauge
	NewAverager(name string) Averager
	GetCounter(name string) (Counter, error)
	GetGauge(name string) (Gauge, error)
	GetAverager(name string) (Averager, error)
}

// registry wraps a luxfi/metrics instance and tracks averagers
type registry struct {
	metrics   metrics.Metrics
	averagers sync.Map // map[string]Averager
	counters  sync.Map // map[string]Counter
	gauges    sync.Map // map[string]Gauge
}

// NewRegistry returns a new Registry
func NewRegistry() Registry {
	return &registry{
		metrics: metrics.New("consensus"),
	}
}

// NewCounter creates and registers a new counter
func (r *registry) NewCounter(name string) Counter {
	c := &counter{
		ctr: r.metrics.NewCounter(name, "Counter: "+name),
	}
	r.counters.Store(name, c)
	return c
}

// NewGauge creates and registers a new gauge
func (r *registry) NewGauge(name string) Gauge {
	g := &gauge{
		g: r.metrics.NewGauge(name, "Gauge: "+name),
	}
	r.gauges.Store(name, g)
	return g
}

// NewAverager creates and registers a new averager
func (r *registry) NewAverager(name string) Averager {
	a := &averager{}
	r.averagers.Store(name, a)
	return a
}

// GetCounter returns a counter by name
func (r *registry) GetCounter(name string) (Counter, error) {
	if v, ok := r.counters.Load(name); ok {
		return v.(Counter), nil
	}
	return nil, ErrMetricNotFound
}

// GetGauge returns a gauge by name
func (r *registry) GetGauge(name string) (Gauge, error) {
	if v, ok := r.gauges.Load(name); ok {
		return v.(Gauge), nil
	}
	return nil, ErrMetricNotFound
}

// GetAverager returns an averager by name
func (r *registry) GetAverager(name string) (Averager, error) {
	if v, ok := r.averagers.Load(name); ok {
		return v.(Averager), nil
	}
	return nil, ErrMetricNotFound
}
