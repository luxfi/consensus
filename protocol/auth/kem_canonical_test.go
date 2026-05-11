// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth_test

import (
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/auth"
)

// TestKEMSchemeIDs_AllUseCanonicalNumbering confirms that every package
// that names ML-KEM resolves to the same byte values as
// config.KeyExchangeID. This is the structural guarantee that the
// three-way drift named in Bug 3 of the spec/code alignment audit
// (auth=0x52/0x53, config=0x01/0x02, node/kem=0x62/0x63) cannot recur.
//
// Aliasing rule: each package that exports KEM constants does so by
// re-exporting config.KeyExchange* directly. The test below proves
// auth's constants are byte-identical to config's. The node/network/kem
// package is tested in its own package; both tests anchor the same
// invariant.
func TestKEMSchemeIDs_AllUseCanonicalNumbering(t *testing.T) {
	cases := []struct {
		name    string
		auth    auth.KeyExchangeID
		canon   config.KeyExchangeID
		wantHex uint8
	}{
		{"ml-kem-768", auth.KeyExchangeMLKEM768, config.KeyExchangeMLKEM768, 0x01},
		{"ml-kem-1024", auth.KeyExchangeMLKEM1024, config.KeyExchangeMLKEM1024, 0x02},
		{"x25519-unsafe", auth.KeyExchangeX25519Unsafe, config.KeyExchangeX25519Unsafe, 0x90},
		{"invalid", auth.KeyExchangeInvalid, config.KeyExchangeInvalid, 0x00},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if auth.KeyExchangeID(c.auth) != c.canon {
				t.Errorf("auth.%s and config.%s diverge: auth=0x%02x config=0x%02x",
					c.name, c.name, uint8(c.auth), uint8(c.canon))
			}
			if uint8(c.auth) != c.wantHex {
				t.Errorf("auth.%s wire byte = 0x%02x, want 0x%02x", c.name, uint8(c.auth), c.wantHex)
			}
			if uint8(c.canon) != c.wantHex {
				t.Errorf("config.%s wire byte = 0x%02x, want 0x%02x", c.name, uint8(c.canon), c.wantHex)
			}
		})
	}
}
