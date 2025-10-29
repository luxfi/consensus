// Copyright (C) 2025, Lux Partners Limited All rights reserved.
// Orthogonal, Composable AI Interfaces - One Way To Do Everything

package ai

import (
	"context"
)

// === CORE ORTHOGONAL INTERFACES ===

// Module is the single interface all AI components implement
type Module interface {
	// Identity
	ID() string
	Type() ModuleType

	// Lifecycle - exactly one way to manage state
	Initialize(ctx context.Context, config Config) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Processing - exactly one way to process data
	Process(ctx context.Context, input Input) (Output, error)
}

// ModuleType defines orthogonal module categories
type ModuleType string

const (
	// Inference modules
	ModuleInference ModuleType = "inference"

	// Decision modules
	ModuleDecision ModuleType = "decision"

	// Learning modules
	ModuleLearning ModuleType = "learning"

	// Coordination modules
	ModuleCoordination ModuleType = "coordination"
)

// Engine composes modules - single composition interface
type Engine interface {
	// Module management - exactly one way
	AddModule(module Module) error
	RemoveModule(id string) error
	GetModule(id string) Module
	ListModules() []Module

	// Processing pipeline - exactly one way
	Process(ctx context.Context, input Input) (Output, error)

	// Configuration - exactly one way
	Configure(config Config) error
}

// === ORTHOGONAL DATA TYPES ===

// Input - single input format for all modules
type Input struct {
	Type    InputType              `json:"type"`
	Data    map[string]interface{} `json:"data"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// Output - single output format for all modules
type Output struct {
	Type    OutputType             `json:"type"`
	Data    map[string]interface{} `json:"data"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// InputType defines what kind of data is being processed
type InputType string

const (
	InputBlock    InputType = "block"
	InputProposal InputType = "proposal"
	InputVote     InputType = "vote"
	InputQuery    InputType = "query"
)

// OutputType defines what kind of result is produced
type OutputType string

const (
	OutputDecision   OutputType = "decision"
	OutputPrediction OutputType = "prediction"
	OutputAnalysis   OutputType = "analysis"
	OutputAction     OutputType = "action"
)

// Config - single configuration format
type Config struct {
	// Module-specific config
	Modules map[string]interface{} `json:"modules"`

	// Global settings
	Global map[string]interface{} `json:"global"`
}

// === COMPOSITION BUILDER ===

// Builder creates engines with composed modules - single way to build
type Builder interface {
	// Fluent interface for composition
	WithInference(moduleID string, config interface{}) Builder
	WithDecision(moduleID string, config interface{}) Builder
	WithLearning(moduleID string, config interface{}) Builder
	WithCoordination(moduleID string, config interface{}) Builder

	// Build the final engine
	Build() (Engine, error)
}