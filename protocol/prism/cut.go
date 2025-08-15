package prism

// Cut manages alpha/beta thresholds
type Cut struct {
	AlphaPreference int
	AlphaConfidence int
	Beta            int
}

// NewCut creates a new Cut
func NewCut(alphaPreference, alphaConfidence, beta int) *Cut {
	return &Cut{
		AlphaPreference: alphaPreference,
		AlphaConfidence: alphaConfidence,
		Beta:            beta,
	}
}

// PreferenceThreshold returns the preference threshold
func (c *Cut) PreferenceThreshold(k int) int {
	if c.AlphaPreference > 0 {
		return c.AlphaPreference
	}
	return (k + 1) / 2 // Default to majority
}

// ConfidenceThreshold returns the confidence threshold
func (c *Cut) ConfidenceThreshold(k int) int {
	if c.AlphaConfidence > 0 {
		return c.AlphaConfidence
	}
	return k // Default to all
}

// IsConfident checks if beta rounds have passed
func (c *Cut) IsConfident(rounds int) bool {
	return rounds >= c.Beta
}
