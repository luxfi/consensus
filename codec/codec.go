// Package codec provides encoding/decoding for consensus types
package codec

import (
	"encoding/json"
	"fmt"
)

// CodecVersion represents the codec version
type CodecVersion uint16

const (
	// CurrentVersion is the current codec version
	CurrentVersion CodecVersion = 0
)

// Codec provides marshaling/unmarshaling
var Codec = &JSONCodec{}

// JSONCodec implements JSON encoding/decoding
type JSONCodec struct{}

// Marshal marshals an object to bytes
func (c *JSONCodec) Marshal(version CodecVersion, v interface{}) ([]byte, error) {
	if version != CurrentVersion {
		return nil, fmt.Errorf("unsupported codec version: %d", version)
	}
	return json.Marshal(v)
}

// Unmarshal unmarshals bytes to an object
func (c *JSONCodec) Unmarshal(data []byte, v interface{}) (CodecVersion, error) {
	err := json.Unmarshal(data, v)
	return CurrentVersion, err
}