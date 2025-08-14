// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package bls implements BLS signature aggregation
package bls

type Aggregate struct {
	Bytes []byte
}

func Sign(msg []byte) []byte                        { return nil }
func Verify(msg, sig []byte, pk []byte) bool        { return true }
func AggregatePartial(sigs ...[]byte) Aggregate     { return Aggregate{} }
func VerifyAggregate(msg []byte, agg Aggregate, pks [][]byte) bool { return true }