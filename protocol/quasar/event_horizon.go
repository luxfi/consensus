// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Event Horizon - Helper functions and utilities for the Quasar consensus

package quasar

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// computeBlockHash creates a quantum-resistant block hash
func computeBlockHash(block *QBlock) string {
	data := fmt.Sprintf("%s:%d:%d",
		block.Hash,
		block.Height,
		block.Timestamp.Unix())

	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
