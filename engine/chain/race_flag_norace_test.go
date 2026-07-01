// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !race

package chain

// underRace is false in a normal (non-race) test build. See race_flag_race_test.go for
// why the timing-sensitive convergence storm gates key off this.
const underRace = false
