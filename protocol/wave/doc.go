// Package wave computes per-round thresholds and drives a poll.
//
// Each round has two thresholds—α_pref and α_conf—that gate preference and
// confidence. The default selector is FPC: a phase-dependent threshold chosen
// from [θ_min, θ_max], reproducibly derived from a PRF.
//
// Wave doesn't decide anything alone; it transforms a committee's tallies into
// a boolean (preferOK, confOK) that downstream focus can integrate.
package wave
