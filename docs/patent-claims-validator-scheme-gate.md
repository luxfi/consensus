# ValidatorSchemeID Cross-Axis NodeID Gate — Patent Claim Drafts (Attorney Review)

> **Internal working document.** Bundle #10 of the Lux PATENT-INVENTORY.
> Not a filed application; not a legal opinion.

## §0 Bundle summary

- **Title**: One-byte cryptographic-identity scheme identifier
  embedded in blockchain validator NodeIDs and enforced as a
  cross-axis check against the chain's security profile, with
  defense-in-depth refusal of classical schemes regardless of
  permissive operator flags.
- **Inventors**: Lux Industries cryptography team.
- **Priority date**: file as US provisional within 12 months.
- **Estimated claim count**: 10 (1 independent + 9 dependent).
- **Defensive-vs-offensive**: **Offensive.**

## §1 Background and prior art

1. **Ethereum NodeID** (devp2p, 2014-2026): node identifier derived
   from secp256k1 public key — single scheme.
2. **libp2p PeerID** (libp2p protocol, 2018-2026): multi-codec
   wrapping the public key bytes; supports multiple schemes
   (Ed25519, secp256k1, RSA, ECDSA) but does not enforce a
   chain-wide policy on which scheme is required.
3. **NIST PQ migration guidance** (NIST IR 8413): suggests scheme
   negotiation for transition; no concrete cross-axis enforcement.

Closest prior art is [2] libp2p PeerID multi-codec. Lux's
contribution:

- A **one-byte** `NodeIDScheme` wire field (e.g., `0x42` =
  ML-DSA-65, `0x90` = secp256k1) embedded in the NodeID itself.
- **Cross-axis check**: the chain's security profile is the source
  of truth; the profile reports which `NodeIDScheme` is accepted
  via `ChainSecurityProfile.ValidatorSchemeID()`.
- **Defense-in-depth refusal**: strict-PQ profiles refuse
  classical schemes (`0x90`) **regardless** of any operator
  permissive flag (e.g., `classicalCompatUnsafe`). This is the
  load-bearing invariant — operators cannot accidentally bypass
  the PQ requirement by setting a flag.
- A **typed error** (`ErrValidatorSchemeMismatch`) propagated
  through the wire-decode → peer-handshake → validator-set-
  membership chain, allowing every layer to refuse mismatched
  scheme.

## §2 Inventive concept

The chain's security profile defines:

```
ChainSecurityProfile.ValidatorSchemeID() -> SigSchemeID
ChainSecurityProfile.AcceptsValidatorScheme(
    presented SigSchemeID,
    classicalCompatUnsafe bool,
) bool
```

For a strict-PQ profile (e.g., `Pulsar`), `ValidatorSchemeID()`
returns `SigSchemeMLDSA65 = 0x42` and
`AcceptsValidatorScheme(presented, operatorFlag)` returns:
- `true` if `presented == 0x42`;
- `false` otherwise — including when `operatorFlag == true`.

The `peer.SchemeGate` is the node-side check that funnels every
inbound NodeID through `AcceptsValidatorScheme` and emits the
typed error on mismatch.

## §3 Independent claim (draft)

> **Claim 1.** A computer-implemented method for enforcing
> cryptographic-identity scheme conformance on a blockchain
> validator network, the method comprising:
>
> (a) encoding, in each validator's network identifier (NodeID),
>     a one-byte cryptographic-scheme identifier field naming the
>     cryptographic scheme under which the validator's public key
>     was generated, selected from at least: `0x42` for
>     FIPS 204 ML-DSA-65, `0x90` for secp256k1 ECDSA, `0xED` for
>     Ed25519 EdDSA, and additional reserved bytes for future
>     schemes;
>
> (b) defining, on the chain's security configuration, a
>     `ValidatorSchemeID` field naming the chain-wide required
>     scheme identifier, and an
>     `AcceptsValidatorScheme(presented, classicalCompatUnsafe)`
>     decision function returning whether a presented identifier
>     is acceptable on the chain;
>
> (c) configuring the chain's security configuration such that
>     for any chain marked strict-post-quantum, the
>     `AcceptsValidatorScheme` decision function returns false
>     for any classical scheme identifier `presented` regardless
>     of the value of `classicalCompatUnsafe`;
>
> (d) on every inbound peer connection or validator-set-
>     membership check, extracting the scheme identifier field
>     from the presented NodeID and invoking
>     `AcceptsValidatorScheme(presented, operator-flag)`;
>
> (e) on a false return from `AcceptsValidatorScheme`, refusing
>     the connection or membership and emitting a typed
>     `ErrValidatorSchemeMismatch` error to the calling layer;
>     and
>
> (f) on a true return, accepting the connection or membership
>     and proceeding with the chain protocol.

## §4 Dependent claims (drafts)

**Claim 2.** The method of claim 1, wherein the scheme
identifier `0x42` corresponds to FIPS 204 ML-DSA-65 and the
identifier `0x90` corresponds to secp256k1 ECDSA, with a fixed
wire-byte mapping shared across the validator network.

**Claim 3.** The method of claim 1, wherein strict-post-quantum
profiles enforce defense-in-depth refusal of classical schemes:
the `AcceptsValidatorScheme` function returns false for any
classical-cryptography identifier even when the operator has
set the permissive flag, preventing operator misconfiguration
from breaking the chain's PQ invariant.

**Claim 4.** The method of claim 1, wherein the typed
`ErrValidatorSchemeMismatch` error is shared between the wire-
decode layer (e.g., `ids.ErrNodeIDSchemeMismatch`) and the
peer-handshake layer (e.g., `config.ErrValidatorSchemeMismatch`),
ensuring a single canonical error type propagates through every
layer.

**Claim 5.** The method of claim 1, wherein the chain's
security configuration provides `IdentitySchemeID` as a one-byte
field directly readable by both the peer-gate logic and the
mempool / proposer / membership-check logic, eliminating
multiple sources of truth.

**Claim 6.** The method of claim 1, wherein the strict-post-
quantum profile additionally enforces that the validator's
identity public key has been verified to match the validator's
NodeID scheme bytes via a one-time challenge-response check at
peer-handshake time.

**Claim 7.** The method of claim 1, wherein the chain's security
profile is one of a plurality of named profiles (e.g., `Pulsar`,
`Aurora`, `Polaris`), each profile naming a unique
`ValidatorSchemeID` and a unique `AcceptsValidatorScheme`
decision function, allowing profile-based migration of scheme
policy without code changes.

**Claim 8.** The method of claim 1, further comprising
maintaining a continuous-integration test that pins the
literal-byte mapping between `ids.NodeIDScheme` (the wire enum)
and `config.SigSchemeID` (the security-profile enum) so that
any renumber in either trips the test and prevents merge.

**Claim 9.** The method of claim 1, wherein the
`AcceptsValidatorScheme` decision function is wired into the
node's wave-tick scheduler (e.g., the GPU EVM `quasar_wave`
service ring) such that scheme refusal is detected at the
earliest GPU-resident point of the inbound packet path.

**Claim 10.** A non-transitory computer-readable medium storing
the Go source code of `config.ValidatorSchemeID`,
`config.AcceptsValidatorScheme`,
`config.ErrValidatorSchemeMismatch`, and the corresponding
wire-decode layer's `ids.NodeIDScheme` enum.

## §5 Reference to implementation

- `~/work/lux/consensus/config/validator_scheme.go` (v1.23.4+
  per consensus CLAUDE.md).
- `~/work/lux/consensus/config/security_profile.go`.
- `~/work/lux/node/network/peer/scheme_gate.go` (SchemeGate).
- `~/work/lux/ids/node_id.go` (NodeIDScheme enum).

## §6 Defensive vs offensive

**OFFENSIVE.** The defense-in-depth refusal of classical schemes
under strict-PQ profiles regardless of operator flag is a
load-bearing security invariant; competitor PQ chains lacking
this gate will be measurably less secure.

---

**Document metadata**
- Path: `consensus/docs/patent-claims-validator-scheme-gate.md`
- Bundle: #10 of `lps/PATENT-INVENTORY.md`
- Created: 2026-05-19
