// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build production

package zchain

// requireBackendBinding pins the fail-closed behaviour in production
// builds: VerifyZProofUnderProfile refuses every envelope whose
// VerifierID has no BackendVerifier bound to it. There is no dev-mode
// bypass on this path; the only way to verify a proof in production is
// to register a real backend.
const requireBackendBinding = true
