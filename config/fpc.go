package config

// FPCConfig represents Firing Photon Cannon configuration
type FPCConfig struct {
	// Enable indicates whether FPC is enabled (default: true)
	Enable bool

	// Rounds is the number of FPC rounds to run
	Rounds int

	// Threshold is the confidence threshold for FPC
	Threshold float64

	// SampleSize is the number of nodes to sample per round
	SampleSize int
}

// DefaultFPC returns the default FPC configuration
func DefaultFPC() FPCConfig {
	return FPCConfig{
		Enable:     true, // FPC enabled by default
		Rounds:     10,
		Threshold:  0.8,
		SampleSize: 20,
	}
}

// WithFPC adds FPC configuration to Parameters
func (p Parameters) WithFPC(fpc FPCConfig) Parameters {
	// In a real implementation, you'd add FPC fields to Parameters
	// For now, just return the parameters unchanged
	return p
}
