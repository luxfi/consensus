// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// Single AI Engine Implementation - Composable Modules

package ai

import (
	"context"
	"fmt"
	"sync"
)

// engine is the single AI engine implementation
type engine struct {
	mu      sync.RWMutex
	modules map[string]Module
	config  Config
}

// NewEngine creates the single AI engine
func NewEngine() Engine {
	return &engine{
		modules: make(map[string]Module),
	}
}

// === MODULE MANAGEMENT - ONE WAY ===

func (e *engine) AddModule(module Module) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	id := module.ID()
	if _, exists := e.modules[id]; exists {
		return fmt.Errorf("module %s already exists", id)
	}

	e.modules[id] = module
	return nil
}

func (e *engine) RemoveModule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	module, exists := e.modules[id]
	if !exists {
		return fmt.Errorf("module %s not found", id)
	}

	// Stop module before removal
	if err := module.Stop(context.Background()); err != nil {
		return fmt.Errorf("failed to stop module %s: %w", id, err)
	}

	delete(e.modules, id)
	return nil
}

func (e *engine) GetModule(id string) Module {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.modules[id]
}

func (e *engine) ListModules() []Module {
	e.mu.RLock()
	defer e.mu.RUnlock()

	modules := make([]Module, 0, len(e.modules))
	for _, module := range e.modules {
		modules = append(modules, module)
	}
	return modules
}

// === PROCESSING PIPELINE - ONE WAY ===

func (e *engine) Process(ctx context.Context, input Input) (Output, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Process through pipeline: Inference → Decision → Learning → Coordination
	currentOutput := Output{Type: OutputAnalysis, Data: make(map[string]interface{})}

	// Stage 1: Inference modules
	for _, module := range e.modules {
		if module.Type() == ModuleInference {
			output, err := module.Process(ctx, input)
			if err != nil {
				return Output{}, fmt.Errorf("inference module %s failed: %w", module.ID(), err)
			}
			// Merge outputs
			for k, v := range output.Data {
				currentOutput.Data[k] = v
			}
		}
	}

	// Stage 2: Decision modules
	decisionInput := Input{
		Type: InputQuery,
		Data: currentOutput.Data,
	}

	for _, module := range e.modules {
		if module.Type() == ModuleDecision {
			output, err := module.Process(ctx, decisionInput)
			if err != nil {
				return Output{}, fmt.Errorf("decision module %s failed: %w", module.ID(), err)
			}
			currentOutput.Type = OutputDecision
			for k, v := range output.Data {
				currentOutput.Data[k] = v
			}
		}
	}

	// Stage 3: Learning modules (async)
	go e.processLearning(context.Background(), input, currentOutput)

	// Stage 4: Coordination modules (async)
	go e.processCoordination(context.Background(), input, currentOutput)

	return currentOutput, nil
}

func (e *engine) processLearning(ctx context.Context, input Input, output Output) {
	learningInput := Input{
		Type: input.Type,
		Data: output.Data,
		Context: map[string]interface{}{
			"original_input": input.Data,
		},
	}

	for _, module := range e.modules {
		if module.Type() == ModuleLearning {
			_, _ = module.Process(ctx, learningInput)
		}
	}
}

func (e *engine) processCoordination(ctx context.Context, input Input, output Output) {
	coordInput := Input{
		Type: input.Type,
		Data: output.Data,
		Context: map[string]interface{}{
			"decision": output.Data,
		},
	}

	for _, module := range e.modules {
		if module.Type() == ModuleCoordination {
			_, _ = module.Process(ctx, coordInput)
		}
	}
}

// === CONFIGURATION - ONE WAY ===

func (e *engine) Configure(config Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.config = config

	// Configure all modules
	for id, module := range e.modules {
		moduleConfig, exists := config.Modules[id]
		if exists {
			if err := module.Initialize(context.Background(), Config{
				Global: config.Global,
				Modules: map[string]interface{}{id: moduleConfig},
			}); err != nil {
				return fmt.Errorf("failed to configure module %s: %w", id, err)
			}
		}
	}

	return nil
}

// === BUILDER IMPLEMENTATION ===

type builder struct {
	modules []moduleSpec
}

type moduleSpec struct {
	id     string
	typ    ModuleType
	config interface{}
}

func NewBuilder() Builder {
	return &builder{
		modules: make([]moduleSpec, 0),
	}
}

func (b *builder) WithInference(moduleID string, config interface{}) Builder {
	b.modules = append(b.modules, moduleSpec{
		id:     moduleID,
		typ:    ModuleInference,
		config: config,
	})
	return b
}

func (b *builder) WithDecision(moduleID string, config interface{}) Builder {
	b.modules = append(b.modules, moduleSpec{
		id:     moduleID,
		typ:    ModuleDecision,
		config: config,
	})
	return b
}

func (b *builder) WithLearning(moduleID string, config interface{}) Builder {
	b.modules = append(b.modules, moduleSpec{
		id:     moduleID,
		typ:    ModuleLearning,
		config: config,
	})
	return b
}

func (b *builder) WithCoordination(moduleID string, config interface{}) Builder {
	b.modules = append(b.modules, moduleSpec{
		id:     moduleID,
		typ:    ModuleCoordination,
		config: config,
	})
	return b
}

func (b *builder) Build() (Engine, error) {
	engine := NewEngine()

	// Create and add modules
	config := Config{
		Modules: make(map[string]interface{}),
		Global:  make(map[string]interface{}),
	}

	for _, spec := range b.modules {
		module, err := createModule(spec.id, spec.typ, spec.config)
		if err != nil {
			return nil, fmt.Errorf("failed to create module %s: %w", spec.id, err)
		}

		if err := engine.AddModule(module); err != nil {
			return nil, fmt.Errorf("failed to add module %s: %w", spec.id, err)
		}

		config.Modules[spec.id] = spec.config
	}

	if err := engine.Configure(config); err != nil {
		return nil, fmt.Errorf("failed to configure engine: %w", err)
	}

	return engine, nil
}

// createModule is the single factory for all module types
func createModule(id string, typ ModuleType, config interface{}) (Module, error) {
	switch typ {
	case ModuleInference:
		return NewInferenceModule(id, config), nil
	case ModuleDecision:
		return NewDecisionModule(id, config), nil
	case ModuleLearning:
		return NewLearningModule(id, config), nil
	case ModuleCoordination:
		return NewCoordinationModule(id, config), nil
	default:
		return nil, fmt.Errorf("unknown module type: %s", typ)
	}
}