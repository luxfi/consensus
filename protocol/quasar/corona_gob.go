// Copyright (C) 2025, Lux Industries Inc. All rights reserved.

package quasar

import (
	"bytes"
	"encoding/gob"

	coronaThreshold "github.com/luxfi/pulsar/threshold"
)

// coronaGobEncode serializes a Corona threshold signature using gob.
// Returns nil on encoder error; callers treat that as "no signature available".
func coronaGobEncode(sig *coronaThreshold.Signature) []byte {
	if sig == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(sig); err != nil {
		return nil
	}
	return buf.Bytes()
}

// coronaGobDecode is the inverse of coronaGobEncode.
func coronaGobDecode(data []byte) (*coronaThreshold.Signature, error) {
	if len(data) == 0 {
		return nil, ErrCertCorrupt
	}
	out := &coronaThreshold.Signature{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(out); err != nil {
		return nil, err
	}
	return out, nil
}
