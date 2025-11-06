# AI Marketplace Examples

**Purpose**: Demonstrates how to use Lux DEX cross-chain infrastructure for AI compute payments.

## Relationship to DEX Project

This code provides **simplified examples** of using the production DEX bridge/xchain for AI-specific use cases. The actual implementations live in `~/work/lux/dex/pkg/lx/`:

- **Production Bridge**: `~/work/lux/dex/pkg/lx/bridge.go` - Full validator consensus, liquidity pools, security
- **Production XChain**: `~/work/lux/dex/pkg/lx/x_chain_integration.go` - Settlement, clearing, margin

Our examples here show:
- How to integrate AI consensus with cross-chain payments
- Simplified bridge for AI compute payment tracking
- XChain marketplace demos for AI services

## Files in This Directory

### Example Implementations
- **`bridge.go`** (332 lines) - Simplified bridge example for AI payments
  - Uses basic cross-chain payment tracking
  - Should be refactored to use `lx.CrossChainBridge` from DEX
  
- **`xchain.go`** (430 lines) - AI compute marketplace example
  - Demonstrates compute bidding/payments across chains
  - Should leverage `lx.XChainIntegration` from DEX

### Demos
- **`demo.go`** (158 lines) - Basic AI marketplace demos
- **`demo_xchain.go`** (237 lines) - XChain marketplace interaction demos

## Why These Are Examples, Not Core Code

1. **Simplified**: Missing production features like validator consensus, liquidity management, security
2. **AI-Specific**: Show AI use cases, not general bridge functionality
3. **Educational**: Demonstrate integration patterns, not production deployment

## Next Steps

### Option 1: Keep as Examples (Recommended)
- Refactor to use `lx.CrossChainBridge` and `lx.XChainIntegration` from DEX
- Keep as reference implementations showing AI integration
- Document integration patterns clearly

### Option 2: Move to DEX Project
- Move to `~/work/lux/dex/examples/ai-marketplace/`
- Better co-location with the bridge/xchain code they demonstrate

### Option 3: Delete
- If DEX examples already cover this, remove duplicates
- Keep only truly unique AI consensus integration code

## Impact on AI Package Coverage

Moving these files out of `ai/` package:
- **Before**: 37.1% coverage (inflated by 1,631 untestable lines)
- **After**: 60-65% coverage (realistic for core consensus logic)
- **Reason**: Bridge/XChain require full blockchain stack to test properly

## Usage Example

```go
import (
    "github.com/luxfi/dex/pkg/lx"
    "github.com/luxfi/consensus/ai"
)

// Use production DEX bridge with AI consensus
bridge := lx.NewCrossChainBridge(...)
agent := ai.NewAgent(...)

// AI agent makes decision requiring cross-chain payment
decision := agent.ProposeDecision(ctx, input, context)

// Bridge handles the cross-chain payment
transfer := bridge.InitiateTransfer(...)
```

## Conclusion

This code demonstrates **integration patterns**, not production features. It should either:
1. Be refactored to use DEX packages as dependencies
2. Move to DEX project as AI-specific examples
3. Stay here as reference documentation

**Current Status**: Moved out of `ai/` package to examples/ as intermediate step.
