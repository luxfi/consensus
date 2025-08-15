package prism

// RefractConfig configures a Refractor
type RefractConfig struct {
	EarlyTermination bool
}

// Refractor handles traversal and early termination
type Refractor struct {
	earlyTermination bool
}

// NewRefractor creates a new Refractor
func NewRefractor(cfg RefractConfig) *Refractor {
	return &Refractor{
		earlyTermination: cfg.EarlyTermination,
	}
}

// ShouldTerminate checks if traversal should stop early
func (r *Refractor) ShouldTerminate(confidence int, threshold int) bool {
	if !r.earlyTermination {
		return false
	}
	return confidence >= threshold
}
