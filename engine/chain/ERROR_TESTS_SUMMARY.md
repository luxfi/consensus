# Error Propagation Tests for Lux Consensus

## Overview
Successfully ported error propagation tests from avalanchego's snowman consensus to Lux consensus engine.

## Tests Implemented

### Core Error Tests
1. **TestErrorOnAccept** - Tests that errors during block acceptance are properly propagated
2. **TestErrorOnRejectSibling** - Tests that errors during sibling rejection are properly propagated
3. **TestMetricsProcessingError** - Tests that metric registration errors are handled properly
4. **TestErrorOnAddDecidedBlock** - Tests that adding an already decided block results in error
5. **TestTransitiveRejectionError** - Tests error propagation during transitive block rejection

### Metrics Error Tests
6. **TestMetricsAcceptedError** - Tests that accepted counter metric conflicts are handled
7. **TestMetricsRejectedError** - Tests that rejected counter metric conflicts are handled

### Graceful Failure Tests
8. **TestGracefulFailureHandling** - Tests that the consensus engine handles failures gracefully
9. **TestMetricsAccuracyDuringErrors** - Tests that metrics remain accurate even when errors occur

## Implementation Details

### Key Components
- **ErrorBlock** - Test block implementation that can inject errors on Accept/Reject/Verify
- **Consensus** - Simplified consensus engine for testing with proper error propagation
- **ConsensusContext** - Context with Prometheus registerer for metrics testing
- **Parameters** - Consensus parameters matching snowball configuration

### Adaptations from avalanchego
1. Used Lux's `bag.Bag` from `node/utils/bag` instead of avalanche's bag implementation
2. Adapted to Lux's block interfaces in `engine/chain/block`
3. Used Lux's test utilities from `engine/chain/chaintest`
4. Fixed bag initialization issues (requires SetThreshold to avoid nil set bug)
5. Used separate Prometheus registries per test to avoid metric conflicts

## Test Results
All 9 error propagation tests pass successfully:
```
=== RUN   TestErrorOnAccept
--- PASS: TestErrorOnAccept (0.00s)
=== RUN   TestErrorOnRejectSibling
--- PASS: TestErrorOnRejectSibling (0.00s)
=== RUN   TestMetricsProcessingError
--- PASS: TestMetricsProcessingError (0.00s)
=== RUN   TestErrorOnAddDecidedBlock
--- PASS: TestErrorOnAddDecidedBlock (0.00s)
=== RUN   TestTransitiveRejectionError
--- PASS: TestTransitiveRejectionError (0.00s)
=== RUN   TestMetricsAcceptedError
--- PASS: TestMetricsAcceptedError (0.00s)
=== RUN   TestMetricsRejectedError
--- PASS: TestMetricsRejectedError (0.00s)
=== RUN   TestGracefulFailureHandling
--- PASS: TestGracefulFailureHandling (0.00s)
=== RUN   TestMetricsAccuracyDuringErrors
--- PASS: TestMetricsAccuracyDuringErrors (0.00s)
```

## File Location
`/Users/z/work/lux/consensus/engine/chain/error_propagation_test.go`

## Known Issues
- The `bag.Bag` implementation has a bug where `metThreshold` set is not properly initialized when threshold is 0
- Worked around by setting threshold to 2 before adding elements
- This should be fixed in the bag implementation itself

## Future Work
- Consider fixing the bag initialization bug in `/Users/z/work/lux/node/utils/bag/bag.go`
- Add more comprehensive error scenarios
- Add benchmarks for error handling performance