package config

import (
    "testing"
    "time"
)

func TestDefaultParams(t *testing.T) {
    p := DefaultParams()
    
    if p.K != 20 {
        t.Errorf("expected K=20, got %d", p.K)
    }
    if p.Alpha != 0.8 {
        t.Errorf("expected Alpha=0.8, got %f", p.Alpha)
    }
    if p.Beta != 15 {
        t.Errorf("expected Beta=15, got %d", p.Beta)
    }
    if p.RoundTO != 250*time.Millisecond {
        t.Errorf("expected RoundTO=250ms, got %v", p.RoundTO)
    }
    if p.BlockTime != 100*time.Millisecond {
        t.Errorf("expected BlockTime=100ms, got %v", p.BlockTime)
    }
}

func TestXChainParams(t *testing.T) {
    p := XChainParams()
    
    // Test ultra-fast 1ms blocks for 100Gbps networks
    if p.BlockTime != 1*time.Millisecond {
        t.Errorf("X-Chain should support 1ms blocks, got %v", p.BlockTime)
    }
    if p.RoundTO != 5*time.Millisecond {
        t.Errorf("X-Chain round timeout should be 5ms, got %v", p.RoundTO)
    }
    if p.K != 5 {
        t.Errorf("X-Chain should have K=5 for low latency, got %d", p.K)
    }
    if p.Alpha != 0.6 {
        t.Errorf("X-Chain should have Alpha=0.6, got %f", p.Alpha)
    }
    if p.Beta != 3 {
        t.Errorf("X-Chain should have Beta=3, got %d", p.Beta)
    }
}

func TestMainnetParams(t *testing.T) {
    p := MainnetParams()
    
    if p.K != 21 {
        t.Errorf("Mainnet should have 21 validators, got %d", p.K)
    }
    if p.Alpha != 0.8 {
        t.Errorf("Mainnet Alpha should be 0.8, got %f", p.Alpha)
    }
    if p.Beta != 15 {
        t.Errorf("Mainnet Beta should be 15, got %d", p.Beta)
    }
    if p.BlockTime != 200*time.Millisecond {
        t.Errorf("Mainnet BlockTime should be 200ms, got %v", p.BlockTime)
    }
}

func TestTestnetParams(t *testing.T) {
    p := TestnetParams()
    
    if p.K != 11 {
        t.Errorf("Testnet should have 11 validators, got %d", p.K)
    }
    if p.Alpha != 0.7 {
        t.Errorf("Testnet Alpha should be 0.7, got %f", p.Alpha)
    }
    if p.Beta != 6 {
        t.Errorf("Testnet Beta should be 6, got %d", p.Beta)
    }
}

func TestLocalParams(t *testing.T) {
    p := LocalParams()
    
    if p.K != 5 {
        t.Errorf("Local should have 5 validators, got %d", p.K)
    }
    if p.BlockTime != 10*time.Millisecond {
        t.Errorf("Local BlockTime should be 10ms, got %v", p.BlockTime)
    }
}

func TestWithBlockTime(t *testing.T) {
    tests := []struct {
        name      string
        blockTime time.Duration
        wantRound time.Duration
    }{
        {"1ms ultra-fast", 1 * time.Millisecond, 5 * time.Millisecond},
        {"5ms very fast", 5 * time.Millisecond, 25 * time.Millisecond},
        {"10ms fast", 10 * time.Millisecond, 250 * time.Millisecond}, // uses original
        {"100ms normal", 100 * time.Millisecond, 250 * time.Millisecond},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            p := DefaultParams().WithBlockTime(tt.blockTime)
            if p.BlockTime != tt.blockTime {
                t.Errorf("BlockTime not set: got %v, want %v", p.BlockTime, tt.blockTime)
            }
            if tt.blockTime < 10*time.Millisecond && p.RoundTO != tt.wantRound {
                t.Errorf("RoundTO not adjusted: got %v, want %v", p.RoundTO, tt.wantRound)
            }
        })
    }
}

func TestValidate(t *testing.T) {
    tests := []struct {
        name    string
        params  Parameters
        wantErr error
    }{
        {
            name:    "valid params",
            params:  DefaultParams(),
            wantErr: nil,
        },
        {
            name: "invalid K",
            params: Parameters{
                K:         0,
                Alpha:     0.8,
                Beta:      15,
                RoundTO:   250 * time.Millisecond,
                BlockTime: 100 * time.Millisecond,
            },
            wantErr: ErrInvalidK,
        },
        {
            name: "invalid Alpha too low",
            params: Parameters{
                K:         20,
                Alpha:     0.3,
                Beta:      15,
                RoundTO:   250 * time.Millisecond,
                BlockTime: 100 * time.Millisecond,
            },
            wantErr: ErrInvalidAlpha,
        },
        {
            name: "invalid Alpha too high",
            params: Parameters{
                K:         20,
                Alpha:     1.5,
                Beta:      15,
                RoundTO:   250 * time.Millisecond,
                BlockTime: 100 * time.Millisecond,
            },
            wantErr: ErrInvalidAlpha,
        },
        {
            name: "invalid Beta",
            params: Parameters{
                K:         20,
                Alpha:     0.8,
                Beta:      0,
                RoundTO:   250 * time.Millisecond,
                BlockTime: 100 * time.Millisecond,
            },
            wantErr: ErrInvalidBeta,
        },
        {
            name: "block time too low",
            params: Parameters{
                K:         20,
                Alpha:     0.8,
                Beta:      15,
                RoundTO:   250 * time.Millisecond,
                BlockTime: 500 * time.Microsecond, // < 1ms
            },
            wantErr: ErrBlockTimeTooLow,
        },
        {
            name: "round timeout too low",
            params: Parameters{
                K:         20,
                Alpha:     0.8,
                Beta:      15,
                RoundTO:   50 * time.Millisecond,
                BlockTime: 100 * time.Millisecond,
            },
            wantErr: ErrRoundTimeoutTooLow,
        },
        {
            name: "1ms blocks are valid",
            params: Parameters{
                K:         5,
                Alpha:     0.6,
                Beta:      3,
                RoundTO:   5 * time.Millisecond,
                BlockTime: 1 * time.Millisecond,
            },
            wantErr: nil,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.params.Validate()
            if err != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}

func BenchmarkValidate(b *testing.B) {
    p := DefaultParams()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = p.Validate()
    }
}

func BenchmarkWithBlockTime(b *testing.B) {
    p := DefaultParams()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = p.WithBlockTime(1 * time.Millisecond)
    }
}