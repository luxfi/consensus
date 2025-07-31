# Prism Package - Optics-Inspired Consensus

The prism package provides an elegant optical metaphor for Lux consensus mechanisms. Just as a prism refracts light into its component wavelengths, this package refracts validator opinions into clear consensus decisions.

## The Optical Model

### 1. Splitter (`splitter.go`)
The **input face** of our prism. Takes the full beam of validators and splits off exactly the sample you need.

- Maps to: AvalancheGo's `Sample()` method
- Function: Weighted sampling of k validators
- Optics parallel: Beam splitter selecting specific wavelengths

### 2. Facet (`facet.go`)
The **internal structure** that guides light paths. Each facet represents a path through the decision dependency graph.

- Maps to: Early-termination graph traversal
- Function: Efficient traversal with confidence accumulation
- Optics parallel: Crystal facets bending and refracting light

### 3. Cut (`cut.go`)
The **output angle** that determines which wavelengths pass through. Defines when you've collected enough votes.

- Maps to: α (quorum) and β (confidence) thresholds
- Function: Vote counting and finalization logic
- Optics parallel: Cut angle filtering specific wavelengths

## Usage Example

```go
// Create a prism with consensus parameters
params := config.DefaultParameters
prism := NewPrism(params, sampler.NewSource(seed))

// Run consensus rounds - light passes through the prism
for round := 1; round <= maxRounds; round++ {
    // The key method: Refract processes the entire optical path
    finalized, err := prism.Poll(validators, decisions)
    if err != nil {
        return err
    }
    
    if finalized {
        fmt.Printf("Consensus reached in round %d\n", round)
        break
    }
}

// Get the consensus result (the dominant wavelength)
preference := prism.GetPreference()
```

## The Refract Method

The `Refract` method is the heart of the prism metaphor. It processes light through all three optical components:

```go
func (p *Prism) Refract(validators ValidatorSet, decisions []ids.ID) (bool, error) {
    // 1. Split - Light enters the prism
    sample := p.splitter.Sample(validators, k)
    
    // 2. Facet - Light refracts through internal facets
    facets := p.traverser.Traverse(decisions)
    
    // 3. Process - Each ray passes through facets
    for _, validator := range sample {
        for _, facet := range facets {
            if facet.CanTerminate(threshold) {
                // This ray contributes to the output
                p.cut.RecordVote(facet.root, weight)
            }
        }
    }
    
    // 4. Cut - Evaluate at the output angle
    changed := p.cut.Refract()
    return changed
}
```

## Advanced Usage: Spectrum Analysis

The package also provides advanced tools for analyzing the consensus process:

```go
// Create a spectrum analyzer
spectrum := NewSpectrum(validators)

// Split beams separately from traversal
spectrum.Split(splitter, decision1, k)
spectrum.Split(splitter, decision2, k)

// Process through facets
spectrum.RefractThroughFacets(decision1, facets1, alphaConfidence)
spectrum.RefractThroughFacets(decision2, facets2, alphaConfidence)

// Analyze the results
analysis := spectrum.Analyze()
```

## Design Philosophy

The prism abstraction provides several benefits:

1. **Clarity**: Each component has a single, well-defined purpose
2. **Composability**: Components can be mixed and matched
3. **Testability**: Each optical element can be tested in isolation
4. **Elegance**: The metaphor makes complex consensus intuitive

## Mapping to AvalancheGo

| Prism Component | AvalancheGo Equivalent | Purpose |
|-----------------|------------------------|---------|
| Splitter | `Sample()` | Validator sampling |
| Facet | Graph traversal | Decision dependencies |
| Cut | α/β thresholds | Vote evaluation |

## Advanced Features

### PrismArray
Manages multiple consensus instances in parallel, like an array of prisms each handling different wavelengths (decisions).

### OpticalBench
Testing harness for experimenting with different optical configurations and measuring their performance.

### CutAnalyzer
Tracks metrics across multiple rounds to understand convergence behavior and optimization opportunities.

## The Beauty of Light

Just as white light contains all colors waiting to be revealed by a prism, the distributed opinions of validators contain consensus waiting to be revealed by the right optical configuration. This package makes that revelation both elegant and efficient.