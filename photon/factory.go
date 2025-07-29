// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import "github.com/luxfi/ids"

var PhotonFactory Factory = photonFactory{}

type photonFactory struct{}

func (photonFactory) NewPolyadic(params Parameters, choice ids.ID) Polyadic {
	return nil // Photon doesn't implement full consensus, just sampling
}

func (photonFactory) NewMonadic(params Parameters) Monadic {
	return nil // Photon doesn't implement full consensus, just sampling
}

