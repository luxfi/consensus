// Copyright (C) 2019-2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"encoding/json"
	"fmt"
)

// JSONByteSlice is a byte slice that marshals to JSON as a hex-encoded string
type JSONByteSlice []byte

// MarshalJSON marshals the byte slice to a hex-encoded string
func (b JSONByteSlice) MarshalJSON() ([]byte, error) {
	if len(b) == 0 {
		return json.Marshal(nil)
	}
	hexStr := fmt.Sprintf("0x%x", []byte(b))
	return json.Marshal(hexStr)
}

// UnmarshalJSON unmarshals a hex-encoded string to a byte slice
func (b *JSONByteSlice) UnmarshalJSON(data []byte) error {
	var hexStr string
	if err := json.Unmarshal(data, &hexStr); err != nil {
		return err
	}
	
	if hexStr == "" {
		*b = nil
		return nil
	}
	
	// Remove 0x prefix if present
	if len(hexStr) >= 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[2:]
	}
	
	// Decode hex string
	bytes := make([]byte, len(hexStr)/2)
	for i := 0; i < len(bytes); i++ {
		fmt.Sscanf(hexStr[i*2:i*2+2], "%02x", &bytes[i])
	}
	
	*b = bytes
	return nil
}