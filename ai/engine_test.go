// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// AI Engine Tests

package ai

import (
	"context"
	"testing"
)

// === Engine Constructor Tests ===

func TestNewEngine(t *testing.T) {
	engine := NewEngine()
	
	if engine == nil {
		t.Fatal("NewEngine() returned nil")
	}
	
	modules := engine.ListModules()
	if len(modules) != 0 {
		t.Errorf("Expected 0 modules initially, got %d", len(modules))
	}
}

// === AddModule Tests ===

func TestEngineAddModule(t *testing.T) {
	engine := NewEngine()
	
	module := &testModule{
		id:   "test-module",
		typ:  ModuleInference,
	}
	
	err := engine.AddModule(module)
	
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	
	modules := engine.ListModules()
	if len(modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(modules))
	}
}

func TestEngineAddModule_Duplicate(t *testing.T) {
	engine := NewEngine()
	
	module := &testModule{
		id:  "test-module",
		typ: ModuleInference,
	}
	
	// Add first time
	err := engine.AddModule(module)
	if err != nil {
		t.Fatalf("First AddModule() error = %v", err)
	}
	
	// Try to add duplicate
	err = engine.AddModule(module)
	if err == nil {
		t.Fatal("Expected error for duplicate module, got nil")
	}
	
	if err.Error() != "module test-module already exists" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// === RemoveModule Tests ===

func TestEngineRemoveModule(t *testing.T) {
	engine := NewEngine()
	
	module := &testModule{
		id:  "test-module",
		typ: ModuleInference,
	}
	
	// Add module
	err := engine.AddModule(module)
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	
	// Remove module
	err = engine.RemoveModule("test-module")
	if err != nil {
		t.Fatalf("RemoveModule() error = %v", err)
	}
	
	// Check it's gone
	modules := engine.ListModules()
	if len(modules) != 0 {
		t.Errorf("Expected 0 modules after removal, got %d", len(modules))
	}
}

func TestEngineRemoveModule_NotFound(t *testing.T) {
	engine := NewEngine()
	
	err := engine.RemoveModule("nonexistent")
	
	if err == nil {
		t.Fatal("Expected error for nonexistent module, got nil")
	}
	
	if err.Error() != "module nonexistent not found" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestEngineRemoveModule_StopError(t *testing.T) {
	engine := NewEngine()
	
	module := &testModule{
		id:       "test-module",
		typ:      ModuleInference,
		stopErr:  &testError{msg: "stop failed"},
	}
	
	err := engine.AddModule(module)
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	
	err = engine.RemoveModule("test-module")
	
	if err == nil {
		t.Fatal("Expected error when module stop fails, got nil")
	}
	
	if !contains(err.Error(), "failed to stop module") {
		t.Errorf("Unexpected error: %v", err)
	}
}

// === GetModule Tests ===

func TestEngineGetModule(t *testing.T) {
	engine := NewEngine()
	
	module := &testModule{
		id:  "test-module",
		typ: ModuleInference,
	}
	
	err := engine.AddModule(module)
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	
	retrieved := engine.GetModule("test-module")
	
	if retrieved == nil {
		t.Fatal("GetModule() returned nil")
	}
	
	if retrieved.ID() != "test-module" {
		t.Errorf("Expected ID 'test-module', got '%s'", retrieved.ID())
	}
}

func TestEngineGetModule_NotFound(t *testing.T) {
	engine := NewEngine()
	
	retrieved := engine.GetModule("nonexistent")
	
	if retrieved != nil {
		t.Error("Expected nil for nonexistent module")
	}
}

// === ListModules Tests ===

func TestEngineListModules_Multiple(t *testing.T) {
	engine := NewEngine()
	
	// Add multiple modules
	modules := []Module{
		&testModule{id: "module-1", typ: ModuleInference},
		&testModule{id: "module-2", typ: ModuleInference},
		&testModule{id: "module-3", typ: ModuleInference},
	}
	
	for _, m := range modules {
		err := engine.AddModule(m)
		if err != nil {
			t.Fatalf("AddModule() error = %v", err)
		}
	}
	
	list := engine.ListModules()
	
	if len(list) != 3 {
		t.Errorf("Expected 3 modules, got %d", len(list))
	}
	
	// Check all modules are present
	ids := make(map[string]bool)
	for _, m := range list {
		ids[m.ID()] = true
	}
	
	for i := 1; i <= 3; i++ {
		id := "module-" + string(rune('0'+i))
		if !ids[id] {
			t.Errorf("Module %s not found in list", id)
		}
	}
}

// === Process Tests ===

func TestEngineProcess(t *testing.T) {
	engine := NewEngine()
	
	module := &testModule{
		id:  "test-module",
		typ: ModuleInference,
	}
	
	err := engine.AddModule(module)
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	
	input := Input{
		Type: "block",
		Data: map[string]interface{}{
			"height": 100,
		},
	}
	
	output, err := engine.Process(context.Background(), input)
	
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	
	// Engine initializes output with "analysis" type, not "processed"
	if output.Type != "analysis" {
		t.Errorf("Expected output type 'analysis', got '%s'", output.Type)
	}
	
	if !module.processCalled {
		t.Error("Module Process() was not called")
	}
}

func TestEngineProcess_ModuleError(t *testing.T) {
	engine := NewEngine()
	
	module := &testModule{
		id:         "test-module",
		typ:        ModuleInference,
		processErr: &testError{msg: "process failed"},
	}
	
	err := engine.AddModule(module)
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}
	
	input := Input{
		Type: "block",
		Data: map[string]interface{}{},
	}
	
	_, err = engine.Process(context.Background(), input)
	
	if err == nil {
		t.Fatal("Expected error from module, got nil")
	}
}

// === Configure Tests ===

func TestEngineConfigure(t *testing.T) {
	engine := NewEngine()
	
	config := Config{
		Global: map[string]interface{}{
			"max_modules": 10,
			"timeout":     "5s",
		},
		Modules: map[string]interface{}{},
	}
	
	err := engine.Configure(config)
	
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
}


func TestEngineConfigure_WithModules(t *testing.T) {
	engine := NewEngine()

	module := &testModule{
		id:  "test-module",
		typ: ModuleInference,
	}

	err := engine.AddModule(module)
	if err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}

	config := Config{
		Global: map[string]interface{}{
			"max_modules": 10,
		},
		Modules: map[string]interface{}{
			"test-module": map[string]interface{}{
				"enabled": true,
			},
		},
	}

	err = engine.Configure(config)

	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	// Verify module was initialized
	if !module.initializeCalled {
		t.Error("Module Initialize() was not called")
	}
}

// === Builder Tests ===

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder()
	
	if builder == nil {
		t.Fatal("NewBuilder() returned nil")
	}
}

func TestBuilderBuild(t *testing.T) {
	builder := NewBuilder()
	
	engine, err := builder.Build()
	
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	
	if engine == nil {
		t.Fatal("Build() returned nil engine")
	}
}

func TestBuilderWithInference(t *testing.T) {
	builder := NewBuilder()
	
	builder = builder.WithInference("test-inference", nil)
	
	engine, err := builder.Build()
	
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	
	modules := engine.ListModules()
	
	// Should have inference module
	hasInference := false
	for _, m := range modules {
		if m.Type() == ModuleInference {
			hasInference = true
			break
		}
	}
	
	if !hasInference {
		t.Error("Expected inference module after WithInference()")
	}
}

func TestBuilderWithDecision(t *testing.T) {
	builder := NewBuilder()
	
	builder = builder.WithDecision("test-decision", nil)
	
	engine, err := builder.Build()
	
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	
	modules := engine.ListModules()
	
	hasDecision := false
	for _, m := range modules {
		if m.Type() == ModuleDecision {
			hasDecision = true
			break
		}
	}
	
	if !hasDecision {
		t.Error("Expected decision module after WithDecision()")
	}
}

func TestBuilderWithLearning(t *testing.T) {
	builder := NewBuilder()
	
	builder = builder.WithLearning("test-learning", nil)
	
	engine, err := builder.Build()
	
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	
	modules := engine.ListModules()
	
	hasLearning := false
	for _, m := range modules {
		if m.Type() == ModuleLearning {
			hasLearning = true
			break
		}
	}
	
	if !hasLearning {
		t.Error("Expected learning module after WithLearning()")
	}
}

func TestBuilderWithCoordination(t *testing.T) {
	builder := NewBuilder()
	
	builder = builder.WithCoordination("test-coordination", nil)
	
	engine, err := builder.Build()
	
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	
	modules := engine.ListModules()
	
	hasCoordination := false
	for _, m := range modules {
		if m.Type() == ModuleCoordination {
			hasCoordination = true
			break
		}
	}
	
	if !hasCoordination {
		t.Error("Expected coordination module after WithCoordination()")
	}
}

func TestBuilderChaining(t *testing.T) {
	builder := NewBuilder()
	
	engine, err := builder.
		WithInference("inference-1", nil).
		WithDecision("decision-1", nil).
		WithLearning("learning-1", nil).
		WithCoordination("coordination-1", nil).
		Build()
	
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	
	modules := engine.ListModules()
	
	if len(modules) != 4 {
		t.Errorf("Expected 4 modules, got %d", len(modules))
	}
}

// === Test Helper Types ===

type testModule struct {
	id             string
	typ            ModuleType
	stopErr        error
	processErr     error
	processCalled  bool
	initializeCalled bool
}

func (m *testModule) ID() string {
	return m.id
}

func (m *testModule) Type() ModuleType {
	return m.typ
}

func (m *testModule) Initialize(ctx context.Context, config Config) error {
	m.initializeCalled = true
	return nil
}

func (m *testModule) Process(ctx context.Context, input Input) (Output, error) {
	m.processCalled = true
	
	if m.processErr != nil {
		return Output{}, m.processErr
	}
	
	return Output{
		Type: "processed",
		Data: map[string]interface{}{
			"processed": true,
		},
	}, nil
}

func (m *testModule) Start(ctx context.Context) error {
	return nil
}

func (m *testModule) Stop(ctx context.Context) error {
	return m.stopErr
}
