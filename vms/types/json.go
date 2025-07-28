// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"encoding/json"
)

// JSONByteSlice is a byte slice that marshals to JSON as a hex string
type JSONByteSlice []byte

// MarshalJSON marshals bytes to hex string
func (b JSONByteSlice) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// String returns the hex representation
func (b JSONByteSlice) String() string {
	return "0x" + bytesToHex(b)
}

// bytesToHex converts bytes to hex string
func bytesToHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hexChars[v>>4]
		result[i*2+1] = hexChars[v&0x0f]
	}
	return string(result)
}