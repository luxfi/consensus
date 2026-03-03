# Metastable Signed Queries — consensus-embedded finality certs

## Idea

The Quasar family (photon → wave → flare → focus → horizon → ray → nova → nebula → prism → quasar) already samples `k` validators per round and accumulates preferences across `β` rounds until metastable convergence.

**Extend**: each query carries an ML-DSA signature from the responder over the block it's voting for. The cert that lands on-chain is the **bundle of signatures collected during the converging rounds**, with no separate VRF sampling step and no separate finality-cert protocol.

```
BEFORE (two-step):
  photon → pick committee via VRF  →  committee runs separate signing round  →  cert

AFTER (one-step):
  photon → sample peers and query  →  responses carry sigs  →  cert = accumulated sigs
```

Consensus and cert generation happen in the same gossip.

## Why this is a good fit

1. **Zero extra rounds** — sigs piggyback on the queries that were happening anyway.
2. **No separate VRF needed** — committee is implicitly "whoever my photon sampled across the converging rounds."
3. **Natural fit with DAG** — each DAG vertex carries its own signed query accumulator. Certs bubble up the DAG just like votes.
4. **Probabilistic sampling** — different validators see slightly different committees (as in Avalanche). Metastable means that once enough converge, the decision is locked with exponentially decaying disagreement probability.
5. **Scale invariance** — `k=20` (photon default) is independent of total network size.

## Protocol sketch

```
Proposer P announces block B at height h.

For each honest validator v:
  round 0 .. β-1:
    sample s_v = photon.Emit(k)   // k=20 uniform sample from active set
    for each n in s_v:
      msg = (B.hash, h, round, v.id)
      query(n, msg) → response{preference, σ_n}
      where σ_n = ML-DSA.Sign(sk_n, canonical_msg || n.preference)
    update v.preference via wave accumulator

  if v.preference has crossed the ray/nova threshold:
    emit finality vote with (v.preference, ∪ collected σ from converging rounds)

Block finality cert:
  cert = { block_hash, bitmap, collected_sigs }
  where bitmap marks which validators appear in at least one sig
```

The cert is compiled by the block proposer (or any aggregator) from the signed queries it observed.

## Security argument

The classical Avalanche argument says that after β rounds with metastable threshold `α > k/2`, the probability of a safety failure decays as `negl(β)` assuming < 1/3 Byzantine stake.

**With signed queries:** a Byzantine validator can't forge a signed query from an honest peer. So a valid cert implicitly proves that the accumulation happened honestly — every signature is a cryptographic commitment to the responder's preference at some round.

Forging the cert requires forging ML-DSA signatures for > 2k/3 of the entries in the bitmap. This reduces to the standard ML-DSA unforgeability argument, same as the fixed-committee case.

**PQ safety:** ML-DSA is PQ. No classical primitive in the finality path. BLS is not needed.

## Round complexity

| Layer | Rounds | Notes |
|-------|--------|-------|
| Per-query gossip | 1 | validator samples k peers, waits for responses |
| β rounds to converge | β (≈ 15 for 10⁻⁹ safety) | parallel across validators |
| Finality cert | 0 extra | emitted when metastable threshold reached |

Total rounds to finality: **β** (same as vanilla Avalanche). Extra rounds for cert generation: **0**.

## Concrete sizing

Parameters (default photon):
- k = 20 (sample size per query round)
- β = 15 (convergence rounds for safety 10⁻⁹)
- α = 15 (metastable threshold, conservative)

Per-validator work to produce finality cert:
- Sign k responses per round × β rounds = 300 ML-DSA sigs per validator per block
- At ~430 µs/sig that is 129 ms of sign work per validator per block

This is heavy. Two mitigations:

1. **Query batching**: a validator signs *once* per block and reuses the sig across queries for that block. Reduces to 1 sign per validator per block. ~430 µs.
2. **Response caching**: responses to the same (block, round) are identical for a given validator; compute once, serve many.

Under batching, per-block work per validator is roughly:
- 1 sign (~0.4 ms)
- k×β verifies of peer sigs = 300 × 40 µs = 12 ms
- Amortized across β rounds running in parallel → < 1 ms wall-clock per round

## Cert size under metastable

The final cert includes sigs from every validator that appeared in at least one converging query. For a well-distributed sampling over 10⁴-10⁵ validators with `k=20` and `β=15`:

- Expected unique signers ≈ k × β × (1 - stickiness) ≈ 200-300
- Cert size ≈ 300 × 2.4 kB = 720 kB

This is larger than the fixed k=128 cert (310 kB). Trade-off: we save the separate VRF + committee-signing protocol and get consensus + cert in a single pass.

Compression options:
- **Per-epoch Ringtail** over converging signers (off the hot path)
- **Per-era Z-chain Groth16** proof over epoch certs

## How it extends the existing protocols

- `photon`: sample `k` peers (already exists, extend responses with sig)
- `wave`: accumulate preferences (already exists, extend to accumulate sigs)
- `flare`: threshold α over accumulated (already exists, unchanged)
- `focus`: disambiguate between candidates (already exists)
- `horizon`: mark finalization horizon (already exists)
- `ray` / `nova`: finality signal (emit the sig bundle when crossed)
- `prism`: committee cut (inputs to stake-weighted sampling)
- `quasar`: compose the final cert from accumulated sigs

## Comparison to fixed-committee

|  | Fixed k=128 + VRF | Metastable signed queries |
|--|-------------------|---------------------------|
| Separate committee election | yes (VRF step) | no (implicit) |
| Extra rounds for cert | 1 (sig collection) | 0 |
| Cert size | 310 kB | ~720 kB |
| Round complexity to finality | 1 + β | β |
| Light-client verify | straightforward (known committee) | requires round-by-round replay or SNARK |
| Sybil resistance | VRF weighting | photon weights by stake already |

The metastable path is operationally simpler (nothing separate from consensus) but produces larger certs. For light clients we compress with Z-chain ZK proofs.

## 1ms verify path

For k=32 sampling with β=15 rounds, expected unique signers ≈ 50-80.

- 50 ML-DSA verifies parallel on 10 cores: ~1ms
- Achievable on M-series GPU via `luxfi/accel` batch verify

**1ms finality verify at N=100,000 validators is attainable** with k=32 photon sampling + β=15 metastable + GPU batch verify.

## References

- Existing Quasar family: `~/work/lux/consensus/protocol/{photon,wave,flare,focus,horizon,ray,nova,nebula,prism,quasar}`
- Avalanche/Snowman metastable family
- LP-045 hierarchical quorum certs
- `~/work/lux/proofs/pq-finality-no-bls.tex`
