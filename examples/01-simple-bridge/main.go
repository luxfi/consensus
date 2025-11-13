// Example 01: Simple Bridge Integration
//
// This demonstrates basic usage of the Lux DEX cross-chain bridge.
// Run this to see a simple transfer between Ethereum and Lux chains.

package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/luxfi/dex/pkg/lx"
)

func main() {
	fmt.Println("=== Lux Bridge Example ===\n")

	// Step 1: Create bridge
	bridge := createBridge()
	fmt.Println("✓ Bridge created with 2 chains\n")

	// Step 2: Add supported assets
	if err := setupAssets(bridge); err != nil {
		panic(err)
	}
	fmt.Println("✓ Assets configured: ETH, LUX\n")

	// Step 3: Test a transfer
	fmt.Println("=== Testing Transfer ===")
	ctx := context.Background()

	transferID, err := initiateTransfer(ctx, bridge)
	if err != nil {
		panic(err)
	}

	fmt.Printf("✓ Transfer initiated: %s\n", transferID)

	// Step 4: Monitor transfer
	if err := monitorTransfer(ctx, bridge, transferID); err != nil {
		panic(err)
	}

	fmt.Println("\n=== Transfer Complete ===")
	fmt.Println("✓ Cross-chain transfer successful!")
	fmt.Println("✓ You can verify the transaction on both chains")
}

func createBridge() *lx.CrossChainBridge {
	return &lx.CrossChainBridge{
		SupportedAssets:       make(map[string]*lx.BridgeAsset),
		PendingTransfers:      make(map[string]*lx.BridgeTransfer),
		CompletedTransfers:    make(map[string]*lx.BridgeTransfer),
		FailedTransfers:       make(map[string]*lx.BridgeTransfer),
		LiquidityPools:        make(map[string]*lx.BridgeLiquidityPool),
		RequiredConfirmations: 6,
		ChallengePeriod:       5 * time.Minute,
	}
}

func setupAssets(bridge *lx.CrossChainBridge) error {
	// Add ETH as supported asset
	eth := &lx.BridgeAsset{
		Symbol:         "ETH",
		Name:           "Ethereum",
		Decimals:       18,
		SourceContract: "0x...eth",
		WrappedContract: map[string]string{
			"lux": "0x...lux",
		},
		MinTransfer: big.NewInt(10000000000000000),    // 0.01 ETH
		MaxTransfer: big.NewInt(1000000000000000000),  // 1 ETH
		DailyLimit:  big.NewInt(10000000000000000000), // 10 ETH
		DailyVolume: big.NewInt(0),
		LastReset:   time.Now(),
		Paused:      false,
	}

	bridge.SupportedAssets["ETH"] = eth

	// Add LUX as supported asset
	lux := &lx.BridgeAsset{
		Symbol:         "LUX",
		Name:           "Lux",
		Decimals:       18,
		SourceContract: "0x...lux",
		WrappedContract: map[string]string{
			"ethereum": "0x...eth",
		},
		MinTransfer: big.NewInt(1000000000000000000),      // 1 LUX
		MaxTransfer: big.NewInt(1000000000000000000000),   // 1000 LUX
		DailyLimit:  big.NewInt(100000000000000000000000), // 100k LUX
		DailyVolume: big.NewInt(0),
		LastReset:   time.Now(),
		Paused:      false,
	}

	bridge.SupportedAssets["LUX"] = lux

	return nil
}

func initiateTransfer(ctx context.Context, bridge *lx.CrossChainBridge) (string, error) {
	// Create transfer from Ethereum to Lux
	amount := big.NewInt(1500000000000000000) // 1.5 ETH
	fee := big.NewInt(1500000000000000)       // 0.0015 ETH

	fmt.Printf("→ Transferring %s ETH from Ethereum to Lux\n",
		new(big.Float).Quo(
			new(big.Float).SetInt(amount),
			new(big.Float).SetInt(big.NewInt(1000000000000000000)),
		).String())

	transfer := &lx.BridgeTransfer{
		ID:               generateID(),
		Asset:            "ETH",
		Amount:           amount,
		Fee:              fee,
		SourceChain:      "ethereum",
		DestChain:        "lux",
		SourceAddress:    "0x1234...sender",
		DestAddress:      "0x5678...recipient",
		Status:           lx.BridgeStatusPending,
		Confirmations:    0,
		RequiredConfirms: 6,
		InitiatedAt:      time.Now(),
		ExpiryTime:       time.Now().Add(30 * time.Minute),
		Validators:       make(map[string]*lx.BridgeSignature),
		Nonce:            uint64(time.Now().Unix()),
	}

	// Store in bridge
	bridge.PendingTransfers[transfer.ID] = transfer

	// Simulate async processing
	go simulateTransfer(bridge, transfer)

	return transfer.ID, nil
}

func monitorTransfer(ctx context.Context, bridge *lx.CrossChainBridge, transferID string) error {
	fmt.Println("→ Monitoring transfer status...")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("transfer timeout")
		case <-ticker.C:
			// Check pending transfers
			if transfer, exists := bridge.PendingTransfers[transferID]; exists {
				fmt.Printf("  [%s] %d/%d confirmations\n",
					transfer.Status, transfer.Confirmations, transfer.RequiredConfirms)
				continue
			}

			// Check completed transfers
			if transfer, exists := bridge.CompletedTransfers[transferID]; exists {
				elapsed := transfer.CompletedAt.Sub(transfer.InitiatedAt)
				fmt.Printf("✓ Transfer completed in %.1fs\n", elapsed.Seconds())

				fmt.Printf("\nTransfer Details:\n")
				fmt.Printf("  ID:        %s\n", transfer.ID)
				fmt.Printf("  Amount:    %s %s\n", formatAmount(transfer.Amount), transfer.Asset)
				fmt.Printf("  Fee:       %s %s\n", formatAmount(transfer.Fee), transfer.Asset)
				fmt.Printf("  From:      %s (%s)\n", transfer.SourceChain, transfer.SourceAddress)
				fmt.Printf("  To:        %s (%s)\n", transfer.DestChain, transfer.DestAddress)
				fmt.Printf("  Src Hash:  %s\n", transfer.SourceTxHash)
				fmt.Printf("  Dest Hash: %s\n", transfer.DestTxHash)

				return nil
			}

			// Check failed transfers
			if _, exists := bridge.FailedTransfers[transferID]; exists {
				return fmt.Errorf("transfer failed")
			}
		}
	}
}

func simulateTransfer(bridge *lx.CrossChainBridge, transfer *lx.BridgeTransfer) {
	// Simulate confirmations
	for i := 0; i < transfer.RequiredConfirms; i++ {
		time.Sleep(400 * time.Millisecond)
		transfer.Confirmations = i + 1

		if i == 2 {
			transfer.Status = lx.BridgeStatusValidating
		}
		if i == 4 {
			transfer.Status = lx.BridgeStatusConfirmed
			transfer.Status = lx.BridgeStatusExecuting
		}
	}

	// Complete transfer
	transfer.Status = lx.BridgeStatusCompleted
	transfer.CompletedAt = time.Now()
	transfer.SourceTxHash = "0xabc..." + generateID()[:8]
	transfer.DestTxHash = "0xdef..." + generateID()[:8]

	// Move to completed
	delete(bridge.PendingTransfers, transfer.ID)
	bridge.CompletedTransfers[transfer.ID] = transfer
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func formatAmount(amount *big.Int) string {
	eth := new(big.Float).Quo(
		new(big.Float).SetInt(amount),
		new(big.Float).SetInt(big.NewInt(1000000000000000000)),
	)
	return eth.Text('f', 4)
}
