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
    /// The execution identity this position binds, with the degrade applied: a
    /// block with no inner/outer split binds its transport id.
    ///
    /// The degrade is resolved here and only here. Two places resolving it is
    /// how one node signs the block id and another signs the empty id for the
    /// same block.
    pub fn canonical(&self) -> Id {
        if self.canonical_id == EMPTY {
            self.block_id
        } else {
            self.canonical_id
        }
    }

    /// The parent's execution identity, degraded the same way.
    pub fn parent_canonical(&self) -> Id {
        if self.parent_canonical_id == EMPTY {
            self.parent_id
        } else {
            self.parent_canonical_id
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

    buf.extend_from_slice(&pos.canonical());
    buf.extend_from_slice(&pos.parent_canonical());

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

/// The smallest Byzantine-fault-tolerant committee.
const MIN_BFT_COMMITTEE: i64 = 4;

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

/// The export vote count for an equal-stake set of n validators: the closed form
/// `floor(2n/3) + 1`. For n = 21 this is 15, not 14 — 14/21 does not strictly
/// exceed two thirds.
pub fn equal_stake_quasar(n: i64) -> i64 {
    if n <= 0 {
        return 1;
    }
    two_thirds_stake_floor(n as u64) as i64 + 1
}

/// The minimum vote count that CAN reach the two-thirds-by-stake predicate for a
/// weight vector: order heaviest first and count until the running stake first
/// exceeds the floor. Returns 0 for an empty set or zero total — no stake model,
/// fail closed.
pub fn weighted_quasar(weights: &[u64]) -> usize {
    let total: u64 = weights.iter().copied().sum();
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

/// A validator identity: 20 bytes, the width `luxfi/ids.NodeID` is and the width
/// a certificate carries. It is not the 32-byte [`Id`] — a node that widened it
/// to 32 produced certificates no Lux node can parse.
pub type Node = [u8; 20];

/// The width of a node identity on the wire.
pub const NODE_LEN: usize = 20;

/// The fixed part of a certificate: everything before the vote records.
pub const CERT_HEADER_LEN: usize = 2 + 1 + 1 + 32 + 8 + 4 + 32 * 7 + 4 + 4;

/// The fixed part of one vote record: identity, the accept byte, the length prefix.
pub const CERT_VOTE_HEADER_LEN: usize = NODE_LEN + 1 + 4;

/// One signed record inside a certificate.
///
/// Distinct from the sampler's vote in `types`: that one names a block and a
/// vote kind and belongs to a round in flight, this one names a validator and
/// the signature it produced over the certificate's own position. They should be
/// one type — the sampler's is the older shape and does not carry what a
/// certificate has to carry.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Vote {
    pub node: Node,
    pub accept: bool,
    pub signature: Vec<u8>,
}

/// A finality certificate: a position, the rung it claims, and the signatures
/// that carry it there.
///
/// This is `engine/chain.QuorumCert`, field for field and byte for byte. The
/// signatures are individual and each is checked against the ONE message derived
/// from `position` — not a single aggregate blob, which cannot say who signed and
/// cannot be checked against a signer set.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Cert {
    pub version: u16,
    pub role: u8,
    pub tier: Finality,
    pub position: Position,
    pub threshold: u32,
    pub votes: Vec<Vote>,
}

/// Why a certificate is not finality. Every variant is a refusal; there is no
/// variant that means "probably fine".
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Refusal {
    /// The version is not the one this network certifies under.
    Version,
    /// The role byte is not a finality attestation.
    Role,
    /// A certificate attests Nova or Quasar. Nothing else is a rung a
    /// certificate can claim.
    Tier,
    /// A threshold of zero is a quorum of nobody.
    ThresholdZero,
    /// A certificate with no votes attests nothing.
    NoVotes,
    /// Node ids are not strictly increasing: a duplicate signer counted twice,
    /// or a re-ordering that changes the bytes without changing the meaning.
    Order,
    /// A finality certificate carries accept votes only.
    NotAccept,
    /// A signature does not verify against its signer's key over this
    /// certificate's own position.
    Signature,
    /// Fewer valid distinct accept votes than the threshold demands.
    BelowThreshold,
    /// The signers do not hold the stake this rung requires, or the validator
    /// set could not be resolved — which is the same answer.
    BelowStake,
    /// The bytes are not a certificate: short, over-long, or a length prefix
    /// that runs past the end.
    Wire,
}

/// Resolves a validator's key and checks one signature.
///
/// The message is handed in already derived from the certificate's position, so
/// an implementation cannot rebuild it from the vote and cannot be induced to
/// check a signature against a statement its signer never made. Returning false
/// for an unknown signer is the required behavior: an unregistered key is not a
/// valid signature.
pub trait Keys {
    fn verify(&self, node: &Node, message: &[u8], signature: &[u8]) -> bool;
}

impl<F: Fn(&Node, &[u8], &[u8]) -> bool> Keys for F {
    fn verify(&self, node: &Node, message: &[u8], signature: &[u8]) -> bool {
        self(node, message, signature)
    }
}

/// Supplies the weights a stake predicate is read in. Deterministic per height,
/// and zero for a validator outside the set at that height — so an outsider
/// contributes no stake and cannot inflate the numerator.
pub trait Stake {
    fn weight(&self, node: &Node, height: u64) -> u64;
    fn total(&self, height: u64) -> u64;
    fn count(&self, height: u64) -> i64;
}

impl Cert {
    /// Assemble a certificate from votes.
    ///
    /// Votes are sorted ascending by node id and duplicates are refused here
    /// rather than at verification, so a certificate that exists is already in
    /// canonical order and there is exactly one byte string for one set of
    /// votes.
    pub fn assemble(
        position: Position,
        tier: Finality,
        threshold: u32,
        votes: Vec<Vote>,
    ) -> Result<Self, Refusal> {
        if tier != Finality::Nova && tier != Finality::Quasar {
            return Err(Refusal::Tier);
        }
        if threshold == 0 {
            return Err(Refusal::ThresholdZero);
        }
        if votes.is_empty() {
            return Err(Refusal::NoVotes);
        }

        let mut votes = votes;
        for v in &votes {
            if !v.accept {
                return Err(Refusal::NotAccept);
            }
        }
        votes.sort_by(|a, b| a.node.cmp(&b.node));
        if votes.windows(2).any(|w| w[0].node == w[1].node) {
            return Err(Refusal::Order);
        }

        Ok(Cert {
            version: QUORUM_CERT_VERSION,
            role: QC_FINALITY,
            tier,
            position,
            threshold,
            votes,
        })
    }

    /// The exact bytes a certificate is gossiped as.
    ///
    /// Big-endian and fixed width throughout, then one record per vote:
    /// `node:20 accept:1 length:4 signature:length`.
    pub fn encode(&self) -> Vec<u8> {
        let mut buf = Vec::with_capacity(CERT_HEADER_LEN);
        buf.extend_from_slice(&self.version.to_be_bytes());
        buf.push(self.role);
        buf.push(self.tier as u8);
        buf.extend_from_slice(&self.position.chain_id);
        buf.extend_from_slice(&self.position.height.to_be_bytes());
        buf.extend_from_slice(&self.position.round.to_be_bytes());
        buf.extend_from_slice(&self.position.block_id);
        buf.extend_from_slice(&self.position.parent_id);
        buf.extend_from_slice(&self.position.canonical_id);
        buf.extend_from_slice(&self.position.parent_canonical_id);
        buf.extend_from_slice(&self.position.execution_state_root);
        buf.extend_from_slice(&self.position.payload_root);
        buf.extend_from_slice(&self.position.validator_set_root);
        buf.extend_from_slice(&self.threshold.to_be_bytes());
        buf.extend_from_slice(&(self.votes.len() as u32).to_be_bytes());
        for v in &self.votes {
            buf.extend_from_slice(&v.node);
            buf.push(if v.accept { 0x01 } else { 0x00 });
            buf.extend_from_slice(&(v.signature.len() as u32).to_be_bytes());
            buf.extend_from_slice(&v.signature);
        }
        buf
    }

    /// Read a certificate off the wire. Fail-closed on every short read, and a
    /// trailing byte is a refusal — an encoder that appends is not speaking this
    /// protocol, and accepting the remainder would make one certificate have
    /// many byte strings.
    pub fn decode(data: &[u8]) -> Result<Self, Refusal> {
        let mut r = Reader { buf: data, at: 0 };
        let version = r.u16()?;
        let role = r.u8()?;
        let tier = match r.u8()? {
            0 => Finality::Photon,
            1 => Finality::Wave,
            2 => Finality::Nova,
            3 => Finality::Quasar,
            4 => Finality::Horizon,
            _ => return Err(Refusal::Tier),
        };
        let position = Position {
            chain_id: r.id()?,
            height: r.u64()?,
            round: r.u32()?,
            block_id: r.id()?,
            parent_id: r.id()?,
            canonical_id: r.id()?,
            parent_canonical_id: r.id()?,
            execution_state_root: r.id()?,
            payload_root: r.id()?,
            validator_set_root: r.id()?,
        };
        let threshold = r.u32()?;
        let count = r.u32()? as usize;

        // A count is not a capacity. Each record is at least its fixed part, so
        // a count that cannot fit in what remains is refused before anything is
        // allocated for it.
        if count.saturating_mul(CERT_VOTE_HEADER_LEN) > data.len().saturating_sub(r.at) {
            return Err(Refusal::Wire);
        }
        let mut votes = Vec::with_capacity(count);
        for _ in 0..count {
            let node = r.node()?;
            let accept = match r.u8()? {
                0 => false,
                1 => true,
                _ => return Err(Refusal::Wire),
            };
            let len = r.u32()? as usize;
            votes.push(Vote { node, accept, signature: r.take(len)?.to_vec() });
        }
        if r.at != data.len() {
            return Err(Refusal::Wire);
        }
        Ok(Cert { version, role, tier, position, threshold, votes })
    }

    /// The one message every signature in this certificate is checked against,
    /// derived from the certificate's OWN position. A vote that signed a
    /// different position fails, which is the point.
    pub fn message(&self) -> Vec<u8> {
        canonical_vote_message(&self.position, true)
    }

    /// The structural and signature predicate, tier-agnostic.
    ///
    /// Every clause is a refusal, and there is no path that returns success
    /// without every signature having verified against a resolved key. The
    /// predicate this replaces returned true whenever the aggregate signature
    /// field was non-empty, so forty votes carrying no signatures at all
    /// certified a block.
    pub fn verify<K: Keys>(&self, keys: &K) -> Result<(), Refusal> {
        if self.version != QUORUM_CERT_VERSION {
            return Err(Refusal::Version);
        }
        if self.role != QC_FINALITY {
            return Err(Refusal::Role);
        }
        if self.tier != Finality::Nova && self.tier != Finality::Quasar {
            return Err(Refusal::Tier);
        }
        if self.threshold == 0 {
            return Err(Refusal::ThresholdZero);
        }
        if self.votes.is_empty() {
            return Err(Refusal::NoVotes);
        }

        let message = self.message();
        let mut count: u32 = 0;
        let mut prev: Option<Node> = None;
        for v in &self.votes {
            if let Some(p) = prev {
                if p >= v.node {
                    return Err(Refusal::Order);
                }
            }
            prev = Some(v.node);

            if !v.accept {
                return Err(Refusal::NotAccept);
            }
            if !keys.verify(&v.node, &message, &v.signature) {
                return Err(Refusal::Signature);
            }
            count += 1;
        }

        if count < self.threshold {
            return Err(Refusal::BelowThreshold);
        }
        Ok(())
    }

    /// The full finality predicate: [`Cert::verify`], then the stake this
    /// certificate's rung demands.
    ///
    /// The threshold is re-derived from the live validator set, never read from
    /// the certificate's own `threshold` field, so a certificate cannot forge
    /// its rung upward — a Nova set of votes relabeled Quasar fails the
    /// two-thirds check. An unresolved set fails closed: a majority of an
    /// unknown set is not a majority.
    pub fn verify_stake<K: Keys, S: Stake>(&self, keys: &K, stake: &S) -> Result<(), Refusal> {
        self.verify(keys)?;

        let height = self.position.height;
        let n = stake.count(height);
        let total = stake.total(height);
        if n < 1 || total == 0 {
            return Err(Refusal::BelowStake);
        }

        let mut held: u64 = 0;
        for v in &self.votes {
            held = held.saturating_add(stake.weight(&v.node, height));
        }

        match self.tier {
            Finality::Nova => {
                if (self.votes.len() as i64) < nova_signer_floor(n) {
                    return Err(Refusal::BelowThreshold);
                }
                if held <= half_stake_floor(total) {
                    return Err(Refusal::BelowStake);
                }
                Ok(())
            }
            Finality::Quasar => {
                if held <= two_thirds_stake_floor(total) {
                    return Err(Refusal::BelowStake);
                }
                Ok(())
            }
            _ => Err(Refusal::Tier),
        }
    }
}

/// A bounds-checked forward reader. Every read either advances or refuses; none
/// can run past the end.
struct Reader<'a> {
    buf: &'a [u8],
    at: usize,
}

impl<'a> Reader<'a> {
    fn take(&mut self, n: usize) -> Result<&'a [u8], Refusal> {
        let end = self.at.checked_add(n).ok_or(Refusal::Wire)?;
        if end > self.buf.len() {
            return Err(Refusal::Wire);
        }
        let out = &self.buf[self.at..end];
        self.at = end;
        Ok(out)
    }

    fn u8(&mut self) -> Result<u8, Refusal> {
        Ok(self.take(1)?[0])
    }

    fn u16(&mut self) -> Result<u16, Refusal> {
        let b = self.take(2)?;
        Ok(u16::from_be_bytes([b[0], b[1]]))
    }

    fn u32(&mut self) -> Result<u32, Refusal> {
        let b = self.take(4)?;
        Ok(u32::from_be_bytes([b[0], b[1], b[2], b[3]]))
    }

    fn u64(&mut self) -> Result<u64, Refusal> {
        let b = self.take(8)?;
        let mut a = [0u8; 8];
        a.copy_from_slice(b);
        Ok(u64::from_be_bytes(a))
    }

    fn id(&mut self) -> Result<Id, Refusal> {
        let b = self.take(32)?;
        let mut a = EMPTY;
        a.copy_from_slice(b);
        Ok(a)
    }

    fn node(&mut self) -> Result<Node, Refusal> {
        let b = self.take(NODE_LEN)?;
        let mut a = [0u8; NODE_LEN];
        a.copy_from_slice(b);
        Ok(a)
    }
}
