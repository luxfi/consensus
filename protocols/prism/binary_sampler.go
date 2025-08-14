package prism

// BinarySampler provides binary sampling for consensus
type BinarySampler struct {
    k int
}

// NewBinarySampler creates a new binary sampler
func NewBinarySampler(k int) *BinarySampler {
    return &BinarySampler{k: k}
}

// Sample performs binary sampling
func (b *BinarySampler) Sample(n int) bool {
    // Simple implementation - should be enhanced with proper sampling logic
    return n >= b.k
}