// Copyright (C) 2025, Lux Industries Inc. All rights reserved.

package quasar

import (
	"bytes"
	"encoding/gob"

	ringtailThreshold "github.com/luxfi/pulsar/threshold"
)

// ringtailGobEncode serializes a Ringtail threshold signature using gob.
// Returns nil on encoder error; callers treat that as "no signature available".
func ringtailGobEncode(sig *ringtailThreshold.Signature) []byte {
	if sig == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(sig); err != nil {
		return nil
	}
	return buf.Bytes()
}

// ringtailGobDecode is the inverse of ringtailGobEncode.
func ringtailGobDecode(data []byte) (*ringtailThreshold.Signature, error) {
	if len(data) == 0 {
		return nil, ErrCertCorrupt
	}
	out := &ringtailThreshold.Signature{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(out); err != nil {
		return nil, err
	}
	return out, nil
}
