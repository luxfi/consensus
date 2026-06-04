// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package zap implements LP-182 — the single consensus wire for the new
// final Lux network. Activated at 2025-12-25T16:20:00-08:00 (unix
// 1766708400); the binary speaks ZAP and nothing else.
//
// One wire. The Go struct IS the wire. No marshal, no decode, no codec.
// Each schema in this package is a fixed offset map over a *zap.Message
// buffer. `Wrap*` returns a typed window onto a parsed buffer; `New*`
// writes fields into a buffer at known offsets; `Bytes()` returns the
// underlying buffer.
//
// Three layers, each one job (see LP-182):
//
//   - Transcript: TupleHash256 / cSHAKE256 with SP 800-185 framing —
//     unchanged, lives in protocol/quasar/round_digest.go and
//     protocol/quasar/qblock.go::TranscriptHash.
//   - Wire: ZAP buffer = struct = bytes. This package.
//   - Identity: sha256.Sum256 over the wire bytes. Lives in
//     pkg/wire/candidate.go, pkg/wire/credentials.go, and per-protocol
//     identity sites that hash whatever the wire layer presents.
package zap

// SchemaID is the LP-182 schema identifier. Every wire schema is a fixed
// offset map keyed by this single byte. The schema ID does NOT appear in
// the wire bytes — each protocol envelope already knows which schema it
// is carrying. The ID is the cross-implementation registry handle for
// `test/e2e/{python,cpp,c,rust}_node.go`.
type SchemaID uint8

// SchemaID registry. Single source of truth — cross-impl stubs read this
// file. The values are LP-182 §"Wire schemas" verbatim.
const (
	SchemaID_QuasarCert            SchemaID = 0x01
	SchemaID_QBlock                SchemaID = 0x02
	SchemaID_WitnessProof          SchemaID = 0x03
	SchemaID_MagnetarAggregateCert SchemaID = 0x04
	SchemaID_PolarisLegs           SchemaID = 0x05
	SchemaID_QuasarSig             SchemaID = 0x06
	SchemaID_EpochBundle           SchemaID = 0x07
	SchemaID_PrismCut              SchemaID = 0x08
	SchemaID_StakeWeightedCut      SchemaID = 0x09
	SchemaID_TxAuthEnvelope        SchemaID = 0x0A
	SchemaID_PQPermit              SchemaID = 0x0B
	SchemaID_DAGVertex             SchemaID = 0x0C
	SchemaID_PolicyCert            SchemaID = 0x0D
)

// SchemaName returns the LP-182 type name for the schema ID. Used by
// cross-impl stubs and by debug logging. Returns "" for unknown IDs.
func SchemaName(id SchemaID) string {
	switch id {
	case SchemaID_QuasarCert:
		return "QuasarCert"
	case SchemaID_QBlock:
		return "QBlock"
	case SchemaID_WitnessProof:
		return "WitnessProof"
	case SchemaID_MagnetarAggregateCert:
		return "MagnetarAggregateCert"
	case SchemaID_PolarisLegs:
		return "PolarisLegs"
	case SchemaID_QuasarSig:
		return "QuasarSig"
	case SchemaID_EpochBundle:
		return "EpochBundle"
	case SchemaID_PrismCut:
		return "PrismCut"
	case SchemaID_StakeWeightedCut:
		return "StakeWeightedCut"
	case SchemaID_TxAuthEnvelope:
		return "TxAuthEnvelope"
	case SchemaID_PQPermit:
		return "PQPermit"
	case SchemaID_DAGVertex:
		return "DAGVertex"
	case SchemaID_PolicyCert:
		return "PolicyCert"
	default:
		return ""
	}
}

// AllSchemas returns every LP-182 schema ID in ID order. Cross-impl
// stubs iterate this slice to generate accessor code for each schema.
func AllSchemas() []SchemaID {
	return []SchemaID{
		SchemaID_QuasarCert,
		SchemaID_QBlock,
		SchemaID_WitnessProof,
		SchemaID_MagnetarAggregateCert,
		SchemaID_PolarisLegs,
		SchemaID_QuasarSig,
		SchemaID_EpochBundle,
		SchemaID_PrismCut,
		SchemaID_StakeWeightedCut,
		SchemaID_TxAuthEnvelope,
		SchemaID_PQPermit,
		SchemaID_DAGVertex,
		SchemaID_PolicyCert,
	}
}
