// Copyright (C) 2019-2025, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package simplex

import (
	simplex "github.com/luxfi/bft"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/proto/pb/p2p"
)

func newBlockProposal(
	chainID ids.ID,
	block []byte,
	vote simplex.Vote,
) *p2p.BFT {
	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_BlockProposal{
			BlockProposal: &p2p.BlockProposal{
				Block: block,
				Vote: &p2p.Vote{
					BlockHeader: blockHeaderToP2P(vote.Vote.BlockHeader),
					Signature: &p2p.Signature{
						Signer: vote.Signature.Signer,
						Value:  vote.Signature.Value,
					},
				},
			},
		},
	}
}

func newVote(
	chainID ids.ID,
	vote *simplex.Vote,
) *p2p.BFT {
	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_Vote{
			Vote: &p2p.Vote{
				BlockHeader: blockHeaderToP2P(vote.Vote.BlockHeader),
				Signature: &p2p.Signature{
					Signer: vote.Signature.Signer,
					Value:  vote.Signature.Value,
				},
			},
		},
	}
}

func newEmptyVote(
	chainID ids.ID,
	emptyVote *simplex.EmptyVote,
) *p2p.BFT {
	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_EmptyVote{
			EmptyVote: &p2p.EmptyVote{
				Metadata: protocolMetadataToP2P(emptyVote.Vote.ProtocolMetadata),
				Signature: &p2p.Signature{
					Signer: emptyVote.Signature.Signer,
					Value:  emptyVote.Signature.Value,
				},
			},
		},
	}
}

func newFinalizeVote(
	chainID ids.ID,
	finalizeVote *simplex.FinalizeVote,
) *p2p.BFT {
	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_FinalizeVote{
			FinalizeVote: &p2p.Vote{
				BlockHeader: blockHeaderToP2P(finalizeVote.Finalization.BlockHeader),
				Signature: &p2p.Signature{
					Signer: finalizeVote.Signature.Signer,
					Value:  finalizeVote.Signature.Value,
				},
			},
		},
	}
}

func newNotarization(
	chainID ids.ID,
	notarization *simplex.Notarization,
) *p2p.BFT {
	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_Notarization{
			Notarization: &p2p.QuorumCertificate{
				BlockHeader:       blockHeaderToP2P(notarization.Vote.BlockHeader),
				QuorumCertificate: notarization.QC.Bytes(),
			},
		},
	}
}

func newEmptyNotarization(
	chainID ids.ID,
	emptyNotarization *simplex.EmptyNotarization,
) *p2p.BFT {
	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_EmptyNotarization{
			EmptyNotarization: &p2p.EmptyNotarization{
				Metadata:          protocolMetadataToP2P(emptyNotarization.Vote.ProtocolMetadata),
				QuorumCertificate: emptyNotarization.QC.Bytes(),
			},
		},
	}
}

func newFinalization(
	chainID ids.ID,
	finalization *simplex.Finalization,
) *p2p.BFT {
	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_Finalization{
			Finalization: &p2p.QuorumCertificate{
				BlockHeader:       blockHeaderToP2P(finalization.Finalization.BlockHeader),
				QuorumCertificate: finalization.QC.Bytes(),
			},
		},
	}
}

func newReplicationRequest(
	chainID ids.ID,
	replicationRequest *simplex.ReplicationRequest,
) *p2p.BFT {
	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_ReplicationRequest{
			ReplicationRequest: &p2p.ReplicationRequest{
				Seqs:        replicationRequest.Seqs,
				LatestRound: replicationRequest.LatestRound,
			},
		},
	}
}

func newReplicationResponse(
	chainID ids.ID,
	replicationResponse *simplex.VerifiedReplicationResponse,
) (*p2p.BFT, error) {
	data := replicationResponse.Data
	latestRound := replicationResponse.LatestRound

	// Convert each QuorumRound to bytes
	qrBytes := make([][]byte, 0, len(data))
	for _, qr := range data {
		// Get the QC bytes from the verified block or notarization
		var qcBytes []byte
		if qr.Notarization != nil {
			qcBytes = qr.Notarization.QC.Bytes()
		} else if qr.Finalization != nil {
			qcBytes = qr.Finalization.QC.Bytes()
		} else if qr.EmptyNotarization != nil {
			qcBytes = qr.EmptyNotarization.QC.Bytes()
		}
		qrBytes = append(qrBytes, qcBytes)
	}

	// Convert latest round to bytes
	var latestQRBytes []byte
	if latestRound.Notarization != nil {
		latestQRBytes = latestRound.Notarization.QC.Bytes()
	} else if latestRound.Finalization != nil {
		latestQRBytes = latestRound.Finalization.QC.Bytes()
	} else if latestRound.EmptyNotarization != nil {
		latestQRBytes = latestRound.EmptyNotarization.QC.Bytes()
	}

	return &p2p.BFT{
		ChainId: chainID[:],
		Message: &p2p.BFT_ReplicationResponse{
			ReplicationResponse: &p2p.ReplicationResponse{
				Data:        qrBytes,
				LatestRound: latestRound.GetRound(),
				LatestQr:    latestQRBytes,
			},
		},
	}, nil
}

func blockHeaderToP2P(bh simplex.BlockHeader) *p2p.BlockHeader {
	return &p2p.BlockHeader{
		BlockId:     bh.Digest[:],
		Round:       bh.Round,
		ParentRound: 0, // Set to 0 for now as we don't have parent round in BlockHeader
	}
}

func protocolMetadataToP2P(md simplex.ProtocolMetadata) *p2p.ProtocolMetadata {
	return &p2p.ProtocolMetadata{
		Round:      md.Round,
		ParentHash: md.Prev[:],
	}
}

func quorumRoundToP2P(qr *simplex.VerifiedQuorumRound) (*p2p.QuorumRound, error) {
	p2pQR := &p2p.QuorumRound{
		Round: qr.GetRound(),
	}

	// Get QC bytes from the appropriate field
	if qr.Notarization != nil {
		p2pQR.QuorumCertificate = qr.Notarization.QC.Bytes()
	} else if qr.Finalization != nil {
		p2pQR.QuorumCertificate = qr.Finalization.QC.Bytes()
	} else if qr.EmptyNotarization != nil {
		p2pQR.QuorumCertificate = qr.EmptyNotarization.QC.Bytes()
	}
	return p2pQR, nil
}
