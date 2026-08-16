package config

import "testing"

// AlphaForK must return the SMALLEST sample-vote count that reaches the 69%
// agreement threshold: at or above 69% of K, and one vote fewer must fall short.
// Rounding down anywhere here lets a chain finalize under the Byzantine bound;
// rounding up more than necessary costs liveness. Checked across every K a
// committee is plausibly sized at rather than a handful of sampled rows.
func TestAlphaForKIsTheCeilingOfTheThreshold(t *testing.T) {
	for k := 1; k <= 1000; k++ {
		alpha := AlphaForK(k)

		if float64(alpha) < float64(k)*ConsensusSuperMajority {
			t.Fatalf("AlphaForK(%d) = %d, under the %.0f%% threshold", k, alpha, ConsensusSuperMajority*100)
		}
		if alpha > 1 && float64(alpha-1) >= float64(k)*ConsensusSuperMajority {
			t.Fatalf("AlphaForK(%d) = %d, but %d already clears the threshold", k, alpha, alpha-1)
		}
		if alpha > k {
			t.Fatalf("AlphaForK(%d) = %d, more votes than the sample holds", k, alpha)
		}
	}
}
