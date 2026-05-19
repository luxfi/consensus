# Quasar Certificate — Patent Claim Drafts (Attorney Review)

> **Internal working document.** Bundle #1 of the Lux PATENT-INVENTORY.
> Not a filed application; not a legal opinion.

## §0 Bundle summary

- **Title**: Method and system for a single blockchain finality
  certificate composed of multiple independently-verifiable
  signatures over disjoint hardness assumptions, evaluated in
  parallel by validator participants.
- **Inventors**: Lux Industries cryptography team (specific
  inventorship to be assigned per commit-history audit on
  `~/work/lux/consensus/protocol/quasar/` and `~/work/lux/quasar`).
- **Priority date**: file as US provisional within **90 days** of
  this document (target by 2026-08-19) to lock priority before any
  Q-Chain mainnet announcement.
- **Filing class**: software (cryptographic protocol); class 380
  (cryptography) + 713 (electrical computers) USPC; G06F 21/60 IPC.
- **Estimated claim count**: 22 claims (3 independent + 19 dependent).
- **Defensive-vs-offensive**: **Offensive**. This is the headline
  Quasar IP — defensible against any party shipping a multi-hardness
  consensus certificate.

## §1 Background and prior art

Prior-art constellation:

1. **BLS aggregate signatures** (Boneh-Lynn-Shacham 2001; Boneh-Drijvers-
   Neven 2018 BDN aggregation; Ethereum 2.0 BLS12-381 aggregation): single-
   hardness (discrete log on BLS12-381), aggregate signature for many
   signers. The Lux fast-path BLS lane is this prior art unmodified.
2. **Threshold ML-DSA** (academic literature: Doerner-Kondi-Lee-Shelat
   2023 ePrint 2023/078 attempts; Pulsar ePrint 2026 Lux submission;
   Bandersnatch-FROST hybrids 2024): single-hardness (Module-LWE),
   threshold signature with FROST-style aggregation.
3. **Hybrid classical+PQ signatures** (Bindel-Brendel-Fischlin-Goncalves-
   Stebila 2017 "Transitioning to a Quantum-Safe Internet"; Stebila-
   Mosca 2016; Bos-Costello-Naehrig 2014 hybrid TLS; IETF draft-ietf-pquip-
   hybrid-signature-spectrums): TWO independent signatures (classical +
   PQ), but each individually verifiable; not a composed single
   certificate; not threshold; not multi-PQ.
4. **Dual-curve signatures** (Cremers-Düzlü-Hülsing-Vasconcellos 2022
   ePrint 2022/780): two-signature scheme with two different elliptic
   curves; not multi-hardness across discrete-log vs lattice.
5. **Ethereum 2.0 BLS attestation aggregation** (Buterin et al.): single-
   hardness, no PQ lane.
6. **Filecoin BLS+Groth16 anchor proofs**: BLS for finality + Groth16 for
   storage proofs — two signatures for different purposes, not
   parallel hardness on the same finality decision.

Closest prior art that touches the Lux claim:
- Reference [3] (hybrid sig spectrum) and IETF draft-ietf-pquip-hybrid-
  signature-spectrums. The Lux claim is **distinct** because (a) it
  composes THREE independent hardness assumptions, not two; (b) it is
  a **single certificate** for a single consensus decision, not two
  independent signatures stapled at the application layer; (c) the
  PQ lanes are threshold-aggregated and one of them (Pulsar) is
  byte-equal to a FIPS-conformant verifier, not "PQ as a tacked-on
  extra signature".

## §2 Inventive concept (plain English)

A blockchain consensus protocol emits **one certificate per accepted
block** that combines:

1. A BLS12-381 aggregate signature over the message-context binder
   (classical hardness: ECDLP / co-CDH).
2. A threshold Module-LWE signature (Pulsar) byte-equal to a single-
   party FIPS 204 ML-DSA-65 signature on the SAME message-context
   binder, produced by an unmodified FIPS verifier (post-quantum
   hardness: M-LWE + M-SIS).
3. A Groth16 proof rolling up N per-validator FIPS 204 ML-DSA-65
   identity signatures into a single ~192-byte proof (post-quantum
   hardness: M-LWE; aggregation hardness: discrete log on BLS12-381
   via the Groth16 SRS, providing a second classical layer of bind).
4. (optional fourth lane) A threshold Ring-LWE signature (Corona)
   over the same binder (post-quantum hardness: R-LWE), distinct
   from the M-LWE assumption underlying lane #2.

All four lanes are produced in PARALLEL by each participating
validator (independent thread/goroutine/GPU stream), and combined
into one certificate at the aggregator. An adversary must break **all
hardness assumptions in the active lanes** to forge a certificate.

The composition is **orthogonal**: lanes can be independently enabled
or disabled via a `PQMode` runtime selector (BLS-only / BLS+MLDSA /
BLS+Corona / TripleQuantum / +Z-Chain-rollup), with each mode using
a different subset of lanes against the same wire format.

## §3 Independent claims (drafts)

### Claim 1 (system claim, draft)

> **Claim 1.** A computer-implemented method for producing a finality
> certificate for an accepted blockchain block, the method comprising:
>
> (a) determining, by each of `n` validator nodes participating in
>     consensus for the block, a canonical message-context binder
>     `m` representing the block header, state root, validator set
>     commitment, and round identifier, said binder being a fixed-
>     length byte string committed to by every validator before any
>     signature is produced;
>
> (b) computing, in parallel by each validator node `i`:
>
>     (b1) a BLS12-381 signature `σ_BLS,i = sign_BLS(sk_BLS,i, m)`
>          where `sk_BLS,i` is the validator's BLS12-381 secret key
>          and the resulting signature lies in the BLS12-381 G1
>          subgroup;
>
>     (b2) a partial threshold signature contribution
>          `σ_M-LWE,i = pulsar_partial(s_M-LWE,i, m, ctx)` where
>          `s_M-LWE,i` is the validator's Shamir share of a single
>          centralized FIPS 204 ML-DSA-65 secret key and `ctx` is a
>          FIPS 204 §5.4.1 ExternalMu context binder;
>
>     (b3) a FIPS 204 ML-DSA-65 single-party signature
>          `σ_ID,i = sign_MLDSA(sk_ID,i, m)` over the validator's
>          long-term identity ML-DSA secret key `sk_ID,i`;
>
> (c) aggregating, by an aggregator node, the per-validator outputs
>     of (b) into a single certificate `Cert` comprising:
>
>     (c1) a BLS12-381 aggregate `σ_BLS = Σ_i σ_BLS,i` over the
>          subset of validators whose partial signatures are
>          received;
>
>     (c2) a single threshold FIPS 204 ML-DSA-65 signature `σ_M-LWE`
>          formed by Lagrange-aggregating the per-validator partial
>          contributions from (b2), said threshold signature being
>          byte-identical to a single-party FIPS 204 ML-DSA-65
>          signature on the Lagrange-reconstructed centralized
>          secret key and verifiable by any FIPS 204 conformant
>          verifier without modification;
>
>     (c3) a Groth16 zero-knowledge proof `π_ID` verifying the
>          batch-verification predicate that every per-validator
>          signature `σ_ID,i` from (b3) is a valid FIPS 204 ML-DSA-65
>          signature under the corresponding validator's published
>          identity public key, said Groth16 proof being of fixed
>          size independent of `n` and produced over a Groth16
>          arithmetic circuit instantiated on the BLS12-381 curve;
>
> (d) emitting, by the aggregator node, the certificate `Cert =
>     (σ_BLS, σ_M-LWE, π_ID, epoch, n_signers, m_digest)` as the
>     finality artifact for the block,
>
> wherein verification of `Cert` requires evaluating (i) the BLS12-
> 381 pairing verification of `σ_BLS`, (ii) the unmodified FIPS 204
> ML-DSA-65 verifier on `σ_M-LWE`, and (iii) the Groth16 verifier on
> `π_ID`, and wherein an adversary producing a forged `Cert` must
> simultaneously violate (i) the BLS12-381 co-CDH assumption, (ii)
> the Module-LWE / Module-SIS assumptions underlying FIPS 204
> ML-DSA-65, and (iii) the discrete-log assumption on the Groth16
> BLS12-381 SRS — each of which is independently believed
> computationally infeasible under its respective threat model
> (classical for (i), post-quantum for (ii), classical for (iii)).

### Claim 2 (verifier claim, draft)

> **Claim 2.** A computer-implemented method for verifying the
> finality certificate of claim 1, the method comprising, at any
> verifier node:
>
> (a) parsing the certificate `Cert = (σ_BLS, σ_M-LWE, π_ID, epoch,
>     n_signers, m_digest)` into its three signature lanes;
>
> (b) reconstructing the message-context binder `m` from the block
>     header bound to the certificate by `m_digest`;
>
> (c) verifying `σ_BLS` against the BLS aggregate public key
>     committed to in the validator-set Merkle root of the epoch
>     under the BLS12-381 co-CDH pairing equation;
>
> (d) verifying `σ_M-LWE` against the centralized FIPS 204
>     ML-DSA-65 group public key committed to in the same
>     validator-set Merkle root, using the FIPS 204 §6.3 verifier
>     **unmodified**;
>
> (e) verifying the Groth16 proof `π_ID` against the published
>     verification key of the Groth16 circuit whose statement
>     batch-verifies `n_signers` FIPS 204 ML-DSA-65 signatures under
>     the validator identity public keys committed to in the same
>     validator-set Merkle root;
>
> (f) accepting the certificate as finalizing the block if and only
>     if all three of steps (c), (d), and (e) succeed; rejecting
>     otherwise.

### Claim 3 (PQ-mode selector claim, draft)

> **Claim 3.** A computer-implemented method for selecting, at
> runtime, the cryptographic posture of the certificate of claim 1
> from a finite set of named modes, the method comprising:
>
> (a) storing a profile selector `mode ∈ {BLSOnly, BLSPlusMLDSA,
>     BLSPlusCorona, TripleQuantum, ...}` on the consensus
>     configuration of each validator node, said selector
>     determining which subset of the certificate's signature
>     lanes are produced and required;
>
> (b) on entering a consensus round, querying the selector and:
>
>     (b1) if `mode == BLSOnly`, executing only step (b1) of claim 1
>          and emitting a certificate carrying only `σ_BLS`;
>
>     (b2) if `mode == BLSPlusMLDSA`, executing steps (b1) and (b3)
>          of claim 1 and emitting `(σ_BLS, π_ID)`;
>
>     (b3) if `mode == BLSPlusCorona`, executing steps (b1) and
>          the Ring-LWE-threshold variant of (b2) of claim 1 and
>          emitting `(σ_BLS, σ_R-LWE)`;
>
>     (b4) if `mode == TripleQuantum`, executing all of (b1), (b2),
>          and (b3) of claim 1 and emitting the full
>          `(σ_BLS, σ_M-LWE, π_ID)` triple;
>
> (c) verifying certificates produced under one mode using only the
>     subset of verifiers corresponding to that mode, allowing
>     graceful migration from `BLSOnly` to `TripleQuantum` over
>     successive validator-set rotations without breaking
>     verification compatibility.

## §4 Dependent claims (drafts)

**Claim 4.** The method of claim 1, wherein the message-context
binder `m` is bound by SHAKE256 cSHAKE customization tag
`"lux-quasar-cert-v1"` per SP 800-185, ensuring domain separation
between Lux certificates and any other application using the same
underlying primitives.

**Claim 5.** The method of claim 1, wherein the Lagrange
aggregation of step (c2) is performed coordinate-wise on the
polynomial vectors comprising the FIPS 204 ML-DSA-65 response
vector `z`, hint vector `h`, and challenge `c_tilde`, such that
the aggregated signature lies in the same byte-encoded format as
a single-party FIPS 204 sigEncode output.

**Claim 6.** The method of claim 1, wherein the Groth16 circuit
of step (c3) shares the public matrix `A` (FIPS 204 §3.5.3
ExpandA expansion of the validator-set ML-DSA-65 seed) across all
`n` per-validator verification sub-circuits, yielding an amortized
constraint count below `n × 2^22.5` for `n ≥ 21` validators.

**Claim 7.** The method of claim 6, wherein the Groth16 SRS is
produced by a multi-party computation ceremony with at least one
honest participant providing a contribution before the SRS is
finalized for use in `π_ID` production.

**Claim 8.** The method of claim 1, wherein each validator's
contributions in step (b) are produced under a context binder
that includes the Lux NodeIDScheme byte identifying the
cryptographic identity scheme (ML-DSA-65 = 0x42, secp256k1 = 0x90)
under which the validator participates, ensuring scheme-typed
NodeIDs are bound into the finality transcript.

**Claim 9.** The method of claim 1, further comprising producing
a **fourth** signature lane comprising a threshold Ring-LWE
signature `σ_R-LWE` over the same message-context binder `m` and
appending `σ_R-LWE` to the certificate, wherein verification of
`σ_R-LWE` reduces to the Ring-LWE / Module-LWE assumption (distinct
from the M-LWE assumption underlying lane (c2)) and thereby
provides defense-in-depth against any future cryptanalytic
breakthrough specific to the M-LWE assumption.

**Claim 10.** The method of claim 1, wherein the threshold for
the Lagrange aggregation of step (c2) is `t = 2f + 1` where `f`
is the maximum tolerated number of Byzantine-faulty validators
per the consensus protocol's safety parameter, and wherein
recovery of the threshold M-LWE signature requires receipt of
partial contributions from at least `t` distinct validators.

**Claim 11.** The method of claim 1, wherein the per-validator
partial signing in step (b2) is performed in two rounds: a
commitment round in which each validator broadcasts the high-
bits decomposition `w1_i` of its mask-multiplied matrix product
`A · y_i`, followed by a response round in which each validator
broadcasts the per-party response `z_i = y_i + c · s_i` after the
Lagrange-aggregated challenge `c` has been computed by the
aggregator from the Lagrange-aggregated `w1`.

**Claim 12.** The method of claim 1, wherein the per-validator
identity signature of step (b3) and the per-validator partial
threshold contribution of step (b2) are produced using DISTINCT
ML-DSA-65 secret keys: the threshold lane uses a share of a
single group secret key, while the identity lane uses each
validator's own independent long-term ML-DSA-65 keypair.

**Claim 13.** The method of claim 1, wherein each validator's
signing operations in step (b) are dispatched to a graphics
processing unit (GPU) when the number of concurrent finality
certificates being produced exceeds a threshold (e.g. 64), and
to a central processing unit (CPU) otherwise, said dispatch
selection determined by an automatic accelerator-resolution
module.

**Claim 14.** The method of claim 1, wherein the certificate
emission in step (d) is performed by an aggregator node whose
identity is determined deterministically per consensus round by
a verifiable random function (VRF) over the validator set, such
that the aggregator role rotates across consensus rounds and no
single validator is a permanent aggregator.

**Claim 15.** The method of claim 1, wherein the validator-set
Merkle root is committed to the certificate by embedding a
48-byte BLS12-381 G1 commitment to the validator-set vector
within the certificate envelope, said commitment binding every
validator's BLS public key, ML-DSA-65 identity public key, and
NodeIDScheme byte.

**Claim 16.** The method of claim 1, wherein the certificate is
serialized into a fixed wire format with deterministic byte
layout, said layout enabling certificate hashing for parent-block
referencing in the chain's accepted-block index without
re-aggregation.

**Claim 17.** The method of claim 1, wherein the FIPS 204
ML-DSA-65 threshold signature `σ_M-LWE` of step (c2) is
verifiable by an unmodified third-party FIPS 140-3-validated
cryptographic module (such as BoringSSL FIPS, AWS-LC, or OpenSSL
3.0 PQ provider) acting as a black-box verifier with no
knowledge of the threshold protocol used to produce the
signature.

**Claim 18.** The method of claim 1, further comprising emitting
identifiable-abort evidence in the event that a participating
validator fails to provide a valid partial signature in step
(b2), said evidence comprising a typed envelope
`AbortEvidence{Kind, Accuser, Accused, Evidence, Signature}`
suitable for protocol-level slashing of the misbehaving
validator.

**Claim 19.** The method of claim 1, wherein the round time
budget for steps (b)-(d) is approximately 3 seconds for a
validator set of 21 validators, said budget being achievable
through parallel execution of the three signature lanes on
commodity hardware.

**Claim 20.** A blockchain network in which the certificate of
claim 1 is emitted per accepted block and stored as the
canonical finality artifact in the chain's block-acceptance
index, such that any verifier capable of executing the
verification method of claim 2 can independently confirm block
finality without re-running consensus.

**Claim 21.** A non-transitory computer-readable medium storing
instructions that, when executed by a processor of a validator
node, cause the processor to perform the method of claim 1.

**Claim 22.** A non-transitory computer-readable medium storing
instructions that, when executed by a processor of a verifier
node, cause the processor to perform the method of claim 2.

## §5 Reference to spec and implementation

- Spec: `~/work/lux/consensus/protocol/quasar/doc.go`,
  `~/work/lux/consensus/protocol/quasar/types.go` (QuasarCert),
  `~/work/lux/consensus/CLAUDE.md` §"Quasar Certificate".
- Paper: `~/work/lux/papers/lp-105-quasar-consensus.tex`
  (Definition 6.1 QuasarCert, Theorem 7.5 Soundness, Theorem 7.6
  Parallel Liveness, Theorem 7.7 Post-Quantum Safety).
- Proof: `~/work/lux/proofs/quasar-cert-soundness.tex`
  (App B ML-DSA-65 R1CS constraint count; App E Pulsar
  parameter tightness).
- Implementation:
  `~/work/lux/consensus/protocol/quasar/{bls,corona_gob,epoch,qblock,
  quasar,types,wave_signer,witness}.go`
  + `~/work/lux/consensus/protocol/quasar/engine.go`
  + `~/work/lux/consensus/protocol/quasar/grouped_threshold.go`.
- Benchmark: `~/work/lux/consensus/protocol/quasar/quasar_bench_test.go`,
  `~/work/lux/consensus/bench/pq_modes_bench_test.go`
  ("BLS aggregated verify @ 100 signers" = 875 µs, "QuasarCert verify
  approximate" = 2-5 ms per cert).

## §6 Prior-art differentiation summary

| Reference | Closest aspect | Why claim 1 still novel |
|-----------|----------------|--------------------------|
| Bindel et al. 2017 hybrid sig spectrum | Two-signature classical+PQ pair | Two NOT three; not threshold for the PQ lane; not byte-equal to FIPS verifier; not a single composed certificate |
| Ethereum 2.0 BLS aggregation | Aggregate BLS across attesters | Single-hardness only; no PQ lane |
| Filecoin BLS+Groth16 | BLS + ZK proof | Different purposes (finality + storage); not parallel hardness on the same finality decision |
| FROST-Dilithium 2024 hybrids | Classical Schnorr + PQ | Two-lane; not in a single consensus cert; not byte-equal FIPS-conformant on the PQ side |
| draft-ietf-pquip-hybrid-signature-spectrums | Hybrid signature taxonomy | Discusses paired sigs; Quasar's triple-or-quad composition is beyond the taxonomy's scope |
| Cremers et al. 2022 dual-curve | Two-curve signature | Both curves classical; no PQ; not multi-hardness across discrete-log vs lattice |

## §7 Filing strategy and timing

- US provisional: target by **2026-08-19** (90 days from this
  document).
- Public disclosure constraint: filing MUST precede any mainnet
  Q-Chain emission of QuasarCert under the production parameter set
  to preserve foreign-filing rights (12-month grace in US; 0-month
  grace in EPO/JP).
- PCT at 12 months from provisional → designate US, EPO, JP, KR, CN,
  AU, CA, UK, BR.
- National-phase entry at 30 months from priority → prioritize US,
  EPO, JP based on anticipated deployment.
- **Continuations**: file divisionals for (i) the Z-Chain Groth16
  rollup at production parameters, (ii) the Quasar-on-DAG variant
  (LP-105 §5.5), (iii) the threshold Corona R-LWE fourth lane when
  Corona's submission matures.

## §8 Defensive vs offensive recommendation

**OFFENSIVE.** This is the load-bearing Lux IP claim. Without it,
the multi-hardness consensus story is the published "QuasarCert"
that any third party could reimplement. Filing this and the URGENT
bundles (#2, #3, #5) gives Lux a defensible IP moat across the PQ
consensus + GPU + on-chain-PQ-verifier stack.

Defensive termination (Apache-2.0-style retaliation) should be
included in any filed claim's grant terms, mirroring the Pulsar
PATENTS.md §3 structure, to deter PQ patent trolling against the
broader ecosystem.

---

**Document metadata**
- Path: `consensus/docs/patent-claims-quasar-cert.md`
- Bundle: #1 of `lps/PATENT-INVENTORY.md`
- Created: 2026-05-19
- Status: **Internal working triage for attorney engagement**
