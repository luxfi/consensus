// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.

package quasar

import (
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// coronaGobEncode serializes a Corona threshold signature via the
// canonical CORS wire codec exposed by corona/threshold.Signature.
// The legacy name is preserved for caller compatibility; the encoder
// is now the v0.7.6 wire codec (4-byte magic "CORS" + 2-byte version
// + length-prefixed ring polynomial bytes). The receiver verifies
// with coronaThreshold.Verify after decoding through coronaGobDecode.
//
// Returns nil on encoder error; callers treat nil as "no signature
// available" and the round driver falls back to the next-lower
// witness level.
func coronaGobEncode(sig *coronaThreshold.Signature) []byte {
	if sig == nil {
		return nil
	}
	out, err := sig.MarshalBinary()
	if err != nil {
		return nil
	}
	return out
}

// coronaGobDecode is the inverse of coronaGobEncode. Strict
// trailing-bytes policy is enforced by the underlying CORS wire
// codec.
func coronaGobDecode(data []byte) (*coronaThreshold.Signature, error) {
	if len(data) == 0 {
		return nil, ErrCertCorrupt
	}
	out := &coronaThreshold.Signature{}
	if err := out.UnmarshalBinary(data); err != nil {
		return nil, ErrCertCorrupt
	}
	return out, nil
}
