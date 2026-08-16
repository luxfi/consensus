package config

import "testing"

// Every preset a chain can be started with has to name a quorum that a
// two-thirds-honest network can actually reach and that a one-third adversary
// cannot: Alpha in [0.66, 1.0], and the integer AlphaPreference at least 66% of
// the sample K. A preset below the bound finalizes without agreement; one above
// 1.0 can never finalize at all. These are the values node ships, so an edit
// that drifts any of them must fail here.
func TestPresetsHoldTheByzantineBound(t *testing.T) {
	for name, params := range map[string]Parameters{
		"Default":  DefaultParams(),
		"Mainnet":  MainnetParams(),
		"Testnet":  TestnetParams(),
		"Local":    LocalParams(),
		"LocalBFT": LocalBFTParams(),
		"XChain":   XChainParams(),
		"Single":   SingleValidatorParams(),
	} {
		t.Run(name, func(t *testing.T) {
			if params.Alpha < 0.66 || params.Alpha > 1.0 {
				t.Errorf("Alpha %.3f outside [0.66, 1.0]", params.Alpha)
			}
			if ratio := float64(params.AlphaPreference) / float64(params.K); ratio < 0.66 {
				t.Errorf("AlphaPreference %d of K=%d is %.1f%%, under 66%%",
					params.AlphaPreference, params.K, ratio*100)
			}
			if err := params.Valid(); err != nil {
				t.Errorf("preset does not validate: %v", err)
			}
		})
	}
}
