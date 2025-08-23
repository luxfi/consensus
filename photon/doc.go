// Package photon chooses a K-sized committee each round.
//
// The metaphor: we "emit" K rays into the validator space and read back votes.
// In code, this is just a reproducible, unbiased selection over a population.
// No physics is implied or required.
//
// Typical use: engines pass a seed/phase to produce a stable committee for
// a round; wave applies thresholds over the observed tallies.
package photon