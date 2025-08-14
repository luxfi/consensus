package pulse

import "time"

// Config represents pulse protocol configuration
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

// DefaultConfig returns default pulse configuration
var DefaultConfig = Config{
    K:                     2,
    AlphaPreference:       2,
    AlphaConfidence:       2,
    Beta:                  1,
    ConcurrentPolls:     1,
    OptimalProcessing:     1,
    MaxOutstandingItems:   16,
    MaxItemProcessingTime: 10 * time.Second,
}
