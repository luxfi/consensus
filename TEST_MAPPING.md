# Avalanchego to Lux Consensus Test Mapping

## Naming Convention
- snowball → photon (quantum sampling)
- snowflake → wave (wave interference) 
- snowman → beam (focused consensus)
- avalanche → nova (multiple selection)
- slush → flare (initial preference)

## Test Coverage Mapping

### Snowball Tests (→ Photon)
| Avalanchego Test | Lux Test | Status |
|-----------------|----------|---------|
| binary_snowball_test.go | dyadic_photon_test.go | ✅ Exists |
| binary_snowflake_test.go | dyadic_wave_test.go | ✅ Exists |
| nnary_snowball_test.go | polyadic_photon_test.go | ✅ Exists |
| nnary_snowflake_test.go | polyadic_wave_test.go | ✅ Exists |
| unary_snowball_test.go | monadic_photon_test.go | ✅ Exists |
| unary_snowflake_test.go | monadic_wave_test.go | ✅ Exists |
| tree_test.go | tree_test.go | ✅ Exists |
| flat_test.go | flat_test.go | ✅ Ported |
| consensus_test.go | consensus_test.go | ✅ Exists |
| consensus_performance_test.go | performance_test.go | ✅ Ported |
| consensus_reversibility_test.go | reversibility_test.go | ✅ Ported |
| parameters_test.go | parameters_test.go | ✅ Ported |

### Snowman Tests (→ Beam)
| Avalanchego Test | Lux Test | Status |
|-----------------|----------|---------|
| consensus_test.go (TestTopological) | topological_test.go | ✅ Exists |
| mixed_test.go | mixed_test.go | ✅ Exists |
| network_test.go | network_test.go | ✅ Exists |
| poll/* tests | poll/* tests | ✅ Exists |
| bootstrapper/* tests | bootstrapper/* tests | ✅ Exists |

### Missing Critical Tests to Port

1. **Flat Consensus Test** (snowball/flat_test.go)
   - Tests the flat (non-tree) consensus implementation
   - Critical for understanding basic consensus without tree optimization

2. **Consensus Performance Tests** (snowball/consensus_performance_test.go)
   - Benchmarks consensus performance
   - Important for ensuring our implementation performs well

3. **Consensus Reversibility Tests** (snowball/consensus_reversibility_test.go) 
   - Tests that finalized decisions cannot be reversed
   - Critical for consensus safety

4. **Parameters Tests** (snowball/parameters_test.go)
   - Validates parameter configurations
   - Important for preventing invalid configurations

5. **All Test Functions from consensus_test.go**
   - TestSnowballBinary
   - TestSnowballLastBinary
   - TestSnowballAddPreviouslyRejected
   - TestSnowballNewUnary
   - TestSnowballTransitiveReset
   - TestSnowballTrinary
   - TestSnowballCloseTrinary
   - TestSnowball5Colors
   - TestSnowballSingleton
   - TestSnowballRecordUnsuccessfulPoll
   - TestVirtuousNnarySnowball
   - TestNarySnowballRecordUnsuccessfulPoll
   - TestNarySnowballDifferentSnowflakeColor
   - TestSnowballConsistent
   - TestSnowballFilterBinaryChildren
   - TestSnowballDoubleAdd
   - TestSnowballAddDecidedFirstBit
   - TestSnowballResetChild
   - TestSnowballResetSibling
   - TestSnowballFineGrained
   - TestSnowballGovernance
   - TestSnowballRecordPreferencePollBinary
   - TestSnowballRecordPreferencePollUnary
   - TestBinarySnowballRecordPollPreference
   - TestBinarySnowballAcceptWeirdColor
   - TestBinarySnowballRecordUnsuccessfulPoll
   - TestBinarySnowballLockColor
   - TestSnowflakeBinary (from snowflake_test.go)

## Action Items
1. Port all missing tests with proper naming
2. Ensure 100% feature parity
3. Run full test suite to verify correctness
4. Clean up any dead code identified by staticcheck
5. Achieve >80% test coverage