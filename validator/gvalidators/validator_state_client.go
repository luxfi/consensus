// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gvalidators

import (
	"context"
	"errors"

	"github.com/luxfi/consensus/validator"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"

	pb "github.com/luxfi/consensus/proto/pb/validatorstate"
)

var (
	_                             validator.State = (*Client)(nil)
	errFailedPublicKeyDeserialize                 = errors.New("couldn't deserialize public key")
)

type Client struct {
	client pb.ValidatorStateClient
}

func NewClient(client pb.ValidatorStateClient) *Client {
	return &Client{client: client}
}

func (c *Client) GetMinimumHeight(ctx context.Context) (uint64, error) {
	resp, err := c.client.GetMinimumHeight(ctx, &pb.GetMinimumHeightRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Height, nil
}

func (c *Client) GetCurrentHeight(ctx context.Context) (uint64, error) {
	resp, err := c.client.GetCurrentHeight(ctx, &pb.GetCurrentHeightRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Height, nil
}

func (c *Client) GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error) {
	resp, err := c.client.GetSubnetID(ctx, &pb.GetSubnetIDRequest{
		ChainId: chainID[:],
	})
	if err != nil {
		return ids.Empty, err
	}
	return ids.ToID(resp.SubnetId)
}

func (c *Client) GetValidatorSet(
	ctx context.Context,
	height uint64,
	subnetID ids.ID,
) (map[ids.NodeID]*validator.GetValidatorOutput, error) {
	resp, err := c.client.GetValidatorSet(ctx, &pb.GetValidatorSetRequest{
		Height:   height,
		SubnetId: subnetID[:],
	})
	if err != nil {
		return nil, err
	}

	vdrs := make(map[ids.NodeID]*validator.GetValidatorOutput, len(resp.Validators))
	for _, vdr := range resp.Validators {
		nodeID, err := ids.ToNodeID(vdr.NodeId)
		if err != nil {
			return nil, err
		}
		var publicKey *bls.PublicKey
		if len(vdr.PublicKey) > 0 {
			// PublicKeyFromValidUncompressedBytes is used rather than
			// PublicKeyFromCompressedBytes because it is significantly faster
			// due to the avoidance of decompression and key re-verification. We
			// can safely assume that the BLS Public Keys are verified before
			// being added to the P-Chain and served by the gRPC server.
			publicKey = bls.PublicKeyFromValidUncompressedBytes(vdr.PublicKey)
			if publicKey == nil {
				return nil, errFailedPublicKeyDeserialize
			}
		}
		vdrs[nodeID] = &validator.GetValidatorOutput{
			NodeID:    nodeID,
			PublicKey: publicKey,
			Weight:    vdr.Weight,
		}
	}
	return vdrs, nil
}

func (c *Client) GetCurrentValidatorSet(
	ctx context.Context,
	subnetID ids.ID,
) (map[ids.ID]*validator.GetCurrentValidatorOutput, uint64, error) {
	resp, err := c.client.GetCurrentValidatorSet(ctx, &pb.GetCurrentValidatorSetRequest{
		SubnetId: subnetID[:],
	})
	if err != nil {
		return nil, 0, err
	}

	vdrs := make(map[ids.ID]*validator.GetCurrentValidatorOutput, len(resp.Validators))
	for _, vdr := range resp.Validators {
		nodeID, err := ids.ToNodeID(vdr.NodeId)
		if err != nil {
			return nil, 0, err
		}
		var publicKey *bls.PublicKey
		if len(vdr.PublicKey) > 0 {
			// PublicKeyFromValidUncompressedBytes is used rather than
			// PublicKeyFromCompressedBytes because it is significantly faster
			// due to the avoidance of decompression and key re-verification. We
			// can safely assume that the BLS Public Keys are verified before
			// being added to the P-Chain and served by the gRPC server.
			publicKey = bls.PublicKeyFromValidUncompressedBytes(vdr.PublicKey)
			if publicKey == nil {
				return nil, 0, errFailedPublicKeyDeserialize
			}
		}
		validationID, err := ids.ToID(vdr.ValidationId)
		if err != nil {
			return nil, 0, err
		}

		vdrs[validationID] = &validator.GetCurrentValidatorOutput{
			ValidationID:  validationID,
			NodeID:        nodeID,
			PublicKey:     publicKey,
			Weight:        vdr.Weight,
			StartTime:     vdr.StartTime,
			MinNonce:      vdr.MinNonce,
			IsActive:      vdr.IsActive,
			IsL1Validator: vdr.IsL1Validator,
		}
	}
	return vdrs, resp.GetCurrentHeight(), nil
}
