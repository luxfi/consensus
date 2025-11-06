# Example 01: Simple Bridge Integration

This example demonstrates basic integration with the Lux DEX cross-chain bridge.

## What It Shows

- Creating a cross-chain bridge connection
- Adding supported chains
- Initiating transfers between chains
- Monitoring transfer status

## Run the Example

```bash
cd examples/01-simple-bridge
go run main.go
```

## Run the Tests

```bash
go test -v
```

## Expected Output

```
=== Bridge Setup ===
✓ Created bridge with 2 chains
✓ Exchange rate ETH → LUX: 1500

=== Transfer Test ===
→ Initiating transfer: 1.5 ETH from Ethereum to Lux
✓ Transfer initiated: tx_abc123...
✓ Waiting for confirmations...
✓ Transfer completed in 3.2s

=== Verification ===
✓ Transfer verified
✓ Amount: 1.5 ETH = 2250 LUX
✓ Status: Completed
```

## Key Files

- `main.go` - Interactive example demonstrating bridge usage
- `bridge_test.go` - Comprehensive tests you can run
- `go.mod` - Dependencies (DEX bridge package)

## Play With It

Try modifying:
- Transfer amounts
- Add more chains
- Change exchange rates
- Test error scenarios

## Integration Pattern

This shows the basic pattern for using the production DEX bridge:

```go
// 1. Create bridge
bridge := lx.NewCrossChainBridge(config)

// 2. Add supported chains
bridge.AddAsset(asset)

// 3. Initiate transfer
transferID, err := bridge.InitiateTransfer(ctx, transfer)

// 4. Monitor status
transfer, err := bridge.GetTransfer(transferID)
```
