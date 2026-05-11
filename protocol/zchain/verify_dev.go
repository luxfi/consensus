// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !production

package zchain

// requireBackendBinding is false in dev builds: VerifyZProofUnderProfile
// treats "no backend bound for this VerifierID" as a no-op success at
// check 15. Tests and envelope-only validation paths exercise checks
// 1-14 without requiring a real SP1 / RISC0 / P3Q backend.
//
// Production builds set this true via verify_production.go (build tag
// `production`), at which point the absence of a binding is a hard
// refusal.
const requireBackendBinding = false
