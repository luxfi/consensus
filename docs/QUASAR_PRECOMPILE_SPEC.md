# Lux Quasar Precompile Specification

**Address**: `0x000000000000000000000000000000000000001F`

This precompile provides on-chain quantum-secure finality verification for the Lux Network.

---

## 1. State Layout (EVM Storage)

| Slot | Key (keccak) | Value | Notes |
|------|--------------|-------|-------|
| 0x00 | Q_LAST_HEIGHT | uint256 – latest block height covered by a valid Ringtail certificate | monotonic |
| 0x01 | Q_LAST_ROOT | bytes32 – Merkle root of Beam/Nova state sealed at Q_LAST_HEIGHT | |
| 0x02 | Q_LAST_CERT_HASH | bytes32 – keccak256(rtCert) | |
| 0x03 | Q_THRESHOLD_T | uint256 – Ringtail threshold t (e.g. 15) | constant after genesis |
| 0x04 | Q_PUBLIC_KEY_HASH | bytes32 – hash of Ringtail group public key | constant |
| 0x05 | Q_BLOCKS_PER_QUANTUM | uint256 – policy parameter N (e.g. 128) | upgradable via governance |

No per-block map is stored; only the latest seal is kept on-chain.
Historic proofs are re-verifiable off-chain from block headers if needed.

---

## 2. External ABI (4 static-call helpers, 1 mutating function)

```solidity
pragma solidity ^0.8.24;

interface IQuasar {
    /// @return height  Beam/C-Chain height covered by last PQ seal
    /// @return root    Merkle root sealed
    function getCurrentQBlockRef()
        external view
        returns (uint256 height, bytes32 root);

    /// @notice fast yes/no check
    function isBlockQuantumFinalized(uint256 height)
        external view
        returns (bool);

    /// @return delta = block.number - Q_LAST_HEIGHT
    function getBlocksSinceQuantumFinality()
        external view
        returns (uint256 delta);

    /// @notice Anyone may call; reverts if cert invalid
    /// @param height      Beam height sealed
    /// @param root        Merkle root of Beam+Nova state
    /// @param rtCert      Ringtail threshold certificate blob
    function verifyQuantumCert(
        uint256 height,
        bytes32 root,
        bytes calldata rtCert
    ) external returns (bool ok);
}
```

### Function Selectors

| Function | Selector |
|----------|----------|
| getCurrentQBlockRef() | 0xe2ddd8fb |
| isBlockQuantumFinalized(uint256) | 0x41d2a666 |
| getBlocksSinceQuantumFinality() | 0xb4a3576e |
| verifyQuantumCert(uint256,bytes32,bytes) | 0x1b6732c5 |

---

## 3. Execution Flow – verifyQuantumCert

```
caller passes (height, root, rtCert)
│
├── 1.  Sanity checks
│     ├─ require(height > Q_LAST_HEIGHT)
│     ├─ require(height-Q_LAST_HEIGHT <= Q_BLOCKS_PER_QUANTUM*2)
│     └─ msg.sender pays gas, but no auth needed
│
├── 2.  Parse rtCert header
│     ├─ version      : uint8
│     ├─ quorumBitmap : bytes[ceil(n/8)]
│     ├─ sigBytes     : remaining
│
├── 3.  Re-create message digest:
│        M = keccak256(height || root)
│
├── 4.  Verify lattice signature
│     ├─ load group public key (constant) from slot 4
│     ├─ call pre-linked Ringtail C-lib (or BN128 FFI) :
│          ok = ringtail_verify(pubkey, rtCert, M, t)
│     └─ require(ok == true)
│
├── 5.  Update state
│     ├─ SSTORE slot0 = height
│     ├─ SSTORE slot1 = root
│     ├─ SSTORE slot2 = keccak256(rtCert)
│
└── 6.  Emit events
       QuantumSealed(height, root, keccak256(rtCert))
```

Ringtail verification is written in native Go (inside Subnet-EVM) and linked into the precompile just like ecrecover is in geth.
Cost ≈ ~200k gas + 10 gas/KB for the signature blob (bounded, ~3 KB).

---

## 4. Gas Schedule (guideline)

| Opcode / Path | Gas |
|--------------|-----|
| STATICCALL to get* view fns | 700 (cheap, pure SLOAD) |
| verifyQuantumCert base | 20,000 |
| Ringtail lattice verify | 170,000–230,000 (depends on n,t) |
| SSTORE × 3 | 3 × 21,000 = 63,000 (EIP-2929 adjusted) |
| **Total** | ≈ 260,000–320,000 |

At ~25 M gas/blk a mainnet block can fit 80–100 certs, but policy will call it once per quantum (e.g. every 128 blocks), so head-room is ample.

---

## 5. Fast-path Helpers (pure reads)

### 5.1 getCurrentQBlockRef()
Zero-gas cost for contracts when executed off-chain via eth_call.

### 5.2 isBlockQuantumFinalized(h)
```solidity
return h <= Q_LAST_HEIGHT;
```

### 5.3 getBlocksSinceQuantumFinality()
```solidity
return block.number - Q_LAST_HEIGHT;
```

---

## 6. Integration into Block Validity

Consensus engine (Snowman++) rule:

```go
func VerifyHeader(header *BlockHeader) error {
    // 1. Fast BLS check
    if !verifyBLS(header.BLSAgg) { return ErrInvalidCert }

    // 2. Quasar precompile says block is PQ-finalized?
    if !quasar.IsBlockQuantumFinalized(header.Height) {
        return ErrMissingQuantumCert
    }
    return nil
}
```

If `verifyQuantumCert` hasn't been called yet for the height range, proposers must include that transaction in the next block; otherwise further blocks will fail rule 2 and be rejected.

---

## 7. Performance Notes

- **Parallelism** – Ringtail verification is native code executed inside the precompile, not by Solidity; runs in < 3 ms on a single core, well below block times.
- **Storage footprint** – only the latest seal is kept on-chain. Full history lives in block headers for auditors who want deeper proofs.
- **Reorg safety** – because Seal₂ must reference a strictly higher height, you cannot replay or override an older seal without chain reorganisation > MaxLag (forbidden by consensus).

---

## 8. Implementation Notes

The precompile is implemented in Go and integrated into Subnet-EVM at build time. The Ringtail verification uses the `github.com/daryakaviani/ringtail` library v0.3.2.

### 8.1 Precompile Registration
```go
// In vm/evm/precompile/contracts.go
QuasarPrecompile = common.HexToAddress("0x000000000000000000000000000000000000001F")

// Register in init()
func init() {
    PrecompiledContractsBerlin[QuasarPrecompile] = &quasar{}
    PrecompiledContractsCancun[QuasarPrecompile] = &quasar{}
}
```

### 8.2 Storage Keys
```go
var (
    slotLastHeight    = common.Hash{0x00}
    slotLastRoot      = common.Hash{0x01}
    slotLastCertHash  = common.Hash{0x02}
    slotThreshold     = common.Hash{0x03}
    slotPubKeyHash    = common.Hash{0x04}
    slotBlocksPerQ    = common.Hash{0x05}
)
```

---

## 9. Recap

- **BLS + Ringtail dual-cert** is enforced at consensus.
- The precompile exposes tiny helper calls for contracts/bridges and one privileged `verifyQuantumCert` function that any node may invoke to post a fresh PQ seal.
- Overhead is a few hundred k gas every ~10 min (mainnet cadence), negligible latency, and a strongly layered security model: no block advances unless both classical and post-quantum proofs are in place.