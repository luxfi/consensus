package focus

import (
	"sync"
	"time"
)

// Tracker tracks confidence counters for items
type Tracker[ID comparable] struct {
	mu     sync.RWMutex
	counts map[ID]int
}

func NewTracker[ID comparable]() *Tracker[ID] {
	return &Tracker[ID]{
		counts: make(map[ID]int),
	}
}

func (t *Tracker[ID]) Incr(id ID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts[id]++
}

func (t *Tracker[ID]) Count(id ID) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.counts[id]
}

func (t *Tracker[ID]) Reset(id ID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.counts, id)
}

// Confidence tracks confidence building for consensus
type Confidence[ID comparable] struct {
	mu        sync.RWMutex
	threshold int
	alpha     float64
	states    map[ID]int
}

func NewConfidence[ID comparable](threshold int, alpha float64) *Confidence[ID] {
	return &Confidence[ID]{
		threshold: threshold,
		alpha:     alpha,
		states:    make(map[ID]int),
	}
}

func (c *Confidence[ID]) Update(id ID, ratio float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	current := c.states[id]
	if ratio >= c.alpha {
		c.states[id] = current + 1
	} else if ratio <= 1.0-c.alpha {
		c.states[id] = 0 // Reset on opposite preference
	}
}

func (c *Confidence[ID]) State(id ID) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	state := c.states[id]
	decided := state >= c.threshold
	return state, decided
}

// WindowedConfidence tracks confidence with time windows
type WindowedConfidence[ID comparable] struct {
	mu        sync.RWMutex
	threshold int
	alpha     float64
	window    time.Duration
	states    map[ID]int
	lastUpdate map[ID]time.Time
}

func NewWindowed[ID comparable](threshold int, alpha float64, window time.Duration) *WindowedConfidence[ID] {
	return &WindowedConfidence[ID]{
		threshold:  threshold,
		alpha:      alpha,
		window:     window,
		states:     make(map[ID]int),
		lastUpdate: make(map[ID]time.Time),
	}
}

func (w *WindowedConfidence[ID]) Update(id ID, ratio float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	now := time.Now()
	if last, ok := w.lastUpdate[id]; ok {
		if now.Sub(last) > w.window {
			// Window expired, reset
			w.states[id] = 0
		}
	}
	
	current := w.states[id]
	if ratio >= w.alpha {
		w.states[id] = current + 1
	} else if ratio <= 1.0-w.alpha {
		w.states[id] = 0
	}
	w.lastUpdate[id] = now
}

func (w *WindowedConfidence[ID]) State(id ID) (int, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	// Check if window expired
	if last, ok := w.lastUpdate[id]; ok {
		if time.Since(last) > w.window {
			return 0, false
		}
	}
	
	state := w.states[id]
	decided := state >= w.threshold
	return state, decided
}

// Calc calculates confidence based on votes
func Calc(yes, total, prev int) (float64, int) {
	if total == 0 {
		return 0, prev
	}
	
	ratio := float64(yes) / float64(total)
	
	// Calculate new confidence
	var conf int
	if ratio > 0.5 {
		conf = yes - (total - yes) // Difference between yes and no
		if conf < 0 {
			conf = 0
		}
	} else {
		conf = 0
	}
	
	// Consider previous confidence
	if prev > 0 && conf > 0 {
		conf = conf + prev/2 // Boost with previous confidence
	}
	
	return ratio, conf
}