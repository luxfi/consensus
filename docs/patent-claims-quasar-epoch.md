# Quasar Epoch-Key Lifecycle — Patent Claim Drafts (Attorney Review)

> **Internal working document.** Bundle #6 of the Lux PATENT-INVENTORY.
> Not a filed application; not a legal opinion.

## §0 Bundle summary

- **Title**: Blockchain epoch-key lifecycle management combining rate-
  limited share rotation, validator-set-change detection, persistent
  group-key preservation across rotations within a key era, and an
  activation circuit-breaker that gates new committee acceptance on
  successful threshold signing under the unchanged group key.
- **Inventors**: Lux Industries cryptography team.
- **Priority date**: file as US provisional within 12 months.
- **Estimated claim count**: 17 (2 independent + 15 dependent).
- **Defensive-vs-offensive**: **Offensive**.

## §1 Background and prior art

1. **HJKY97 proactive secret sharing** (Herzberg-Jakobsson-Jarecki-
   Krawczyk-Yung, CRYPTO 1995/1997): classical proactive share
   refresh.
2. **Desmedt-Jajodia 1997 redistribution** (Desmedt-Jajodia 1997):
   share redistribution between disjoint committees.
3. **Wong-Wang-Wing 2002 verifiable secret redistribution**: VSR
   for committee changes.
4. **Lindell-Nof CCS 2018 fast secure multiparty signing**: refreshable
   ECDSA threshold signing.
5. **FROST Round-2 refresh** (Komlo-Goldberg, RFC 9591): per-session
   nonces but no committee rotation.
6. **CGGMP21 reshare** (Canetti-Gennaro-Goldfeder-Makriyannis-Peled,
   2021): proactive ECDSA threshold signing.

Closest prior art is [1] HJKY97 and [3] Wong-Wang-Wing 2002 for
share redistribution. Lux's novel composition:

- The **KeyEra abstraction**: a persistent group-key lineage that
  survives every validator-set rotation; only the share
  distribution rotates. Each Quasar emission ("pulse") signs under
  the unchanged GroupKey.
- The **Reanchor** governance event that opens a new key era,
  rotating ALL of `(A, s, e, bTilde)` — distinct from a routine
  Reshare.
- The **activation circuit-breaker**: the new committee is only
  accepted into consensus after successfully threshold-signing a
  canonical block under the unchanged GroupKey, preventing
  acceptance of a broken share distribution.
- The **rate-limited keygen**: minimum 10-minute spacing between
  rotations with maximum 1-hour-force-rotation, validating that
  quantum-progress against the key is invalidated by each rotation.
- The **quantum checkpoint interval**: 3-second PQ-signature
  emission within the otherwise 10-minute share-rotation cadence.

## §2 Inventive concept

A blockchain consensus protocol uses an `EpochManager` that:

1. **Bootstraps** the first key era at chain genesis via a one-time
   MPC ceremony. The trusted dealer is only used at genesis of the
   key era; subsequent rotations are dealerless.
2. **Reshares** the share distribution every time the validator set
   changes, with a minimum interval (rate limit). The Group public
   key `(A, bTilde)` and master signing secret `s` are preserved.
3. **Reanchors** the key era as a rare governance event — rotating
   ALL of `(A, s, e, bTilde)` to a fresh era. This is the only
   path to changing the persistent group key.
4. **Activation-gates** every new committee on a circuit-breaker:
   the new committee must successfully threshold-sign a canonical
   activation block under the unchanged GroupKey within a
   configured timeout. If activation fails, the new committee is
   rejected and the chain stays on the previous committee.
5. **Quantum checkpoints** are emitted every 3 seconds within the
   share-rotation period of 10 minutes, providing frequent PQ-
   safe anchors.

## §3 Independent claims (drafts)

### Claim 1 (epoch-key lifecycle claim, draft)

> **Claim 1.** A computer-implemented method for managing the
> cryptographic key lifecycle of a blockchain consensus
> committee, the method comprising:
>
> (a) at chain genesis, executing a one-time multi-party
>     computation ceremony to produce a group public key `(A,
>     bTilde)` over a Module-Lattice or Ring-Lattice problem and
>     a corresponding share distribution `{s_i}_{i ∈ Committee_0}`
>     such that the Lagrange combination of the shares is a
>     centralized signing secret `s`, said group public key and
>     share distribution forming a first key era `Era_0`;
>
> (b) on every detected validator-set transition from
>     `Committee_k` to `Committee_{k+1}`, executing a dealerless
>     share-redistribution protocol that preserves the group
>     public key `(A, bTilde)` and the centralized signing secret
>     `s` while producing a new share distribution
>     `{s_i}_{i ∈ Committee_{k+1}}` over the new committee;
>
> (c) gating the acceptance of `Committee_{k+1}` into consensus
>     on the successful threshold signing by `Committee_{k+1}` of
>     a canonical activation transcript under the unchanged group
>     public key `(A, bTilde)` within a configured timeout,
>     reverting to `Committee_k` if activation fails;
>
> (d) emitting, every period `T_quantum` (e.g. 3 seconds), a
>     threshold signature under `(A, bTilde)` by the currently
>     active committee, said signature being the per-block
>     finality artifact of the consensus protocol;
>
> (e) on a governance-gated reanchor event, opening a new key era
>     `Era_{k+1}` by executing a fresh multi-party computation
>     ceremony to produce a new group public key `(A', bTilde')`
>     and a new share distribution, with the previous era's group
>     public key archived for historical verification.

### Claim 2 (rate-limit + force-rotate claim, draft)

> **Claim 2.** The method of claim 1, wherein the validator-set
> transitions of step (b) are rate-limited and force-rotated, the
> rate-limiting comprising:
>
> (a) recording, on the chain or in a node-local registry, the
>     wall-clock timestamp of each share-redistribution event;
>
> (b) refusing any new share-redistribution event whose triggering
>     validator-set transition arrives within a minimum interval
>     `T_min` (e.g. 10 minutes) of the previously-completed share-
>     redistribution event, returning a `rate-limited` typed
>     error to the caller;
>
> (c) automatically initiating a share-redistribution event when
>     `T_max` (e.g. 1 hour, equivalently `6 × T_min`) has elapsed
>     since the last share-redistribution event, even if no
>     validator-set transition has been detected, to bound the
>     cryptanalytic progress an adversary can accumulate against
>     the current share distribution.

## §4 Dependent claims (drafts)

**Claim 3.** The method of claim 1, wherein the dealerless
share-redistribution protocol of step (b) is an implementation
of the proactive secret-sharing protocol of Herzberg-Jakobsson-
Jarecki-Krawczyk-Yung (HJKY97), adapted to the Module-Lattice
ring `R_q` for `q` a NIST-PQ-suitable prime.

**Claim 4.** The method of claim 1, wherein the activation
transcript of step (c) is the SHAKE256 cSHAKE digest of a
canonical activation block, with customization tag identifying
the key era and the activating committee.

**Claim 5.** The method of claim 1, wherein the activation
circuit-breaker is implemented as a verifier check
`VerifyActivation(σ_threshold, transcript, GroupKey, threshold)`
that returns success only if the threshold signature verifies
under the unchanged group public key.

**Claim 6.** The method of claim 1, wherein the multi-party
computation ceremony of step (a) is confined to chain genesis
and the rare governance-gated reanchor of step (e); routine
validator-set transitions of step (b) do NOT require a trusted
dealer.

**Claim 7.** The method of claim 1, wherein the period
`T_quantum` is configurable per chain via a consensus parameter
and defaults to 3 seconds for a validator set of 21 validators.

**Claim 8.** The method of claim 2, wherein the minimum
interval `T_min` is configurable per chain and defaults to 10
minutes, and wherein the maximum interval `T_max` is 6 × T_min.

**Claim 9.** The method of claim 1, further comprising
maintaining a history of the last `H` (e.g. 6) share
distributions to support cross-rotation verification of
historical signatures during chain re-orgs or audit operations.

**Claim 10.** The method of claim 1, wherein the validator-set
transition detection of step (b) is performed by computing a
canonical hash of the validator set and comparing against the
previously-recorded hash, with any difference triggering the
share-redistribution.

**Claim 11.** The method of claim 1, wherein the group public
key `(A, bTilde)` is bound into the validator-set Merkle root
committed in each block header, providing chain-state-rooted
authentication of the key era.

**Claim 12.** The method of claim 1, wherein the reanchor event
of step (e) is gated by a governance protocol requiring at least
a supermajority of the active committee plus a time-locked
delay to prevent unauthorized key-era rotation.

**Claim 13.** The method of claim 1, wherein the share-
redistribution protocol of step (b) produces an
identifiable-abort evidence blob in the event of a malicious
contributor, said evidence verifiable by any third party for
slashing.

**Claim 14.** The method of claim 1, wherein the group public
key `(A, bTilde)` is preserved as the BLS aggregate public key
of the validator set in a hybrid mode, allowing classical BLS
verification of certificates emitted under the same key era
alongside the threshold PQ signature.

**Claim 15.** The method of claim 1, wherein the share-
redistribution protocol of step (b) is run by a distinguished
"signature coordinator" role rotated per consensus round via a
verifiable random function over the active committee.

**Claim 16.** A consensus protocol comprising the method of
claim 1 and configured to participate in a Byzantine-fault-
tolerant blockchain network of `n ≥ 3f + 1` validators, where
`f` is the maximum tolerated fault count.

**Claim 17.** A non-transitory computer-readable medium storing
the source code of the `EpochManager` of claim 1, the
`VerifyActivation` checker of claim 5, and a state machine
implementing claims 1 and 2.

## §5 Reference to implementation

- `~/work/lux/consensus/protocol/quasar/epoch.go`
  (`EpochManager`, ~1500 lines).
- `~/work/lux/consensus/protocol/quasar/reshare_epoch.go`.
- `~/work/lux/corona/keyera/keyera.go`,
  `~/work/lux/lens/keyera/keyera.go` (curve sibling).
- `~/work/lux/threshold/protocols/lss/` (LSS DynamicReshare driver).
- `~/work/lux/pulsar/DESIGN.md` (single source of truth).

## §6 Defensive vs offensive

**OFFENSIVE.** The integrated lifecycle — KeyEra + Reshare +
Reanchor + activation circuit-breaker + rate-limit + 3s quantum
checkpoint — is novel as an end-to-end protocol; HJKY97 alone
covers only the primitive.

---

**Document metadata**
- Path: `consensus/docs/patent-claims-quasar-epoch.md`
- Bundle: #6 of `lps/PATENT-INVENTORY.md`
- Created: 2026-05-19
