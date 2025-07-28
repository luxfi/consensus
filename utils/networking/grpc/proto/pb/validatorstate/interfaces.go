// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validatorstate

import (
	"context"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ValidatorStateClient is the client interface for validator state
type ValidatorStateClient interface {
	GetMinimumHeight(ctx context.Context, in *emptypb.Empty) (*GetMinimumHeightResponse, error)
	GetCurrentHeight(ctx context.Context, in *emptypb.Empty) (*GetCurrentHeightResponse, error)
	GetSubnetID(ctx context.Context, in *GetSubnetIDRequest) (*GetSubnetIDResponse, error)
	GetValidatorSet(ctx context.Context, in *GetValidatorSetRequest) (*GetValidatorSetResponse, error)
	GetCurrentValidatorSet(ctx context.Context, in *GetCurrentValidatorSetRequest) (*GetCurrentValidatorSetResponse, error)
}

// ValidatorStateServer is the server interface for validator state
type ValidatorStateServer interface {
	GetMinimumHeight(context.Context, *emptypb.Empty) (*GetMinimumHeightResponse, error)
	GetCurrentHeight(context.Context, *emptypb.Empty) (*GetCurrentHeightResponse, error)
	GetSubnetID(context.Context, *GetSubnetIDRequest) (*GetSubnetIDResponse, error)
	GetValidatorSet(context.Context, *GetValidatorSetRequest) (*GetValidatorSetResponse, error)
	GetCurrentValidatorSet(context.Context, *GetCurrentValidatorSetRequest) (*GetCurrentValidatorSetResponse, error)
}

// UnsafeValidatorStateServer is the unsafe server interface
type UnsafeValidatorStateServer interface {
	ValidatorStateServer
}

// GetMinimumHeightResponse response
type GetMinimumHeightResponse struct {
	Height uint64
}

// GetCurrentHeightResponse response
type GetCurrentHeightResponse struct {
	Height uint64
}

// GetSubnetIDRequest request
type GetSubnetIDRequest struct {
	ChainId []byte
}

// GetSubnetIDResponse response
type GetSubnetIDResponse struct {
	SubnetId []byte
}

// GetCurrentValidatorSetRequest request
type GetCurrentValidatorSetRequest struct {
	SubnetId []byte
}

// GetCurrentValidatorSetResponse response
type GetCurrentValidatorSetResponse struct {
	Validators        []*Validator
	CurrentHeight     uint64
}

// Additional Validator fields for current validator set
type Validator struct {
	NodeId         []byte
	PublicKey      []byte
	Weight         uint64
	StartTime      uint64
	MinNonce       uint64
	ValidationId   []byte
	IsActive       bool
	IsL1Validator  bool
}