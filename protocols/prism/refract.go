package prism

// Refract handles traversal and early termination
type Refract struct {
    earlyTermination bool
}

// ShouldTerminate checks if traversal should stop early
func (r *Refract) ShouldTerminate(confidence int, threshold int) bool {
    if !r.earlyTermination {
        return false
    }
    return confidence >= threshold
}
