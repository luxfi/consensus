// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gvalidators

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/validator"

	pb "github.com/luxfi/consensus/networking/grpc/proto/pb/validatorstate"
)

var _ pb.ValidatorStateServer = (*Server)(nil)

type Server struct {
	pb.UnsafeValidatorStateServer
	state validator.State
}

func NewServer(state validator.State) *Server {
	return &Server{state: state}
}

func (s *Server) GetMinimumHeight(ctx context.Context, _ *emptypb.Empty) (*pb.GetMinimumHeightResponse, error) {
	height, err := s.state.GetMinimumHeight(ctx)
	return &pb.GetMinimumHeightResponse{Height: height}, err
}

func (s *Server) GetCurrentHeight(ctx context.Context, _ *emptypb.Empty) (*pb.GetCurrentHeightResponse, error) {
	height, err := s.state.GetCurrentHeight(ctx)
	return &pb.GetCurrentHeightResponse{Height: height}, err
}

func (s *Server) GetSubnetID(ctx context.Context, req *pb.GetSubnetIDRequest) (*pb.GetSubnetIDResponse, error) {
	chainID, err := ids.ToID(req.ChainId)
	if err != nil {
		return nil, err
	}

	subnetID, err := s.state.GetSubnetID(ctx, chainID)
	return &pb.GetSubnetIDResponse{
		SubnetId: subnetID[:],
	}, err
}

func (s *Server) GetValidatorSet(ctx context.Context, req *pb.GetValidatorSetRequest) (*pb.GetValidatorSetResponse, error) {
	subnetID, err := ids.ToID(req.SubnetId)
	if err != nil {
		return nil, err
	}

	vdrs, err := s.state.GetValidatorSet(ctx, req.Height, subnetID)
	if err != nil {
		return nil, err
	}

	resp := &pb.GetValidatorSetResponse{
		Validators: make([]*pb.Validator, len(vdrs)),
	}

	i := 0
	for _, vdr := range vdrs {
		vdrPB := &pb.Validator{
			NodeId: vdr.NodeID.Bytes(),
			Weight: vdr.Weight,
		}
		if vdr.PublicKey != nil {
			// Passing in the uncompressed bytes is a performance optimization
			// to avoid the cost of calling PublicKeyFromCompressedBytes on the
			// client side.
			vdrPB.PublicKey = bls.PublicKeyToUncompressedBytes(vdr.PublicKey)
		}
		resp.Validators[i] = vdrPB
		i++
	}
	return resp, nil
}

func (s *Server) GetCurrentValidatorSet(ctx context.Context, req *pb.GetCurrentValidatorSetRequest) (*pb.GetCurrentValidatorSetResponse, error) {
	subnetID, err := ids.ToID(req.SubnetId)
	if err != nil {
		return nil, err
	}

	vdrs, currentHeight, err := s.state.GetCurrentValidatorSet(ctx, subnetID)
	if err != nil {
		return nil, err
	}

	resp := &pb.GetCurrentValidatorSetResponse{
		Validators:    make([]*pb.Validator, len(vdrs)),
		CurrentHeight: currentHeight,
	}

	i := 0
	for _, vdr := range vdrs {
		vdrPB := &pb.Validator{
			NodeId:        vdr.NodeID.Bytes(),
			StartTime:     vdr.StartTime,
			IsActive:      vdr.IsActive,
			ValidationId:  vdr.ValidationID[:],
			Weight:        vdr.Weight,
			MinNonce:      vdr.MinNonce,
			IsL1Validator: vdr.IsL1Validator,
		}
		if vdr.PublicKey != nil {
			// Passing in the uncompressed bytes is a performance optimization
			// to avoid the cost of calling PublicKeyFromCompressedBytes on the
			// client side.
			vdrPB.PublicKey = bls.PublicKeyToUncompressedBytes(vdr.PublicKey)
		}
		resp.Validators[i] = vdrPB
		i++
	}
	return resp, nil
}
