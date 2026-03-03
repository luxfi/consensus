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
	// 69% threshold update
	if p.Alpha != 0.69 {
		t.Errorf("expected Alpha=0.69 (69%% threshold), got %f", p.Alpha)
	}
	if p.Beta != 14 {
		t.Errorf("expected Beta=14 (adjusted for 69%%), got %d", p.Beta)
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
	if p.K != 3 {
		t.Errorf("X-Chain should have K=3, got %d", p.K)
	}
	// 2/3 threshold
	if p.Alpha != 0.67 {
		t.Errorf("X-Chain should have Alpha=0.67 (2/3 threshold), got %f", p.Alpha)
	}
	if p.Beta != 2 {
		t.Errorf("X-Chain should have Beta=2 (adjusted for 2/3), got %d", p.Beta)
	}
}

func TestMainnetParams(t *testing.T) {
	p := MainnetParams()

	if p.K != 21 {
		t.Errorf("Mainnet should have 21 validators, got %d", p.K)
	}
	// 69% threshold update
	if p.Alpha != 0.69 {
		t.Errorf("Mainnet Alpha should be 0.69 (69%% threshold), got %f", p.Alpha)
	}
	if p.Beta != 14 {
		t.Errorf("Mainnet Beta should be 14 (adjusted for 69%%), got %d", p.Beta)
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
	// 69% threshold update
	if p.Alpha != 0.69 {
		t.Errorf("Testnet Alpha should be 0.69 (69%% threshold), got %f", p.Alpha)
	}
	if p.Beta != 8 {
		t.Errorf("Testnet Beta should be 8 (adjusted for 69%%), got %d", p.Beta)
	}
}

func TestLocalParams(t *testing.T) {
	p := LocalParams()

	if p.K != 3 {
		t.Errorf("Local should have 3 validators, got %d", p.K)
	}
	if p.BlockTime != 1*time.Millisecond {
		t.Errorf("Local BlockTime should be 1ms, got %v", p.BlockTime)
	}
	if p.RoundTO != 5*time.Millisecond {
		t.Errorf("Local RoundTO should be 5ms, got %v", p.RoundTO)
	}
}

func TestSoloGPUParams(t *testing.T) {
	p := SoloGPUParams()

	if p.K != 1 {
		t.Errorf("SoloGPU K should be 1, got %d", p.K)
	}
	if p.BlockTime != 1*time.Millisecond {
		t.Errorf("SoloGPU BlockTime should be 1ms, got %v", p.BlockTime)
	}
	if p.GasLimit != 1_000_000_000 {
		t.Errorf("SoloGPU GasLimit should be 1B, got %d", p.GasLimit)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("SoloGPU params should be valid, got %v", err)
	}

	// 1B gas / 21000 gas = 47,619 txs/block × 1000 blocks/sec = 47.6M TPS ceiling
	txsPerBlock := p.GasLimit / 21000
	if txsPerBlock != 47619 {
		t.Errorf("Expected 47619 txs/block, got %d", txsPerBlock)
	}
}

func TestBurstParams(t *testing.T) {
	p := BurstParams()

	if p.BlockTime != 1*time.Millisecond {
		t.Errorf("Burst BlockTime should be 1ms, got %v", p.BlockTime)
	}
	if p.RoundTO != 5*time.Millisecond {
		t.Errorf("Burst RoundTO should be 5ms, got %v", p.RoundTO)
	}
	if p.GasLimit != 2_100_000_000 {
		t.Errorf("Burst GasLimit should be 2.1B, got %d", p.GasLimit)
	}
	if p.MaxOutstandingItems != 8192 {
		t.Errorf("Burst MaxOutstandingItems should be 8192, got %d", p.MaxOutstandingItems)
	}
	if p.ConcurrentPolls != 8 {
		t.Errorf("Burst ConcurrentPolls should be 8, got %d", p.ConcurrentPolls)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Burst params should be valid, got %v", err)
	}

	// Verify: 2.1B gas / 21000 gas per tx = 100K txs/block
	// 100K txs × 1000 blocks/sec = 100M TPS
	txsPerBlock := p.GasLimit / 21000
	tps := txsPerBlock * 1000 // 1ms blocks = 1000 blocks/sec
	if tps != 100_000_000 {
		t.Errorf("Burst TPS should be 100M, got %d", tps)
	}
}

func TestWithBlockTime(t *testing.T) {
	tests := []struct {
		name      string
		blockTime time.Duration
		wantRound time.Duration
	}{
		{"1ms ultra-fast", 1 * time.Millisecond, 5 * time.Millisecond},
		{"5ms very fast", 5 * time.Millisecond, 15 * time.Millisecond},
		{"10ms fast", 10 * time.Millisecond, 26 * time.Millisecond},
		{"100ms normal", 100 * time.Millisecond, 251 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultParams().WithBlockTime(tt.blockTime)
			if p.BlockTime != tt.blockTime {
				t.Errorf("BlockTime not set: got %v, want %v", p.BlockTime, tt.blockTime)
			}
			if p.RoundTO != tt.wantRound {
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
				Alpha:     0.69, // 69% threshold
				Beta:      14,   // Adjusted for 69%
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
				Alpha:     0.69, // 69% threshold
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
				Alpha:     0.69, // 69% threshold
				Beta:      14,   // Adjusted for 69%
				RoundTO:   250 * time.Millisecond,
				BlockTime: 500 * time.Microsecond, // < 1ms
			},
			wantErr: ErrBlockTimeTooLow,
		},
		{
			name: "round timeout too low",
			params: Parameters{
				K:         20,
				Alpha:     0.69, // 69% threshold
				Beta:      14,   // Adjusted for 69%
				RoundTO:   50 * time.Millisecond,
				BlockTime: 100 * time.Millisecond,
			},
			wantErr: ErrRoundTimeoutTooLow,
		},
		{
			name: "1ms blocks are valid",
			params: Parameters{
				K:         5,
				Alpha:     0.69, // 69% threshold
				Beta:      4,    // Adjusted for 69%
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

func TestValidateForNetwork(t *testing.T) {
	tests := []struct {
		name      string
		params    Parameters
		networkID uint32
		wantErr   error
	}{
		// Mainnet (networkID=1): K >= 11 required
		{
			name:      "mainnet params on mainnet",
			params:    MainnetParams(),
			networkID: 1,
			wantErr:   nil,
		},
		{
			name:      "testnet params on mainnet",
			params:    TestnetParams(),
			networkID: 1,
			wantErr:   nil, // K=11, exactly at minimum
		},
		{
			name:      "burst params on mainnet rejected",
			params:    BurstParams(),
			networkID: 1,
			wantErr:   ErrKTooLowForMainnet,
		},
		{
			name:      "solo GPU params on mainnet rejected",
			params:    SoloGPUParams(),
			networkID: 1,
			wantErr:   ErrKTooLowForMainnet,
		},
		{
			name:      "local params on mainnet rejected",
			params:    LocalParams(),
			networkID: 1,
			wantErr:   ErrKTooLowForMainnet,
		},

		// Testnet (networkID=5): K >= 5 required
		{
			name:      "testnet params on testnet",
			params:    TestnetParams(),
			networkID: 5,
			wantErr:   nil,
		},
		{
			name:      "burst params on testnet rejected",
			params:    BurstParams(),
			networkID: 5,
			wantErr:   ErrKTooLowForTestnet,
		},
		{
			name:      "solo GPU params on testnet rejected",
			params:    SoloGPUParams(),
			networkID: 5,
			wantErr:   ErrKTooLowForTestnet,
		},

		// Local/devnet (networkID >= 1337): any K allowed
		{
			name:      "burst params on local",
			params:    BurstParams(),
			networkID: 1337,
			wantErr:   nil,
		},
		{
			name:      "solo GPU params on local",
			params:    SoloGPUParams(),
			networkID: 1337,
			wantErr:   nil,
		},
		{
			name:      "local params on devnet",
			params:    LocalParams(),
			networkID: 9999,
			wantErr:   nil,
		},

		// Basic validation still runs first
		{
			name: "invalid params fail before network check",
			params: Parameters{
				K:     0,
				Alpha: 0.69,
				Beta:  14,
			},
			networkID: 1337,
			wantErr:   ErrInvalidK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.ValidateForNetwork(tt.networkID)
			if err != tt.wantErr {
				t.Errorf("ValidateForNetwork(%d) error = %v, wantErr %v", tt.networkID, err, tt.wantErr)
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
