// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metric

import (
	"fmt"
	"sync"
)

// Averager tracks a running average
type Averager interface {
	Observe(value float64)
	Read() float64
}

// averager implements Averager
type averager struct {
	mu    sync.RWMutex
	sum   float64
	count float64
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
	return a.sum / a.count
}

// Counter tracks a count
type Counter interface {
	Inc()
	Add(delta int64)
	Read() int64
}

// counter implements Counter
type counter struct {
	mu    sync.RWMutex
	value int64
}

// NewCounter returns a new Counter
func NewCounter() Counter {
	return &counter{}
}

// Inc increments the counter by 1
func (c *counter) Inc() {
	c.Add(1)
}

// Add adds delta to the counter
func (c *counter) Add(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += delta
}

// Read returns the current count
func (c *counter) Read() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// Gauge tracks a value that can go up or down
type Gauge interface {
	Set(value float64)
	Add(delta float64)
	Read() float64
}

// gauge implements Gauge  
type gauge struct {
	mu    sync.RWMutex
	value float64
}

// NewGauge returns a new Gauge
func NewGauge() Gauge {
	return &gauge{}
}

// Set sets the gauge to a specific value
func (g *gauge) Set(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = value
}

// Add adds delta to the gauge
func (g *gauge) Add(delta float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value += delta
}

// Read returns the current value
func (g *gauge) Read() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.value
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

// registry implements Registry
type registry struct {
	mu        sync.RWMutex
	counters  map[string]Counter
	gauges    map[string]Gauge
	averagers map[string]Averager
}

// NewRegistry returns a new Registry
func NewRegistry() Registry {
	return &registry{
		counters:  make(map[string]Counter),
		gauges:    make(map[string]Gauge),
		averagers: make(map[string]Averager),
	}
}

// NewCounter creates and registers a new counter
func (r *registry) NewCounter(name string) Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	c := NewCounter()
	r.counters[name] = c
	return c
}

// NewGauge creates and registers a new gauge
func (r *registry) NewGauge(name string) Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	g := NewGauge()
	r.gauges[name] = g
	return g
}

// NewAverager creates and registers a new averager
func (r *registry) NewAverager(name string) Averager {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Create a simple averager without prometheus registration
	a := &averager{}
	r.averagers[name] = a
	return a
}

// GetCounter returns a counter by name
func (r *registry) GetCounter(name string) (Counter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	c, ok := r.counters[name]
	if !ok {
		return nil, fmt.Errorf("counter %q not found", name)
	}
	return c, nil
}

// GetGauge returns a gauge by name
func (r *registry) GetGauge(name string) (Gauge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	g, ok := r.gauges[name]
	if !ok {
		return nil, fmt.Errorf("gauge %q not found", name)
	}
	return g, nil
}

// GetAverager returns an averager by name
func (r *registry) GetAverager(name string) (Averager, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	a, ok := r.averagers[name]
	if !ok {
		return nil, fmt.Errorf("averager %q not found", name)
	}
	return a, nil
}