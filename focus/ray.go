// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package focus implements confidence building with ray reduction (Snowball/FPC-friendly)
package focus

import "github.com/luxfi/consensus/photon"

type Step[ID comparable] struct {
	Prefer bool
	Conf   uint32
}

type Params struct {
	Alpha float64 // success threshold (e.g., 0.8)
}

func Apply[ID comparable](samples []photon.Photon[ID], prev Step[ID], p Params) Step[ID] {
	if len(samples) == 0 {
		return prev
	}
	var yes int
	for _, ph := range samples {
		if ph.Prefer {
			yes++
		}
	}
	ratio := float64(yes) / float64(len(samples))
	next := prev
	if ratio >= p.Alpha {
		if prev.Prefer {
			next.Conf++
		} else {
			next.Prefer = true
			next.Conf = 1
		}
	} else if ratio <= (1.0 - p.Alpha) {
		if !prev.Prefer {
			next.Conf++
		} else {
			next.Prefer = false
			next.Conf = 1
		}
	} else {
		// inconclusive; keep state
	}
	return next
}