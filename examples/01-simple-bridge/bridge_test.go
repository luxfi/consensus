package main

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/luxfi/dex/pkg/lx"
)

func TestBridgeCreation(t *testing.T) {
	bridge := createBridge()

	if bridge == nil {
		t.Fatal("bridge creation failed")
	}

	if bridge.RequiredConfirmations != 6 {
		t.Errorf("expected 6 confirmations, got %d", bridge.RequiredConfirmations)
	}

	if bridge.ChallengePeriod != 5*time.Minute {
		t.Errorf("expected 5min challenge period, got %v", bridge.ChallengePeriod)
	}
}

func TestAssetSetup(t *testing.T) {
	bridge := createBridge()

	if err := setupAssets(bridge); err != nil {
		t.Fatalf("asset setup failed: %v", err)
	}

	// Verify ETH asset
	eth, exists := bridge.SupportedAssets["ETH"]
	if !exists {
		t.Fatal("ETH asset not found")
	}

	if eth.Symbol != "ETH" {
		t.Errorf("expected symbol ETH, got %s", eth.Symbol)
	}

	if eth.Decimals != 18 {
		t.Errorf("expected 18 decimals, got %d", eth.Decimals)
	}

	// Verify LUX asset
	lux, exists := bridge.SupportedAssets["LUX"]
	if !exists {
		t.Fatal("LUX asset not found")
	}

	if lux.Symbol != "LUX" {
		t.Errorf("expected symbol LUX, got %s", lux.Symbol)
	}
}

func TestTransferInitiation(t *testing.T) {
	bridge := createBridge()
	setupAssets(bridge)

	ctx := context.Background()
	transferID, err := initiateTransfer(ctx, bridge)

	if err != nil {
		t.Fatalf("transfer initiation failed: %v", err)
	}

	if transferID == "" {
		t.Fatal("transfer ID is empty")
	}

	// Verify transfer exists in pending
	transfer, exists := bridge.PendingTransfers[transferID]
	if !exists {
		t.Fatal("transfer not found in pending transfers")
	}

	if transfer.Asset != "ETH" {
		t.Errorf("expected asset ETH, got %s", transfer.Asset)
	}

	if transfer.SourceChain != "ethereum" {
		t.Errorf("expected source chain ethereum, got %s", transfer.SourceChain)
	}

	if transfer.DestChain != "lux" {
		t.Errorf("expected dest chain lux, got %s", transfer.DestChain)
	}

	expectedAmount := big.NewInt(1500000000000000000) // 1.5 ETH
	if transfer.Amount.Cmp(expectedAmount) != 0 {
		t.Errorf("expected amount %s, got %s", expectedAmount, transfer.Amount)
	}
}

func TestTransferCompletion(t *testing.T) {
	bridge := createBridge()
	setupAssets(bridge)

	ctx := context.Background()
	transferID, err := initiateTransfer(ctx, bridge)
	if err != nil {
		t.Fatalf("transfer initiation failed: %v", err)
	}

	// Wait for transfer to complete (max 10 seconds)
	err = monitorTransfer(ctx, bridge, transferID)
	if err != nil {
		t.Fatalf("transfer monitoring failed: %v", err)
	}

	// Verify transfer moved to completed
	transfer, exists := bridge.CompletedTransfers[transferID]
	if !exists {
		t.Fatal("transfer not found in completed transfers")
	}

	if transfer.Status != lx.BridgeStatusCompleted {
		t.Errorf("expected status Completed, got %v", transfer.Status)
	}

	if transfer.Confirmations != 6 {
		t.Errorf("expected 6 confirmations, got %d", transfer.Confirmations)
	}

	if transfer.SourceTxHash == "" {
		t.Error("source tx hash is empty")
	}

	if transfer.DestTxHash == "" {
		t.Error("dest tx hash is empty")
	}
}

func TestFormatAmount(t *testing.T) {
	tests := []struct {
		amount   *big.Int
		expected string
	}{
		{big.NewInt(1000000000000000000), "1.0000"}, // 1 ETH
		{big.NewInt(1500000000000000000), "1.5000"}, // 1.5 ETH
		{big.NewInt(10000000000000000), "0.0100"},   // 0.01 ETH
		{big.NewInt(123456789012345678), "0.1235"},  // 0.1235 ETH
	}

	for _, tt := range tests {
		result := formatAmount(tt.amount)
		if result != tt.expected {
			t.Errorf("formatAmount(%s) = %s, expected %s",
				tt.amount, result, tt.expected)
		}
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	time.Sleep(1 * time.Millisecond)
	id2 := generateID()

	if id1 == "" || id2 == "" {
		t.Error("generated ID is empty")
	}

	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
}

// Benchmark transfer initiation
func BenchmarkTransferInitiation(b *testing.B) {
	bridge := createBridge()
	setupAssets(bridge)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		initiateTransfer(ctx, bridge)
	}
}
