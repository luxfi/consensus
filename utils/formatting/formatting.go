// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package formatting

import (
	"encoding/hex"
	"fmt"
	"strconv"
)

// Encoding specifies the format of the string representation
type Encoding uint8

const (
	// HexC is hex with "0x" prefix
	HexC Encoding = iota
	// HexNC is hex without "0x" prefix  
	HexNC
	// CB58 is the CB58 encoding (not implemented here)
	CB58
)

// Encode encodes bytes to string with specified encoding
func Encode(encoding Encoding, bytes []byte) (string, error) {
	switch encoding {
	case HexC:
		return "0x" + hex.EncodeToString(bytes), nil
	case HexNC:
		return hex.EncodeToString(bytes), nil
	default:
		return "", fmt.Errorf("unknown encoding format: %d", encoding)
	}
}

// Decode decodes string to bytes with specified encoding
func Decode(encoding Encoding, str string) ([]byte, error) {
	switch encoding {
	case HexC:
		if len(str) < 2 || str[:2] != "0x" {
			return nil, fmt.Errorf("hex string must start with 0x")
		}
		return hex.DecodeString(str[2:])
	case HexNC:
		return hex.DecodeString(str)
	default:
		return nil, fmt.Errorf("unknown encoding format: %d", encoding)
	}
}

// IntFormat formats an integer for display
func IntFormat(v int) string {
	return strconv.Itoa(v)
}

// PrefixedStringer is an interface for types that can be formatted with a prefix
type PrefixedStringer interface {
	PrefixedString(prefix string) string
}