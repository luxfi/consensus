# CI/CD Workflows

## SDK Verification Matrix

**File**: `sdk-verification.yml`

### Purpose

This workflow ensures ALL SDKs are tested against their **actual capabilities** (not marketing claims). After the 2025-11-10 audit revealed that most SDKs are stubs/wrappers, this CI prevents regressions and keeps documentation honest.

### What It Tests

#### 1. Python SDK ✅ Production Ready
- **Tests**: All 64 comprehensive tests
- **Verifies**: Actual consensus behavior (not just APIs)
- **Benchmarks**: Real workload (1.6M votes/sec verified)
- **Matrix**: Python 3.10, 3.11, 3.12

**Why**: Python is the ONLY SDK with complete Chain, DAG, and PQ implementations.

#### 2. Go SDK ⚠️ Partial (BFT Only)
- **Tests**: All Go tests (400+ tests)
- **Verifies**:
  - ❌ Chain is still stub (<100 LOC)
  - ❌ DAG is still stub (<100 LOC)
  - ❌ PQ still uses mock certificates
  - ✅ BFT consensus works
- **Matrix**: Go 1.22, 1.23

**Why**: Go "reference implementation" has stub engines that need to be tracked.

#### 3. C SDK ⚠️ Data Structures Only
- **Tests**: 57/58 tests
- **Verifies**:
  - All 3 types share same implementation (no real consensus)
  - Performance measures data structures, not consensus
- **Benchmarks**: 21K votes/sec (real measurement)

**Why**: C SDK is fast but doesn't implement consensus algorithms.

#### 4. Rust SDK ⚠️ FFI Wrapper
- **Tests**: 19/19 FFI tests
- **Verifies**:
  - Rust calls C library via FFI
  - Inherits all C SDK limitations
  - Benchmarks checked for impossible claims (>10M/sec)
- **Benchmarks**: Validates FFI overhead measured correctly

**Why**: Rust wraps C, so can't be faster than C (21K/sec). Any benchmark >100K/sec is measuring wrong thing.

#### 5. C++ SDK ⚠️ Snowball Only
- **Tests**: 2/3 tests (stubs expected to fail)
- **Verifies**:
  - Only Snowball implemented
  - Chain/DAG/PQ return nullptr (stubs)

**Why**: C++ has one working algorithm, rest are TODOs.

### Special Checks

#### Documentation Validation
- Checks for misleading claims ("native implementations")
- Verifies audit reports exist
- Ensures LLM.md documents SDK reality

#### Benchmark Sanity
- Detects u8 overflow bug in Rust (caps at 255 instead of 10K)
- Warns about impossible FFI performance claims
- Validates benchmarks measure real workload (not stubs)

#### Summary Report
Prints SDK status table showing:
- ✅ Production ready components
- ⚠️ Partial/stub implementations
- 📄 Audit documentation

### Running Locally

```bash
# Test individual SDKs
cd pkg/python && pytest test_consensus_comprehensive.py -v
cd pkg/rust && cargo test
cd pkg/c && make test
go test ./...

# Run benchmarks
cd pkg/python && python benchmark_cpu_standalone.py
cd pkg/rust && cargo bench
cd pkg/c && ./build/benchmark
go test -bench=. ./engine/...

# Validate stubs are still stubs (should return true)
[ $(wc -l < engine/chain/engine.go) -lt 100 ] && echo "Chain is stub"
[ $(wc -l < engine/dag/engine.go) -lt 100 ] && echo "DAG is stub"
grep -q "mock-bls-aggregate" engine/pq/consensus.go && echo "PQ uses mocks"
```

### What This Prevents

1. **False Claims**: CI fails if docs claim features that don't exist
2. **Benchmark Lies**: Detects impossible performance numbers
3. **Regressions**: Tracks when stubs become real implementations
4. **Inconsistency**: Ensures tests match actual SDK capabilities

### When CI Should Change

#### ✅ Go Chain Implemented
When `engine/chain/engine.go` exceeds 100 LOC with real consensus:
- Remove stub verification check
- Add consensus behavior tests
- Update summary to show Chain as production ready

#### ✅ Go DAG Implemented
When `engine/dag/engine.go` exceeds 100 LOC and stops returning nil:
- Remove stub verification check
- Add vertex processing tests
- Update summary to show DAG as production ready

#### ✅ Go PQ Gets Real Certificates
When `engine/pq/consensus.go` removes mock certificates:
- Remove mock certificate check
- Add ML-DSA/Kyber integration tests
- Update summary to show PQ as production ready

#### ✅ C SDK Implements Consensus
When C SDK differentiates Chain/DAG/PQ with real algorithms:
- Add consensus behavior tests
- Verify correct algorithm per type
- Update status from "data structures" to "consensus"

#### ✅ Rust Implements Native Consensus
When Rust stops using C FFI and implements natively:
- Remove FFI verification check
- Add native implementation tests
- Benchmarks can exceed C SDK performance

#### ✅ C++ Implements Chain/DAG/PQ
When C++ adds real implementations beyond Snowball:
- Remove nullptr checks
- Add algorithm-specific tests
- Update status table

### Why This Matters

The 2025-11-10 audit revealed:
- **99% of benchmarks were fake** (bugs or measuring stubs)
- **Go "reference" has stub engines** (69-83 lines of nil returns)
- **Only Python SDK is complete** (all 3 consensus types work)
- **Documentation was misleading** (claimed features that don't exist)

This CI ensures:
- ✅ Tests verify behavior, not just APIs
- ✅ Benchmarks measure real workload
- ✅ Documentation matches reality
- ✅ Future claims are backed by CI proof

### Related Documentation

- `ALL_SDK_REALITY_CHECK.md` - Complete audit summary
- `GO_SDK_VERIFICATION_REPORT.md` - Go engine analysis
- `pkg/c/TEST_RESULTS.md` - C SDK findings
- `pkg/rust/TEST_REPORT.md` - Rust FFI analysis
- `PQ_VERIFICATION_REPORT.md` - Post-quantum status

---

**Created**: 2025-11-10
**Purpose**: Prevent returning to "fake benchmarks and stub engines" era
**Motto**: "Test reality, not marketing"
