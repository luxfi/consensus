# Double-Lattice (Pulsar M-LWE + Corona R-LWE in one QuasarCert) — Patent Claim Drafts (Attorney Review)

> **Internal working document.** Bundle #18 of the Lux PATENT-INVENTORY.
> Not a filed application; not a legal opinion.

## §0 Bundle summary

- **Title**: A blockchain finality certificate composing TWO
  independent post-quantum lattice threshold-signature lanes
  (Module-LWE byte-equal to FIPS 204 ML-DSA-65 + Ring-LWE
  threshold over distinct algebraic structure) over the same
  message-context binder, providing defense-in-depth against
  cryptanalytic breakthrough specific to either lattice
  structure.
- **Inventors**: Lux Industries cryptography team.
- **Priority date**: file as US provisional within 12 months.
- **Estimated claim count**: 13 (1 independent + 12 dependent).
- **Defensive-vs-offensive**: **Offensive.**
- **Note**: this bundle extends bundle #1 (Quasar Certificate)
  with the **specific** double-lattice composition as a separate
  inventive concept.

## §1 Background and prior art

1. **NIST PQC standards** (FIPS 203/204/205): single PQ primitive
   per scheme; no double-lattice production deployment.
2. **draft-ietf-pquip-hybrid-signature-spectrums**: hybrid
   classical + PQ signatures; not multi-PQ.
3. **Cremers et al. 2022 dual-curve**: two classical curves; no
   PQ.
4. **NIST IR 8413 PQC migration guidance**: suggests transition,
   not double-lattice composition.
5. **Pulsar (luxfi/pulsar)** and **Corona (luxfi/corona)**: the
   two threshold kernels Lux ships as candidate
   "double-lattice" components in QuasarCert.

Closest prior art: no public proposal or production scheme
composes Module-LWE and Ring-LWE threshold signatures into a
single finality certificate. The literature distinguishes them
as DIFFERENT lattice assumptions:

- **Module-LWE** (M-LWE): hardness on `R_q^k` for matrix size
  `k × l`; underlies FIPS 204 ML-DSA, FIPS 203 ML-KEM.
- **Ring-LWE** (R-LWE): hardness on `R_q^1`; underlies Corona,
  Boschini ePrint 2024/1113.

A cryptanalytic breakthrough specific to ONE may not affect the
OTHER. Lux's claim: composing both in a single certificate
provides cryptanalytic-diversity defense-in-depth.

## §2 Inventive concept

```
QuasarCert {
    BLS:           [48 bytes]        // classical ECDLP
    Pulsar:        [3.3 KB]          // M-LWE byte-equal FIPS 204
    Corona:        [33 KB]           // R-LWE construction
    MLDSAProof:    [192 bytes]       // Groth16 over identity sigs
}
```

The two PQ lattice lanes have **distinct hardness assumptions**:
- Pulsar / FIPS 204 = Module-LWE + Module-SIS (different
  algebraic structure: matrix over `R_q`).
- Corona = Ring-LWE over `R_q^1` (single-ring algebra).

A double-lattice certificate is accepted only if BOTH
lattice lanes verify, in addition to the classical BLS lane.
An adversary forging a certificate must break:
- ECDLP for BLS12-381 (classical hardness);
- Module-LWE + Module-SIS for Pulsar (PQ lattice #1);
- Ring-LWE for Corona (PQ lattice #2).

## §3 Independent claim (draft)

> **Claim 1.** A computer-implemented method for producing a
> blockchain finality certificate with defense-in-depth across
> distinct post-quantum lattice cryptographic hardness
> assumptions, the method comprising:
>
> (a) receiving, at an aggregator node, partial cryptographic
>     contributions from a quorum of validator nodes for each of
>     at least three independent signing lanes over the same
>     message-context binder `m`:
>
>     (a1) a BLS12-381 aggregate signature lane producing a
>          ~48-byte aggregate signature `σ_BLS` whose security
>          reduces to the BLS12-381 co-CDH assumption;
>
>     (a2) a Module-Lattice threshold signature lane (Pulsar)
>          producing a single threshold signature `σ_M-LWE`
>          byte-equal to a single-party FIPS 204 ML-DSA-65
>          signature on the Lagrange-reconstructed centralized
>          secret key, said signature's security reducing to the
>          Module-LWE and Module-SIS assumptions on the FIPS 204
>          parameter set (matrix-shaped ring);
>
>     (a3) a Ring-Lattice threshold signature lane (Corona)
>          producing a threshold signature `σ_R-LWE` verifiable
>          under a Ring-LWE threshold verifier, said signature's
>          security reducing to the Ring-LWE assumption on a
>          single-ring algebra `R_q^1` distinct from the
>          matrix-shaped ring of step (a2);
>
> (b) combining the three lanes' aggregator outputs into a
>     single certificate
>     `Cert = (σ_BLS, σ_M-LWE, σ_R-LWE, m_digest)`;
>
> (c) emitting the certificate as the finality artifact for the
>     consensus round;
>
> (d) at any verifier node, parsing the certificate and verifying
>     all three lanes:
>
>     (d1) `σ_BLS` against the BLS12-381 aggregate public key;
>
>     (d2) `σ_M-LWE` against the unmodified FIPS 204 ML-DSA-65
>          verifier; and
>
>     (d3) `σ_R-LWE` against the Corona threshold-verifier;
>
> (e) accepting the certificate as finalizing the block if and
>     only if all three verifications succeed,
>
> wherein an adversary forging a certificate accepted by the
> verifier must simultaneously break (i) the BLS12-381 co-CDH
> assumption, (ii) the Module-LWE / Module-SIS assumptions on the
> FIPS 204 ML-DSA-65 parameter set, AND (iii) the Ring-LWE
> assumption on the Corona parameter set — each of which is
> independently believed computationally infeasible under its
> respective threat model, and where a cryptanalytic breakthrough
> specific to one lattice structure (M-LWE or R-LWE) does NOT
> break the other.

## §4 Dependent claims (drafts)

**Claim 2.** The method of claim 1, wherein the Module-Lattice
threshold signature lane of step (a2) uses parameter set
ML-DSA-65 producing 3309-byte signatures, and the Ring-Lattice
threshold signature lane of step (a3) uses parameter set
M=8, N=7, LogN=8 (ring degree 256), Q=0x1000000004A01,
Dbar=48, Kappa=23 producing ~33 KB signatures.

**Claim 3.** The method of claim 1, wherein the Module-Lattice
threshold signature of step (a2) is produced by a 2-round
Pulsar protocol (commit-then-respond) and the Ring-Lattice
threshold signature of step (a3) is produced by a 2-round
Corona protocol, both run in parallel by the validator nodes.

**Claim 4.** The method of claim 1, wherein the Module-Lattice
and Ring-Lattice threshold signatures use INDEPENDENT key
material — distinct DKG ceremonies producing distinct group
public keys for each lane — such that the share-distributions
of the two lanes are not coupled.

**Claim 5.** The method of claim 1, wherein the certificate
additionally includes a fourth lane comprising a Groth16
zero-knowledge proof rolling up per-validator FIPS 204
ML-DSA-65 identity signatures, providing further defense-in-
depth at the individual-signer level.

**Claim 6.** The method of claim 1, wherein the Ring-Lattice
threshold signature is produced over a distinct Q-Chain
finality lane that emits one Corona signature per consensus
round, while the Module-Lattice threshold signature is produced
over a separate validator-set epoch lane that emits one Pulsar
signature per epoch.

**Claim 7.** The method of claim 1, wherein a chain
configuration option (`LUX_CONSENSUS_PQ_MODE = TripleQuantum`)
enables the double-lattice + BLS + Groth16 four-lane composition
at runtime, while alternative options (`BLSOnly`,
`BLSPlusMLDSA`, `BLSPlusCorona`) enable smaller subsets for
performance tuning.

**Claim 8.** The method of claim 1, wherein the message-context
binder `m` is the SHAKE256 cSHAKE digest of the block header,
state root, validator-set commitment, round identifier, and
profile identifier, with customization tag `lux-quasar-cert-v1`
per SP 800-185.

**Claim 9.** The method of claim 1, wherein the two PQ lattice
lanes are independently profiled and selected: an
implementation may choose to enable only one PQ lattice lane
at a time, with the chain configuration recording the active
profile in the block header.

**Claim 10.** The method of claim 1, wherein the verifier's
verification cost for all three lanes is bounded by
approximately 2-5 milliseconds per certificate on commodity
hardware, with the BLS pairing dominating at ~875 µs, the FIPS
204 ML-DSA-65 verify at ~250 µs, and the Corona verify variable
but amortized via the Pulsar/Corona shared Lagrange
reconstruction step.

**Claim 11.** The method of claim 1, wherein the share-
redistribution for the Module-Lattice lane and the Ring-Lattice
lane runs in parallel during the validator-set rotation
ceremony, each share-redistribution preserving its lane's group
public key across the rotation.

**Claim 12.** A consensus protocol comprising the method of
claim 1 and configured to participate in a Byzantine-fault-
tolerant blockchain network of `n ≥ 21` validators, with each
of the three lanes' threshold equal to `2f + 1` where `f` is the
maximum tolerated fault count.

**Claim 13.** A non-transitory computer-readable medium storing
the Go source code of the Quasar certificate composition, the
Pulsar Module-Lattice kernel, the Corona Ring-Lattice kernel,
the BLS12-381 aggregator, the certificate-emission code, and
the verifier code.

## §5 Reference to implementation

- `~/work/lux/consensus/protocol/quasar/types.go` (QuasarCert
  including BOTH Pulsar and Corona fields where applicable).
- `~/work/lux/consensus/protocol/quasar/quasar.go`.
- `~/work/lux/consensus/protocol/quasar/dual_threshold_test.go`
  (test exercising both lattice lanes).
- `~/work/lux/pulsar/` (Module-Lattice kernel).
- `~/work/lux/corona/` (Ring-Lattice kernel).
- Performance: combined verify ~2-5 ms per consensus CLAUDE.md
  benchmark.

## §6 Defensive vs offensive

**OFFENSIVE.** Composing two distinct lattice assumptions in a
single certificate is genuinely novel against the literature
and against competitor L1s. Filing this protects Lux's
defense-in-depth story.

---

**Document metadata**
- Path: `consensus/docs/patent-claims-double-lattice.md`
- Bundle: #18 of `lps/PATENT-INVENTORY.md`
- Created: 2026-05-19
