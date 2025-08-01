package photon

import "time"

// Config represents photon protocol configuration
type Config struct {
    K                     int
    AlphaPreference       int
    AlphaConfidence       int
    Beta                  int
    ConcurrentPolls     int
    OptimalProcessing     int
    MaxOutstandingItems   int
    MaxItemProcessingTime time.Duration
}

// DefaultConfig returns default photon configuration
var DefaultConfig = Config{
    K:                     1,
    AlphaPreference:       1,
    AlphaConfidence:       1,
    Beta:                  1,
    ConcurrentPolls:     1,
    OptimalProcessing:     1,
    MaxOutstandingItems:   16,
    MaxItemProcessingTime: 10 * time.Second,
}
