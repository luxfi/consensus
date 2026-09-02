// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The finality standard, in Rust.
//!
//! Go is the network. This module is correct exactly insofar as it reproduces
//! what `github.com/luxfi/consensus/engine/chain` produces, and `tests/
//! conformance.rs` holds it to that against `conformance/corpus.json` — the
//! corpus generated from the Go definitions themselves.
//!
//! Two rungs, never collapsed. `Nova` is a strict majority of stake and
//! authorizes local execution; it is reorgable. `Quasar` is a strict two thirds
//! of stake and is the only rung a bridge, a settlement or a cross-chain message
//! may read. An implementation with one rung cannot express the accept that
//! every Lux chain actually runs on.

/// A 32-byte identifier. Empty means unset.
pub type Id = [u8; 32];

/// The empty identifier — the value that triggers the canonical degrade.
pub const EMPTY: Id = [0u8; 32];

/// The domain tag, NUL-terminated. Its own version rides in the tag so a
/// signature over the canonical commitment can never be read as a signature
/// over the outer envelope id.
pub const VOTE_TAG: &[u8] = b"LUX/chain/vote/v2\0";

/// The certificate version folded into every signed message.
pub const QUORUM_CERT_VERSION: u16 = 3;

/// The certificate role. A finality certificate witnesses acceptance, so this is
/// the only role a chain vote carries.
pub const QC_FINALITY: u8 = 1;

/// The length of a signed vote message. Every field is fixed width, so the
/// message is length-free and always exactly this long.
pub const VOTE_MESSAGE_LEN: usize = 226;

/// The consensus position a vote binds to.
///
/// It carries two identities. The canonical execution identity — `canonical_id`,
/// `parent_canonical_id`, `execution_state_root`, `payload_root` — is the
/// primary consensus object and is signed. The transport identity — `block_id`,
/// `parent_id` — is the outer envelope, a cache key for block lookup, and is NOT
/// signed. Two nodes that executed the same inner block therefore sign identical
/// bytes however it was wrapped, and their votes interoperate.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Position {
    pub chain_id: Id,
    pub height: u64,
    pub round: u32,
    pub block_id: Id,
    pub parent_id: Id,
    pub canonical_id: Id,
    pub parent_canonical_id: Id,
    pub execution_state_root: Id,
    pub payload_root: Id,
    pub validator_set_root: Id,
}

impl Position {
    /// The identity a finality index must key on: the value the signature
    /// actually commits to. It is `canonical_id`, or `block_id` only in the
    /// degrade where no canonical id is set — exactly the byte
    /// [`canonical_vote_message`] folds in as the canonical. The transport
    /// `block_id` is otherwise unsigned, so keying finality on it lets a
    /// verified certificate be relabelled to a block it never attested; keying
    /// on this value cannot be, and two positions that sign to the same bytes
    /// resolve to the same finality entry rather than two.
    pub fn signed_identity(&self) -> Id {
        if self.canonical_id == EMPTY {
            self.block_id
        } else {
            self.canonical_id
        }
    }
}

/// The exact bytes a validator signs.
///
/// Layout, big-endian and fixed width throughout:
///
/// ```text
/// "LUX/chain/vote/v2\0"   18
/// version                  2
/// qc_type                  1
/// chain_id                32
/// height                   8
/// round                    4
/// canonical_id            32
/// parent_canonical_id     32
/// execution_state_root    32
/// payload_root            32
/// validator_set_root      32
/// accept                   1
/// ```
///
/// `accept` is bound, so an accept signature and a reject signature over one
/// position are distinct messages and neither can be presented as the other.
///
/// The degrade is resolved here and only here: a position whose canonical slots
/// are unset — a block with no inner/outer split — binds its transport ids under
/// them, so every producer of a position signs the same bytes for the same
/// block.
pub fn canonical_vote_message(pos: &Position, accept: bool) -> Vec<u8> {
    let mut buf = Vec::with_capacity(VOTE_MESSAGE_LEN);
    buf.extend_from_slice(VOTE_TAG);
    buf.extend_from_slice(&QUORUM_CERT_VERSION.to_be_bytes());
    buf.push(QC_FINALITY);
    buf.extend_from_slice(&pos.chain_id);
    buf.extend_from_slice(&pos.height.to_be_bytes());
    buf.extend_from_slice(&pos.round.to_be_bytes());

    // The same degrade rule the finality index keys on — one home, so the bytes
    // signed and the bytes finalized cannot drift apart. See
    // `Position::signed_identity`.
    let canonical = pos.signed_identity();
    let parent = if pos.parent_canonical_id == EMPTY { &pos.parent_id } else { &pos.parent_canonical_id };
    buf.extend_from_slice(&canonical);
    buf.extend_from_slice(parent);

    buf.extend_from_slice(&pos.execution_state_root);
    buf.extend_from_slice(&pos.payload_root);
    buf.extend_from_slice(&pos.validator_set_root);
    buf.push(if accept { 0x01 } else { 0x00 });
    buf
}

/// `floor(2·total/3)` — the threshold an export quorum must STRICTLY exceed.
///
/// Computed from `total` alone because `2·total` overflows near 2^64:
/// `floor(2·total/3) = 2·(total/3) + floor(2·(total mod 3)/3)`, and
/// `floor(2r/3)` for r in {0,1,2} is {0,0,1}.
pub fn two_thirds_stake_floor(total: u64) -> u64 {
    let (q, r) = (total / 3, total % 3);
    let mut floor = 2 * q;
    if r == 2 {
        floor += 1;
    }
    floor
}

/// `floor(total/2)` — the threshold a local-execution quorum must STRICTLY
/// exceed. One rung below the export floor, and deliberately so.
pub fn half_stake_floor(total: u64) -> u64 {
    total / 2
}

/// The majority the sampler needs to ignite a block to Nova. `n < 1` yields 1: a
/// lone node self-ignites, and never 0, which would let a transiently empty view
/// self-accept.
pub fn nova_quorum(n: i64) -> i64 {
    if n < 1 {
        return 1;
    }
    n / 2 + 1
}

/// The smallest Byzantine-fault-tolerant committee: the least n whose fault budget
/// f = ⌊(n−1)/3⌋ reaches one. Below it a two-thirds supermajority tolerates no
/// Byzantine fault at all.
///
/// Two consumers, one constant: [`nova_signer_floor`] saturates its count here so a
/// lone node can never ignite, and [`crate::cert::QuorumCert::verify_weighted`]
/// refuses an EXPORT certificate over a signing set smaller than this. Go's
/// `engine/chain.minBFTCommittee`, C++'s `kMinBFTCommittee`.
pub(crate) const MIN_BFT_COMMITTEE: i64 = 4;

/// The minimum distinct signers a Nova certificate needs whatever the stake
/// distribution. The Nova gate proper is a stake majority; this count is the
/// guard the stake predicate cannot give — a single holder of a stake majority
/// would otherwise self-ignite.
pub fn nova_signer_floor(n: i64) -> i64 {
    let q = nova_quorum(n);
    let m = nova_quorum(MIN_BFT_COMMITTEE);
    if q < m {
        q
    } else {
        m
    }
}

/// The confidence depth: consecutive majority rounds required to ignite Nova.
pub fn nova_beta(n: i64) -> i64 {
    if n <= 1 {
        1
    } else {
        2
    }
}

/// Simultaneous crash faults Nova ignition survives.
pub fn crash_tolerance(n: i64) -> i64 {
    if n < 2 {
        return 0;
    }
    n - nova_quorum(n)
}

/// The two-thirds SUPERMAJORITY COUNT of n — the smallest number of seats that is
/// strictly more than two thirds of them, `floor(2n/3) + 1`. For n = 21 this is
/// 15, not 14: 14/21 does not strictly exceed two thirds. Derived from
/// [`two_thirds_stake_floor`] over n unit weights rather than restated, so the
/// count and the stake predicate cannot drift.
///
/// Two consumers, one rule seen from two sides. It is the count a live
/// equal-stake network sizes alpha to, and it is the floor on DISTINCT signers
/// that [`crate::cert::QuorumCert::verify_weighted`] demands of an export
/// certificate whatever the stake distribution — the guard the stake predicate
/// cannot give, because two thirds of the stake is one signature wherever two
/// thirds of the stake is one validator. Go's `config.TwoThirdsCount`.
pub fn two_thirds_count(n: i64) -> i64 {
    if n <= 0 {
        return 1;
    }
    two_thirds_stake_floor(n as u64) as i64 + 1
}

/// The DISTINCT signers a certificate must carry to attest `tier` over a set of
/// `n` signers — the whole of a certificate's authority in seats, and a function
/// of the set and the rung, never of the certificate.
///
/// One definition, read in three places: the assembler picks its alpha from it,
/// the weighted predicate enforces it, and the derived-threshold clause compares
/// the certificate's own declaration against it. A second spelling anywhere is
/// how a certificate acquires a quorum of its own choosing.
///
/// * Nova — [`nova_signer_floor`], which saturates at three: local execution has
///   to stay reachable on a small chain, and it is reorgable.
/// * Quasar — [`two_thirds_count`], the export supermajority read in seats, the
///   same supermajority the stake clause reads in weight.
///
/// A rung that is not an accept tier has no floor and gets none: 0, which every
/// caller reads as a refusal. Go's `chain.SignerFloor`.
pub fn signer_floor(tier: Finality, n: i64) -> i64 {
    match tier {
        Finality::Nova => nova_signer_floor(n),
        Finality::Quasar => two_thirds_count(n),
        _ => 0,
    }
}

/// The minimum vote count that CAN reach the two-thirds-by-stake predicate for a
/// weight vector: order heaviest first and count until the running stake first
/// exceeds the floor. Returns 0 for an empty set, a zero total, or a vector with
/// no representable total — no stake model, fail closed.
///
/// This is a SIZER, not a floor. It answers "below how many votes is two thirds
/// of this particular stake distribution unreachable", which is what a parameter
/// sizer needs and is Go's `config.WeightedSupermajorityThreshold`. The floor a
/// certificate is held to is [`two_thirds_count`] over the set SIZE, and it is
/// deliberately the larger of the two on a skewed set: the whole point of the
/// floor is that concentrated stake must not shrink the number of parties whose
/// agreement export finality reports.
pub fn weighted_quasar(weights: &[u64]) -> usize {
    // Checked, and out the same door as a zero total. A vector summing past
    // `u64::MAX` has no total, so it has no two thirds of one to size a count
    // against. A plain sum would wrap to a SMALL total and hand back a count
    // below the real stake quorum — a count gate sized under the predicate it
    // exists to anticipate, which is the fail-open direction. No admitted set
    // can be such a vector: `ValidatorSet::insert` refuses the sum at the door.
    let total = match weights.iter().try_fold(0u64, |acc, &w| acc.checked_add(w)) {
        Some(total) => total,
        None => return 0,
    };
    if total == 0 || weights.is_empty() {
        return 0;
    }
    let floor = two_thirds_stake_floor(total);
    let mut sorted = weights.to_vec();
    sorted.sort_unstable_by(|a, b| b.cmp(a));
    let mut cum: u64 = 0;
    let mut count = 0usize;
    for w in sorted {
        count += 1;
        // Bounded by `total`, which is representable, so the running sum is too.
        cum += w;
        if cum > floor {
            break;
        }
    }
    count
}

/// A block's rung: the single highest authority it has reached.
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
#[repr(u8)]
pub enum Finality {
    /// In flight, being sampled. Authorizes nothing.
    Photon = 0,
    /// The forming preference. A preference is not an acceptance.
    Wave = 1,
    /// Ignition on a stake majority. Authorizes LOCAL execution, reorgable.
    Nova = 2,
    /// A two-thirds-by-stake certificate. Authorizes export.
    Quasar = 3,
    /// Post-quantum sealed. Authorizes irreversible settlement.
    Horizon = 4,
}

impl Finality {
    /// Whether this rung may drive local execution — Nova or brighter. A caller
    /// acting here must be prepared to reorg until Quasar.
    pub fn authorizes_local_execution(self) -> bool {
        self >= Finality::Nova
    }

    /// Whether this block may leave the chain as final. THE INVARIANT: nothing
    /// below Quasar may reach a bridge, another chain, or a settlement.
    pub fn authorizes_export(self) -> bool {
        self >= Finality::Quasar
    }

    /// Whether this block is irreversible against a quantum adversary.
    pub fn authorizes_irreversible_settlement(self) -> bool {
        self >= Finality::Horizon
    }

    /// The lowercase ontology name, used verbatim in status and metrics.
    pub fn name(self) -> &'static str {
        match self {
            Finality::Photon => "photon",
            Finality::Wave => "wave",
            Finality::Nova => "nova",
            Finality::Quasar => "quasar",
            Finality::Horizon => "horizon",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::weighted_quasar;

    /// A weight vector with no representable total takes the fail-closed exit,
    /// not the wrapped one. Two validators of 2^63 sum to 2^64: a plain sum
    /// wraps that to 0 in release and panics in debug, and a wrapped total of 0
    /// would hand `two_thirds_stake_floor` a floor the first voter clears — a
    /// count gate of 1 for a set no count can finalize. One less than that pair
    /// is a real total, and still counted exactly.
    #[test]
    fn a_weight_vector_with_no_total_is_fail_closed() {
        assert_eq!(weighted_quasar(&[1 << 63, 1 << 63]), 0);
        assert_eq!(weighted_quasar(&[u64::MAX, 1]), 0);
        assert_eq!(weighted_quasar(&[u64::MAX / 2, u64::MAX / 2]), 2);
        assert_eq!(weighted_quasar(&[u64::MAX]), 1);
    }

    /// The exits that were already there stay where they were.
    #[test]
    fn no_stake_model_is_zero() {
        assert_eq!(weighted_quasar(&[]), 0);
        assert_eq!(weighted_quasar(&[0, 0, 0]), 0);
    }
}
