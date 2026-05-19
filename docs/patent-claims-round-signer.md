# RoundSigner Composition — Patent Claim Drafts (Attorney Review)

> **Internal working document.** Bundle #9 of the Lux PATENT-INVENTORY.
> Not a filed application; not a legal opinion.

## §0 Bundle summary

- **Title**: Composition pattern for blockchain-consensus per-round
  signers, in which a single signer object wraps an independent
  threshold-signature engine and a DAG-cut producer, decomposed by
  concern rather than coupled by inheritance.
- **Inventors**: Lux Industries cryptography team.
- **Priority date**: file as US provisional within 12 months OR
  defensive publication.
- **Estimated claim count**: 11 (1 independent + 10 dependent).
- **Defensive-vs-offensive**: **Defensive.** Software-decomplecting
  pattern; recommend defensive publication.

## §1 Background and prior art

1. **Tendermint / CometBFT** (Cosmos 2016-2026): consensus signer
   tightly coupled with consensus state machine.
2. **Aptos / Diem** (2019-2026): validator signing inheritance
   model.
3. **Ethereum 2.0 Beacon Chain** (2020-2026): per-validator BLS
   signer.

Lux's contribution: the `RoundSigner` value (in
`~/work/lux/consensus/protocol/quasar/round_digest.go`) is COMPOSED
of (a) `pulsarm.ThresholdSigner` for the cryptographic signing
substrate and (b) `prism.Cut` for the DAG-frontier producer, rather
than extending either. Each component is independently complete
and replaceable; the composition glue is a single struct that
references both.

## §2 Inventive concept

```
RoundSigner := struct {
    Threshold pulsarm.ThresholdSigner  // signs over Pulsar
    Cut       prism.Cut                // DAG-frontier producer
    // RoundDigest method binds them together
}
```

The `RoundDigest()` method on `RoundSigner` computes a canonical
digest of (a) the current DAG cut, (b) the round identifier, and
(c) any auxiliary fields, then dispatches to `Threshold.Sign`.

Each of `pulsarm.ThresholdSigner` and `prism.Cut` is independently
useful in other contexts (test harnesses, non-consensus signing,
DAG analysis tools).

## §3 Independent claim (draft)

> **Claim 1.** A computer-implemented method for producing a
> blockchain-consensus per-round signature, the method comprising:
>
> (a) instantiating a per-round signer object as a structure
>     comprising:
>
>     (a1) a reference to a threshold-signature engine
>          implementing post-quantum threshold signing under a
>          stable group public key;
>
>     (a2) a reference to a directed-acyclic-graph cut producer
>          configured to compute the current accepted frontier
>          of the consensus protocol's DAG; and
>
>     (a3) no inheritance relationship between the structure and
>          either the threshold-signature engine or the DAG cut
>          producer;
>
> (b) on entering a consensus round, invoking the DAG cut
>     producer to obtain the current frontier as an ordered set
>     of vertex identifiers;
>
> (c) computing a canonical round digest by hashing, with a
>     domain-separated SHAKE256 customization tag, the
>     concatenation of the round identifier, the frontier
>     vertex identifiers in canonical order, and any auxiliary
>     consensus fields; and
>
> (d) dispatching the round digest to the threshold-signature
>     engine's signing operation, returning the resulting
>     partial-threshold-signature contribution to the consensus
>     protocol's aggregator,
>
> wherein the threshold-signature engine and the DAG cut producer
> are independently usable in other contexts unrelated to
> consensus signing.

## §4 Dependent claims (drafts)

**Claim 2.** The method of claim 1, wherein the threshold-
signature engine is a `pulsarm.ThresholdSigner` implementing
threshold FIPS 204 ML-DSA-65 signing with byte-equality to a
single-party FIPS 204 verifier.

**Claim 3.** The method of claim 1, wherein the DAG cut producer
is a `prism.Cut` configured to compute the safe-prefix-commit
frontier of the consensus protocol's DAG.

**Claim 4.** The method of claim 1, wherein the per-round signer
object exposes the threshold-signature engine and the DAG cut
producer as two distinct accessor methods, allowing test harnesses
to replace either component independently for fault-injection
testing.

**Claim 5.** The method of claim 1, wherein the per-round signer
object's structure is value-typed (Go struct) rather than
reference-typed, allowing per-round signers to be cheaply copied
across goroutines.

**Claim 6.** The method of claim 1, wherein the canonical round
digest's SHAKE256 customization tag is `lux-quasar-round-digest-v1`,
providing domain separation from any other application using the
same hash function.

**Claim 7.** The method of claim 1, wherein the canonical round
digest includes a profile-identifier byte from the chain's
security profile, binding the signature to the active profile.

**Claim 8.** The method of claim 1, wherein the auxiliary
consensus fields hashed into the digest comprise at least:
the parent QBlock hash, the validator-set Merkle root, the
group-public-key hash, and the DKG-transcript root.

**Claim 9.** The method of claim 1, wherein the per-round signer
object is instantiated once per consensus epoch and reused across
every round within the epoch, with the DAG cut producer's
state refreshed per round.

**Claim 10.** The method of claim 1, further comprising adding a
third component to the per-round signer: a BLS aggregator
producing a parallel classical-hardness BLS12-381 aggregate
signature over the same round digest, the threshold-signature
engine and the BLS aggregator running independently in parallel.

**Claim 11.** A non-transitory computer-readable medium storing
the Go source code of the `RoundSigner` type, the
`pulsarm.ThresholdSigner` interface, the `prism.Cut` interface,
and the `RoundDigest` method, packaged as separate Go packages
to enforce the orthogonal-composition structure.

## §5 Reference to implementation

- `~/work/lux/consensus/protocol/quasar/round_digest.go`
- `~/work/lux/consensus/protocol/quasar/wave_signer.go`
- `~/work/lux/pulsar/threshold/...` (pulsarm.ThresholdSigner)
- `~/work/lux/consensus/protocol/prism/cut.go` (prism.Cut)

## §6 Defensive vs offensive

**DEFENSIVE.** Composition-over-inheritance is a 1990s-vintage
software-design pattern (Gang of Four, et al.); claim novelty is
limited to the specific cryptographic composition. Defensive
publication recommended.

---

**Document metadata**
- Path: `consensus/docs/patent-claims-round-signer.md`
- Bundle: #9 of `lps/PATENT-INVENTORY.md`
- Created: 2026-05-19
