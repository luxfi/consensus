# Q-Chain Finality Block (HIP-0079) — Patent Claim Drafts (Attorney Review)

> **Internal working document.** Bundle #11 of the Lux PATENT-INVENTORY.
> Not a filed application; not a legal opinion.

## §0 Bundle summary

- **Title**: A bounded-size blockchain finality block emitting one
  Pulsar-M threshold post-quantum signature over a transcript that
  binds a full cryptographic envelope (profile, hash suite,
  identity scheme, finality scheme, proof policy, proof backend,
  proof format, verifier ID, multiple state roots, and signer
  bitmap commitment), with constant-size growth regardless of
  validator-set cardinality.
- **Inventors**: Lux Industries cryptography team.
- **Priority date**: file as US provisional within 12 months.
- **Estimated claim count**: 14 (2 independent + 12 dependent).
- **Defensive-vs-offensive**: **Offensive.**

## §1 Background and prior art

1. **Ethereum Beacon Chain finality block** (Ethereum 2.0, 2020-
   2026): BLS-aggregated attestations per epoch; not PQ; per-
   validator attestation list grows with `n`.
2. **Cosmos / Tendermint header signatures** (2016-2026): per-
   validator Ed25519 signatures appended to block; grows with `n`.
3. **Avalanche P-Chain header** (Avalanche, 2020-2026): BLS-aggregated
   per epoch; classical.
4. **NIST PQC threshold proposals** (NIST MPTC 2025-2026): not yet
   in production consensus.

Closest prior art is [1] Ethereum Beacon. Lux's contributions:

- A **single Pulsar-M threshold signature** rather than N per-
  validator signatures, bounding the finality artifact size at
  ~3.3 KB regardless of `n`.
- A **transcript binding** of ALL envelope axes
  (`ProfileID`, `HashSuiteID`, `IdentitySchemeID`,
  `FinalitySchemeID`, `ProofPolicyID`, `ProofBackendID`,
  `ProofFormatID`, `VerifierID`, six 48-byte state roots,
  `SignerBitmapCommitment`) under cSHAKE256 customization tag
  `lux-qblock-v1`, such that any post-sign envelope mutation
  breaks signature verification at the threshold layer.
- **48-byte unification** of every state root (matches BLS12-381
  G1 commitment width), so a single field can carry either a hash
  digest or a commitment without envelope-format split.

## §2 Inventive concept

The Lux Q-Chain finality block (`QBlock`):

1. Each block carries `Version`, `NetworkID`, `ChainID`,
   `Height`, `RoundOrView`, `ParentQBlockHash`.
2. Profile envelope: `ProfileID` (binds the tuple
   `(HashSuiteID, FinalitySchemeID, ProofPolicyID,
   ProofBackendID)`).
3. Six 48-byte state-root commitments: `StateRoot`,
   `ZChainStateRoot`, `ValidatorSetRoot`, `CommitteeRoot`,
   `DKGTranscriptRoot`, `GroupPublicKeyHash`.
4. Payload anchors: `PayloadRoot`, `DARoot`.
5. Envelope axes: `ProofPolicyID`, `ProofBackendID`,
   `ProofFormatID`, `VerifierID`, `HashSuiteID`,
   `IdentitySchemeID`, `FinalitySchemeID`.
6. `SignerBitmapCommitment` (48-byte commitment to signer set).
7. `PulsarMThresholdSignature` (the single PQ threshold sig).

All envelope axes are bound into `TranscriptHash()` so a flipped
byte breaks signature verification, not just an envelope
comparison.

## §3 Independent claims (drafts)

### Claim 1 (Q-chain finality block claim, draft)

> **Claim 1.** A computer-implemented method for emitting a
> bounded-size blockchain finality artifact, the method
> comprising:
>
> (a) maintaining a finality lane separate from the chain's
>     transaction lane, the finality lane emitting one finality
>     block per consensus round;
>
> (b) constructing each finality block as a structure comprising
>     at least:
>
>     (b1) a version, network identifier, chain identifier, block
>          height, round-or-view counter, and parent finality-
>          block hash;
>
>     (b2) a profile identifier binding the tuple of: hash-suite
>          identifier, finality-scheme identifier, proof-policy
>          identifier, and proof-backend identifier;
>
>     (b3) at least six state-root commitments each of 48 bytes:
>          a Lux state root, a Z-chain state root, a validator-
>          set root, a committee root, a DKG-transcript root, and
>          a group-public-key hash;
>
>     (b4) two payload-anchor 48-byte commitments: a payload root
>          and a data-availability root;
>
>     (b5) explicit envelope-axis identifiers: a proof-policy
>          identifier, a proof-backend identifier, a proof-format
>          identifier, a verifier identifier, a hash-suite
>          identifier, an identity-scheme identifier, and a
>          finality-scheme identifier;
>
>     (b6) a 48-byte signer-bitmap commitment to the committee
>          signer-set or weight Merkle root;
>
>     (b7) a single post-quantum threshold signature over a
>          transcript hash binding all of the above;
>
> (c) computing the transcript hash via SP 800-185 cSHAKE256 with
>     a customization tag pinning the block schema identifier,
>     such that the customization tag is wire-format-stable
>     across protocol versions;
>
> (d) emitting the finality block as the chain's per-round finality
>     artifact, with the artifact size bounded at approximately
>     the post-quantum threshold-signature size plus the fixed
>     envelope size, independent of the cardinality of the
>     validator set; and
>
> (e) refusing to accept any finality block whose post-quantum
>     threshold signature does not verify against the transcript
>     hash recomputed by the verifier from the block's envelope
>     bytes.

### Claim 2 (envelope binding claim, draft)

> **Claim 2.** The method of claim 1, wherein the transcript hash
> of step (c) binds every envelope-axis identifier and every
> state-root commitment, such that a malicious adversary who
> modifies any envelope axis post-signing produces a finality
> block whose threshold signature does NOT verify against the
> recomputed transcript hash, thereby converting envelope-axis
> tampering from a receiver-side envelope-comparison check into
> a cryptographic-signature-verification failure.

## §4 Dependent claims (drafts)

**Claim 3.** The method of claim 1, wherein the post-quantum
threshold signature is a Pulsar-M threshold signature with M ∈
{ML-DSA-44, ML-DSA-65, ML-DSA-87} producing output byte-equal to
the corresponding single-party FIPS 204 ML-DSA signature on the
Lagrange-reconstructed centralized secret key.

**Claim 4.** The method of claim 1, wherein the customization
tag for the transcript hash is the wire-format-stable string
`lux-qblock-v1` per SP 800-185 cSHAKE256.

**Claim 5.** The method of claim 1, wherein the 48-byte state-root
unification accommodates either a hash digest (e.g., SHAKE256
output truncated to 48 bytes) or a BLS12-381 G1 / KZG commitment,
allowing the same field to carry different proof artifact types
without envelope-format split.

**Claim 6.** The method of claim 1, wherein the bulky validator
identity, DKG transcripts, and per-validator identity-signature
data are stored on a separate Z-Chain and referenced from the
finality block only via the corresponding Z-Chain epoch
commitment's 48-byte state root.

**Claim 7.** The method of claim 1, wherein the signer-bitmap
commitment is computed as the cSHAKE256 hash of the signer-set
bitmap (one bit per validator) concatenated with the validator-
set Merkle root, such that a missing or short commitment is
unconditionally below the threshold.

**Claim 8.** The method of claim 1, wherein the chain
configuration's `IdentitySchemeID` binds the validator-identity
signature scheme to the block envelope, such that cross-identity-
scheme replay is closed.

**Claim 9.** The method of claim 1, wherein the `ParentQBlockHash`
field embeds the previous finality block's transcript hash,
forming a hash chain over the finality lane independent of the
transaction lane's block hash chain.

**Claim 10.** The method of claim 1, wherein the finality block
is serialized into a fixed-byte-layout wire format, with every
field's byte position deterministic such that re-serialization
on the verifier side produces byte-identical input to the
transcript hash function.

**Claim 11.** The method of claim 1, wherein the finality block
size is bounded above at approximately 3.5 KB for a Pulsar-M-65
threshold signature, regardless of whether the validator set
contains 21 or 1,021 validators.

**Claim 12.** The method of claim 1, wherein an additional
`SubmitterIdentitySignature` field carries a single-party
ML-DSA-65 identity signature by the proposing validator over the
finality block's transcript hash, providing a defense-in-depth
identity proof alongside the threshold signature.

**Claim 13.** The method of claim 1, wherein the chain enforces a
finality-cadence parameter such that one finality block is
emitted approximately every 3 seconds.

**Claim 14.** A non-transitory computer-readable medium storing
the Go source code of the `QBlock` type, the `TranscriptHash`
method, the serialization functions, and the verifier function.

## §5 Reference to implementation

- `~/work/lux/consensus/protocol/quasar/qblock.go`.
- `~/work/lux/consensus/protocol/quasar/qblock_test.go`.
- `~/work/lux/lps/LP-0079-q-chain-finality-block.md`.

## §6 Defensive vs offensive

**OFFENSIVE.** Bounded-size PQ-threshold finality is a defining
moat against competitor L1s that ship per-validator ML-DSA-65
signature lists (which grow to ~70 KB at n=21).

---

**Document metadata**
- Path: `consensus/docs/patent-claims-q-chain-finality-block.md`
- Bundle: #11 of `lps/PATENT-INVENTORY.md`
- Created: 2026-05-19
