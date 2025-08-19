package config

import "time"

type Parameters struct {
    K         int
    Alpha     float64
    Beta      uint32
    RoundTO   time.Duration
    BlockTime time.Duration
}

func DefaultParams() Parameters {
    return Parameters{
        K:         20,
        Alpha:     0.8,
        Beta:      15,
        RoundTO:   250 * time.Millisecond,
        BlockTime: 100 * time.Millisecond,
    }
}
