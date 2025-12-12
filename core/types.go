// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package core

// SendConfig configures message sending.
type SendConfig struct {
	NodeIDs       []interface{}
	Validators    int
	NonValidators int
	Peers         int
}
