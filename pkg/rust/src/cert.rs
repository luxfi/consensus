// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The quorum certificate — the predicate that decides.
//!
//! A certificate is the only evidence that a block was accepted, so this module
//! is the whole of the accept rule. It is a port of
//! `github.com/luxfi/consensus/engine/chain.QuorumCert`, clause for clause, and
//! `tests/cert.rs` holds it there.
//!
//! The rule, stated once:
//!
//! > α distinct validators each produced a correctly signed ACCEPT over the same
//! > canonical position, the summed stake of those distinct voters strictly
//! > exceeds the tier's stake floor, and there are at least the tier's floor of
//! > them. Both rungs carry both clauses, because a threshold read only in stake
//! > reports one party wherever stake is concentrated in one.
//!
//! Two properties of the port are worth naming because they are what makes a
//! certificate evidence rather than decoration.
//!
//! **Fail closed by type.** Go's `Verify` opens with a nil check on the verifier,
//! because a Go interface can be nil and a certificate that nobody can check
//! must never pass. Here the verifier is a `&dyn VoteVerifier`, which cannot be
//! null, so that clause is discharged by the compiler and cannot regress. The
//! same holds for the stake source in [`QuorumCert::verify_weighted`].
//!
//! **The message comes from the certificate's own position, never from the
//! vote.** A vote carries a signature and nothing else to sign over. So a vote
//! that signed some other position does not verify here, and a certificate
//! cannot be assembled from votes cast for a different block, height or round.
//!
//! **A voter is a 20-byte NodeID, not a 32-byte block id.** [`NodeId`] is the
//! identity Go names a validator by, the identity votes are ordered on, and the
//! identity a proof of possession binds — one value, one width, at all three
//! places. Two widths would be two standards: the proof the network makes over
//! `node(20) ‖ key(48)` would not be the proof this crate checked.

use std::collections::HashMap;

use blst::min_pk::{PublicKey, Signature};
use blst::BLST_ERROR;

use crate::finality::{
    canonical_vote_message, half_stake_floor, signer_floor, two_thirds_stake_floor, Finality,
    Position, MIN_BFT_COMMITTEE, QC_FINALITY, QUORUM_CERT_VERSION,
};
use crate::pop::{self, PopError};

/// A validator is named by the 20-byte NodeID, re-exported from its one home in
/// [`crate::pop`] — the module that defines the proof binding a key to it.
///
/// It is NOT [`crate::finality::Id`], the 32-byte block identifier. Go's
/// `SignedVote.NodeID` is `ids.NodeID`, 20 bytes, and the proof of possession
/// signs `node ‖ key` over exactly those 20. A set that named its validators by
/// the 32-byte id would compute a different preimage from the same registrant,
/// so a proof the network accepts would be refused here and one it refuses could
/// pass — the two would not be the same standard. One width, from one home.
pub use crate::pop::NodeId;

/// The ciphersuite Lux signs consensus votes under, from `luxfi/crypto/bls`.
/// Public keys are compressed G1 (48 bytes) and signatures compressed G2 (96),
/// which is blst's `min_pk`.
///
/// The proof-of-possession ciphersuite is a separate tag and lives with the
/// proof, in [`crate::pop::POP_DST`]. Two homes for one domain tag is how the
/// two drift apart, so there is one.
pub const DST: &[u8] = b"BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_";

/// A compressed BLS public key, in bytes.
pub const PUBLIC_KEY_LEN: usize = 48;

/// A compressed BLS signature, in bytes.
pub const SIGNATURE_LEN: usize = 96;

/// One validator's signed opinion of one position.
///
/// The position is not carried here. It lives once, on the certificate, and the
/// message this signature is checked against is derived from it — so a vote is
/// meaningless outside the certificate that quotes it, which is what stops a
/// vote being replayed under a different position.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Vote {
    /// The signing validator, by its 20-byte NodeID. Votes in a certificate are
    /// strictly increasing on this field, which is both the distinctness clause
    /// and the canonical order — the same field and the same comparison Go
    /// sorts on, so a certificate ordered on one side is ordered on the other.
    pub node_id: NodeId,
    pub accept: bool,
    pub signature: Vec<u8>,
}

/// The first clause that failed. Every rejection names one; there is no bare
/// `false` anywhere in this module.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum CertError {
    Version { got: u16, want: u16 },
    Type { got: u8, want: u8 },
    UnknownTier(Finality),
    ThresholdZero,
    NoVotes,
    NotStrictlyIncreasing(usize),
    VoteNotAccept(usize),
    SigInvalid(usize),
    BelowThreshold { have: u32, need: u32 },
    UnresolvedSet { n: i64 },
    /// An EXPORT certificate over a signing set too small for a Byzantine
    /// supermajority to mean anything: f = ⌊(n−1)/3⌋ is zero below
    /// [`crate::finality::MIN_BFT_COMMITTEE`], so the certificate absorbs no fault
    /// and one compromised key forges it. A floor on the SET, not on the voters —
    /// Go folds it into `ErrQCBelowThreshold`.
    MinCommittee { n: i64, need: i64 },
    SignerFloor { have: i64, need: i64, n: i64 },
    StakeZero { epoch_height: u64 },
    StakeBelowMajority { voted: u64, signer: u64, need_above: u64 },
    StakeBelowSupermajority { voted: u64, signer: u64, need_above: u64 },
    /// A public key that does not decode to a non-identity point of the right
    /// subgroup.
    KeyEncoding,
    /// A registration carrying no public key at all. Not a malformed key: the
    /// caller wanted [`ValidatorSet::insert_unkeyed`], the door for a member
    /// this node holds no key for. Go's `ErrNoKey`.
    NoKey,
    /// A keyed validator admitted with no stake — a phantom signer, which
    /// raises the count of distinct signers a floor is read against without
    /// raising the weight. Go's `ErrZeroWeight`.
    ZeroWeight,
    /// A public key already registered to a node. A key belongs to at most one
    /// validator, so counting distinct voters counts distinct keys. Go's
    /// `ErrDuplicateKey`.
    DuplicateKey,
    /// A node id already registered. A node holds at most one key, so one
    /// operator cannot occupy several signer slots and several shares of the
    /// weight. Go's `ErrDuplicateNode`.
    DuplicateNode,
    /// A proof of possession that is not a valid signature by this key over its
    /// own (node, key) message under [`crate::pop::POP_DST`].
    PopInvalid,
    /// The certificate declares a quorum floor that is not the one this validator
    /// set derives for the rung it attests. A certificate may STATE its quorum; it
    /// may not choose it. Go's `ErrQCThresholdNotDerived`.
    ///
    /// Equality, not a lower bound: an over-claim asserts a quorum this set does
    /// not require, and tolerating it would let a certificate redefine the rung
    /// upward exactly as tolerating an under-claim lets it redefine the rung down.
    /// Neither direction is the set's number.
    ThresholdNotDerived { declared: u32, derived: i64, n: i64 },
    /// The set's weights sum past what a `u64` can hold. Go's
    /// `ErrWeightOverflow`, and refused for the reason Go refuses it: every
    /// threshold in this crate is read against the total, so a total that
    /// cannot be represented is a total two thirds of which means nothing.
    ///
    /// It is the clause that keeps the two sides of the weighted predicate
    /// honest. Were the sum merely clamped, a set summing past `u64::MAX`
    /// would report `u64::MAX` staked AND `u64::MAX` voted, and
    /// `u64::MAX > floor(2·u64::MAX/3)` — half the stake would read as an
    /// export supermajority. So the set is refused at the door instead, and
    /// the predicate never sees a number that is not the sum it claims to be.
    WeightOverflow,
}

impl std::fmt::Display for CertError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            CertError::Version { got, want } => write!(f, "cert version: got {got} want {want}"),
            CertError::Type { got, want } => write!(f, "cert type: got {got} want {want}"),
            CertError::UnknownTier(t) => write!(f, "cert tier: {} is not an accept tier", t.name()),
            CertError::ThresholdZero => write!(f, "cert threshold is zero"),
            CertError::NoVotes => write!(f, "cert carries no votes"),
            CertError::NotStrictlyIncreasing(i) => {
                write!(f, "cert votes not strictly increasing at vote {i}")
            }
            CertError::VoteNotAccept(i) => write!(f, "cert vote {i} is not an accept"),
            CertError::SigInvalid(i) => write!(f, "cert vote {i} signature does not verify"),
            CertError::BelowThreshold { have, need } => {
                write!(f, "cert below threshold: have {have} need {need}")
            }
            CertError::UnresolvedSet { n } => {
                write!(f, "cert over an unresolved validator set (n={n})")
            }
            CertError::MinCommittee { n, need } => write!(
                f,
                "quasar over {n} signers, need at least {need} — below the minimum Byzantine \
                 committee f=(n-1)/3 is 0 and a two-thirds supermajority tolerates no fault"
            ),
            CertError::SignerFloor { have, need, n } => {
                write!(f, "cert has {have} distinct voters, need {need} of {n}")
            }
            CertError::StakeZero { epoch_height } => {
                write!(f, "total stake is zero at epoch height {epoch_height}")
            }
            CertError::StakeBelowMajority { voted, signer, need_above } => {
                write!(f, "nova voted={voted} signer={signer}, need > {need_above}")
            }
            CertError::StakeBelowSupermajority { voted, signer, need_above } => {
                write!(f, "quasar voted={voted} signer={signer}, need > {need_above}")
            }
            CertError::KeyEncoding => write!(f, "public key does not decode to a valid point"),
            CertError::NoKey => write!(f, "registration carries no public key"),
            CertError::ZeroWeight => write!(f, "validator has zero weight"),
            CertError::DuplicateKey => write!(f, "public key is registered to more than one node"),
            CertError::DuplicateNode => write!(f, "node is registered more than once"),
            CertError::PopInvalid => write!(f, "proof of possession does not verify for this key"),
            CertError::ThresholdNotDerived { declared, derived, n } => write!(
                f,
                "cert declares threshold {declared}, a set of {n} signers derives {derived}"
            ),
            CertError::WeightOverflow => write!(f, "weight overflowed"),
        }
    }
}

impl std::error::Error for CertError {}

/// Resolves a voter's key and checks one signature.
///
/// Taken by reference throughout, so there is no "no verifier" case to handle:
/// a certificate cannot be verified without the means to check it.
pub trait VoteVerifier {
    /// Whether `signature` is `node`'s signature over `message` at the given
    /// P-chain epoch height. An unknown voter is a false, never an error — an
    /// unresolvable key is exactly as good as a bad signature.
    fn verify_vote(
        &self,
        node: &NodeId,
        message: &[u8],
        signature: &[u8],
        epoch_height: u64,
    ) -> bool;
}

/// The stake distribution at an epoch. Read at the epoch height the signatures
/// were cast under, never at the value-chain height.
///
/// **The set is the signers, and only the signers.** Every number here is read
/// over the validators that can actually produce a verifiable signature. A member
/// the chain carries without a key is a spectator: it holds stake and can never
/// cast a vote any verifier will accept. Counting it in the denominator raises the
/// bar for everyone who CAN sign, by stake no quorum is able to reach — past a
/// third of what the chain carries, `two_thirds_stake_floor` becomes unreachable
/// and the export rung is stranded with the whole signing set agreeing.
///
/// This is the ⅔ rule read over the right set, not a weakening of it. Quorum
/// intersection is untouched: two disjoint quorums each past ⅔ of the signer stake
/// would sum past the whole of it. What changes is that the fault budget is no
/// longer spent on parties known in advance to cast no vote — a key absent at the
/// epoch is a KNOWN non-participant, and BFT's third is a budget for unknown ones.
///
/// The three projections must describe ONE set at ONE height. A source answering
/// `signer_stake` over the signers and `signer_count` over every member states two
/// different sets, and the two floors stop being one supermajority in two units.
pub trait StakeSource {
    /// The voting weight of `node`, or 0 for a stranger, a member outside the set,
    /// or a member holding no key at this epoch — none of the three can put stake
    /// behind a vote, so none may inflate a tally.
    fn weight(&self, node: &NodeId, epoch_height: u64) -> u64;
    /// The stake held by validators that can sign — the denominator every stake
    /// floor is read against.
    fn signer_stake(&self, epoch_height: u64) -> u64;
    /// The number of distinct validators that can sign — the `n` every count floor
    /// is read against, over the same set as `signer_stake`.
    fn signer_count(&self, epoch_height: u64) -> i64;
}

/// A certificate: one position, and the distinct signed accepts that carry it.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct QuorumCert {
    pub version: u16,
    pub qc_type: u8,
    pub tier: Finality,
    pub position: Position,
    pub threshold: u32,
    pub votes: Vec<Vote>,
}

impl QuorumCert {
    /// Assemble a certificate over `position` from `votes`.
    ///
    /// Sorts by node id and drops non-accepts, so the result satisfies the
    /// ordering and accept clauses by construction. It does NOT check
    /// signatures — assembling is not accepting, and every consumer verifies.
    /// Fails only when the surviving votes cannot reach `threshold`.
    pub fn assemble(
        tier: Finality,
        position: Position,
        threshold: u32,
        votes: &[Vote],
    ) -> Result<Self, CertError> {
        if threshold == 0 {
            return Err(CertError::ThresholdZero);
        }

        let mut sorted: Vec<Vote> = votes.iter().filter(|v| v.accept).cloned().collect();
        sorted.sort_by_key(|a| a.node_id);
        sorted.dedup_by(|a, b| a.node_id == b.node_id);

        if (sorted.len() as u64) < threshold as u64 {
            return Err(CertError::BelowThreshold {
                have: sorted.len() as u32,
                need: threshold,
            });
        }

        Ok(QuorumCert {
            version: QUORUM_CERT_VERSION,
            qc_type: QC_FINALITY,
            tier,
            position,
            threshold,
            votes: sorted,
        })
    }

    /// The number of distinct voters. The ordering clause makes distinctness a
    /// property of a verified certificate, so this is a length.
    pub fn voter_count(&self) -> i64 {
        self.votes.len() as i64
    }

    /// The exact bytes every vote in this certificate must have signed.
    pub fn message(&self) -> Vec<u8> {
        canonical_vote_message(&self.position, true)
    }

    /// The count-only predicate: every structural clause, plus a valid signature
    /// from each of at least `threshold` distinct voters.
    ///
    /// A CERTIFICATE IS A RECORD, NOT A CAPABILITY. Every field on it is public
    /// and anyone may build one that says anything; holding one authorises
    /// nothing. What authorises is [`Self::verify_weighted`], and EVERY acceptor
    /// runs it — this, the count-only predicate, is the structural pre-filter
    /// inside it.
    ///
    /// So it is NOT a standalone accept rule, and must not be used as one. The
    /// `threshold` it counts against is a field of the certificate — the
    /// certificate names its own quorum — so on its own this clears a 1-of-n
    /// certificate that declares `threshold: 1`. It is a building block:
    /// [`Self::verify_weighted`] is the accept rule, recomputing the floor from
    /// the live stake set so a certificate cannot declare its own quorum. Call
    /// `verify` directly only where the caller itself supplies and checks the
    /// admissible threshold against the set — never as the whole of accept.
    ///
    /// Clauses, in the order Go checks them:
    ///
    /// 0. a verifier exists — discharged by the type
    /// 1. version and type match
    /// 2. tier is an accept tier
    /// 3. threshold is non-zero, and there is at least one vote
    /// 4. node ids strictly increase — distinct, canonically ordered
    /// 5. every vote is an ACCEPT
    /// 6. every signature verifies over the certificate's own position
    /// 7. the count of such votes meets the threshold
    pub fn verify(
        &self,
        verifier: &dyn VoteVerifier,
        epoch_height: u64,
    ) -> Result<(), CertError> {
        if self.version != QUORUM_CERT_VERSION {
            return Err(CertError::Version {
                got: self.version,
                want: QUORUM_CERT_VERSION,
            });
        }
        if self.qc_type != QC_FINALITY {
            return Err(CertError::Type {
                got: self.qc_type,
                want: QC_FINALITY,
            });
        }
        // A certificate attests exactly one accept tier. A wire-decoded cert
        // with a garbage tier byte is rejected before any signature work, so
        // the count-only path fails closed on it too — not just the weighted one.
        if self.tier != Finality::Nova && self.tier != Finality::Quasar {
            return Err(CertError::UnknownTier(self.tier));
        }
        if self.threshold == 0 {
            return Err(CertError::ThresholdZero);
        }
        if self.votes.is_empty() {
            return Err(CertError::NoVotes);
        }

        let message = self.message();

        let mut count: u32 = 0;
        let mut prev: Option<&NodeId> = None;
        for (i, v) in self.votes.iter().enumerate() {
            // Clause 4: strictly increasing ids, compared as the 20 bytes Go
            // compares. Distinctness and canonical order in one comparison —
            // this is what stops one validator being counted twice, and stops a
            // cert being re-ordered into a new one.
            if let Some(p) = prev {
                if p >= &v.node_id {
                    return Err(CertError::NotStrictlyIncreasing(i));
                }
            }
            prev = Some(&v.node_id);

            // Clause 5: a finality certificate carries accepts only.
            if !v.accept {
                return Err(CertError::VoteNotAccept(i));
            }

            // Clause 6: the message is derived from THIS certificate's position,
            // so a vote cast over any other position fails here.
            if !verifier.verify_vote(&v.node_id, &message, &v.signature, epoch_height) {
                return Err(CertError::SigInvalid(i));
            }

            count += 1;
        }

        if count < self.threshold {
            return Err(CertError::BelowThreshold {
                have: count,
                need: self.threshold,
            });
        }
        Ok(())
    }

    /// The full predicate: [`Self::verify`], then the DERIVED authority — the
    /// certificate's declared threshold must equal [`signer_floor`] for its rung
    /// over this set — then the rung's stake and signer floors, all recomputed
    /// from the live set so a certificate can never declare its own.
    pub fn verify_weighted(
        &self,
        verifier: &dyn VoteVerifier,
        stake: &dyn StakeSource,
        epoch_height: u64,
    ) -> Result<(), CertError> {
        self.verify(verifier, epoch_height)?;
        // DERIVED AUTHORITY. The certificate states the quorum it was built to; the
        // SET decides what that quorum is. The two must be the same number, so the
        // field carries no authority of its own — it is a claim this clause checks,
        // never a value the verifier adopts.
        //
        // Without it the field is load-bearing on the count-only road: `verify`'s
        // last clause counts distinct valid accepts against the certificate's OWN
        // threshold, so one declaring 1 clears it on a single signature. The floors
        // below then catch that, which is why this was a divergence rather than a
        // forgery — but only Go and Rust asked the weaker question, while C++ has
        // always demanded the derived floor exactly. One rule now.
        //
        // An unresolved set (n < 1) derives no floor, so nothing is compared against
        // it and the rung's own clause refuses the certificate under the name it has
        // always been refused by. Skipping here is not a pass: both rungs fail closed
        // on n < 1.
        let n = stake.signer_count(epoch_height);
        if n >= 1 {
            let derived = signer_floor(self.tier, n);
            if i64::from(self.threshold) != derived {
                return Err(CertError::ThresholdNotDerived {
                    declared: self.threshold,
                    derived,
                    n,
                });
            }
        }
        match self.tier {
            Finality::Nova => self.verify_nova_majority(stake, epoch_height),
            Finality::Quasar => self.verify_quasar_supermajority(stake, epoch_height),
            other => Err(CertError::UnknownTier(other)),
        }
    }

    /// Nova: a strict majority of stake, and at least `nova_signer_floor(n)`
    /// distinct signers.
    ///
    /// The two are independent and neither is sufficient. Stake majority alone
    /// would let a single holder of a stake majority self-ignite; the signer
    /// floor is the guard the stake predicate cannot give. An unresolved set
    /// fails closed — a majority of an unknown set is not a statement.
    fn verify_nova_majority(
        &self,
        stake: &dyn StakeSource,
        epoch_height: u64,
    ) -> Result<(), CertError> {
        let n = stake.signer_count(epoch_height);
        if n < 1 {
            return Err(CertError::UnresolvedSet { n });
        }
        let floor = signer_floor(Finality::Nova, n);
        if self.voter_count() < floor {
            return Err(CertError::SignerFloor {
                have: self.voter_count(),
                need: floor,
                n,
            });
        }
        let signer = stake.signer_stake(epoch_height);
        if signer == 0 {
            return Err(CertError::StakeZero { epoch_height });
        }
        let voted = self.voted_stake(stake, epoch_height)?;
        let half = half_stake_floor(signer);
        if voted <= half {
            return Err(CertError::StakeBelowMajority {
                voted,
                signer,
                need_above: half,
            });
        }
        Ok(())
    }

    /// Quasar: the summed stake of the distinct voters strictly exceeds
    /// `floor(2·signer/3)`, AND there are at least `two_thirds_count(n)` of them,
    /// AND `n` is at least [`MIN_BFT_COMMITTEE`]. This is the export threshold —
    /// the only rung a bridge, a settlement, or a cross-chain message may read.
    ///
    /// The third clause is a floor on the SET rather than on the voters, and the
    /// other two do not imply it: a supermajority is only a Byzantine claim when
    /// there is a fault budget for it to spend.
    ///
    /// Two independent predicates, neither sufficient alone, the same shape as
    /// Nova one rung up. Stake alone is not export-grade finality: a single
    /// validator holding two thirds of the stake clears the stake floor on its
    /// OWN signature, and a certificate one key can mint is not a claim that a
    /// Byzantine supermajority of independent parties agreed. The count is the
    /// guard the stake predicate cannot give, and it is the SAME supermajority
    /// read in seats rather than in stake — derived from the same arithmetic, so
    /// the two halves of the export rule cannot drift apart.
    ///
    /// Stake is read first. Both clauses must hold, so the order decides nothing
    /// but which one a refusal is NAMED by, and on an equal-stake set the two are
    /// the same bar in two units and bind together — where a stake refusal says
    /// how far short the quorum fell. The count clause can bind ALONE only where
    /// the weights are lopsided, so reaching it means the stake was there and the
    /// signers were not.
    fn verify_quasar_supermajority(
        &self,
        stake: &dyn StakeSource,
        epoch_height: u64,
    ) -> Result<(), CertError> {
        let signer = stake.signer_stake(epoch_height);
        if signer == 0 {
            return Err(CertError::StakeZero { epoch_height });
        }
        let voted = self.voted_stake(stake, epoch_height)?;
        let floor = two_thirds_stake_floor(signer);
        if voted <= floor {
            return Err(CertError::StakeBelowSupermajority {
                voted,
                signer,
                need_above: floor,
            });
        }
        // An unresolved set fails closed, and does so here rather than at the top:
        // a source reporting no validators alongside no stake is already refused
        // above for the reason it has always been refused for, and only a source
        // reporting stake for a set it says is empty reaches this line. Two thirds
        // of no set is not a number, and `two_thirds_count(0)` is 1, which would
        // hand a lone signer a floor of one.
        let n = stake.signer_count(epoch_height);
        if n < 1 {
            return Err(CertError::UnresolvedSet { n });
        }
        // The export rung's floor on the SET, which is a different quantity from
        // its floor on the voters and is not implied by it. Byzantine tolerance is
        // f = (n-1)/3, and that is 0 for n of one, two and three: below four
        // signers a two-thirds supermajority tolerates no fault at all. Every
        // signer is load-bearing, so a single compromised key is not one fault
        // absorbed by a margin, it is a forged export certificate.
        //
        // The count clause below cannot catch it, because it is the supermajority
        // of whatever set it is handed: `two_thirds_count(1)` is 1, so one
        // signature over a one-signer set clears the count and the stake floors
        // together. Reading both floors over the signers is what stops a spectator
        // stranding a chain; it is not a way for a chain with three signers to
        // export, and this is the clause that says so.
        //
        // Nova has no such floor and must not grow one: it authorizes only local
        // execution the chain can still reorg away.
        if n < MIN_BFT_COMMITTEE {
            return Err(CertError::MinCommittee {
                n,
                need: MIN_BFT_COMMITTEE,
            });
        }
        let need = signer_floor(Finality::Quasar, n);
        if self.voter_count() < need {
            return Err(CertError::SignerFloor {
                have: self.voter_count(),
                need,
                n,
            });
        }
        Ok(())
    }

    /// Summed stake of the certificate's voters — checked, and refused rather
    /// than clamped.
    ///
    /// The voters are a subset of the members, so against any set this crate
    /// admitted this sum is bounded by a total [`ValidatorSet::insert`] already
    /// proved representable, and the overflow is unreachable. Reaching it is
    /// therefore evidence about the [`StakeSource`], not about the votes: it
    /// says the source is reporting weights that no admitted set could hold, and
    /// a source that cannot state its own total is not one a threshold can be
    /// read against.
    ///
    /// It must not clamp. Clamping pins BOTH sides of the predicate to
    /// `u64::MAX`, and `u64::MAX > floor(2·u64::MAX/3)` — so a certificate
    /// carrying half the stake would clear the export floor. Monotone is not
    /// the property that matters here; being the sum it claims to be is.
    fn voted_stake(&self, stake: &dyn StakeSource, epoch_height: u64) -> Result<u64, CertError> {
        self.votes.iter().try_fold(0u64, |acc, v| {
            acc.checked_add(stake.weight(&v.node_id, epoch_height))
                .ok_or(CertError::WeightOverflow)
        })
    }
}

/// One validator asking to be counted: the identity it claims, the key it will
/// sign under, the proof binding the two, and the weight staked behind it.
///
/// The field-for-field twin of Go's `validators.Registration`, and the input to
/// [`ValidatorSet::register`] — the door for a set that arrives whole.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Registration {
    /// The identity claimed, by the 20-byte NodeID the proof is bound to.
    pub node: NodeId,
    /// The BLS12-381 min_pk public key: compressed G1, 48 bytes.
    pub public_key: Vec<u8>,
    /// The node-bound proof of possession over `node ‖ key`: compressed G2, 96
    /// bytes. See [`crate::pop`] for the preimage and the domain.
    pub proof: Vec<u8>,
    pub weight: u64,
}

/// A validator set: who is a member, what stake each carries, and the keys
/// those members sign with.
///
/// One structure answers all three because they are read together and at the
/// same epoch — splitting them is how a node ends up verifying a signature
/// against one set and weighing it against another.
///
/// Membership and key material are separate facts, and a member may have no
/// key. Such a member counts toward the set size and toward total stake, which
/// is what raises the bar for everyone else, but it can never produce a
/// verified signature and so can never appear in a verified certificate. That
/// is the fail-closed direction: a missing key withholds a vote, it never
/// admits one.
///
/// **A set is a set on both axes.** A key belongs to at most one node and a node
/// to at most one key, enforced at admission — see [`ValidatorSet::insert`]. That
/// is what makes `nova_signer_floor` a floor on distinct SIGNERS: without the key
/// axis one secret registered under many ids clears a floor written to require
/// many holders, and without the node axis one operator takes many signer slots
/// and many shares of the weight under many proven keys, none of which possession
/// can object to.
///
/// **There is no aggregate verification here, and there must not be.** The
/// network keeps one signature per voter and checks each against exactly one
/// key — the form Go reads — so a certificate stays interoperable. The
/// rogue-key attack on a summed key is closed twice over: [`ValidatorSet::insert`]
/// now demands a proof of possession, so `g1·x − Σ pk_others` cannot be
/// registered in the first place, having no secret to prove; and even were it
/// present, each signature is checked against its own key, so it stands for no
/// one but itself. `tests/rogue_key.rs` builds exactly that key and shows both:
/// it is refused at registration, and its holder cannot sign under it.
///
/// Go does not aggregate either — `engine/chain/cert.go`: "there is no
/// aggregate field, because nothing is aggregated" — so an aggregate accept
/// rule would also be an accept this network cannot express.
#[derive(Clone, Debug, Default)]
pub struct ValidatorSet {
    keys: HashMap<NodeId, PublicKey>,
    weights: HashMap<NodeId, u64>,
    /// The node each public key belongs to, so a key registered to one validator
    /// cannot be registered to a second. Without it, `nova_signer_floor` bounds
    /// distinct node ids and not distinct signers, and one secret key registered
    /// under many ids clears a floor written to require many.
    ///
    /// Keyed on the canonical bytes re-derived from the DECODED point, never on
    /// the caller's input, so two spellings of one key cannot occupy two slots.
    owner: HashMap<[u8; PUBLIC_KEY_LEN], NodeId>,
    /// The summed weight of every member, keyed or not.
    ///
    /// Established one admission at a time and never recomputed, because it is
    /// the admission that decides whether it can be represented at all — see
    /// the WEIGHT clause of [`ValidatorSet::insert`]. A total folded on demand
    /// has to decide what to do about a sum it cannot hold, at a place with no
    /// registration left to refuse; a total accumulated at the door already
    /// knows, and the set that would have overflowed does not exist.
    total: u64,
    /// The summed weight of the members that hold a key — the denominator every
    /// threshold is read against, and never `total`.
    ///
    /// Kept beside `total` rather than derived from it, because the two answer
    /// different questions and only one of them is a floor. `total` is what
    /// admission checks for representability: a set is refused when the whole of
    /// its weight cannot be held, whether or not the seat that overflows it can
    /// sign. `signable` is what a quorum is measured against. Folding one out of
    /// the other would tie a safety number to an admission number and let a
    /// keyless registration move a floor.
    signable: u64,
}

impl ValidatorSet {
    pub fn new() -> Self {
        Self::default()
    }

    /// Admit a whole set of registrations, or none of them. This is the port of
    /// Go's `validators.Register`, and the way to build a set from input this
    /// node did not produce — a genesis file, a peer, a chain read.
    ///
    /// [`ValidatorSet::insert`] admits ONE validator and reports on that one, so
    /// it cannot give a set any property. A caller that inserts in a loop and
    /// logs what failed is left holding a set that is missing a member: `n` is
    /// short by one and the total is short by that member's stake, and those two
    /// numbers are what every threshold in this crate is read against. Nothing
    /// announces the loss — the set is well formed, just not the set that was
    /// registered — so two nodes fed the same registrations disagree about the
    /// floor while both believe they agree.
    ///
    /// So the set is admitted whole or not at all. One inadmissible registration
    /// fails the call and no partial set escapes: the set under construction is
    /// a local, and the caller receives either all of it or the first clause
    /// that failed.
    ///
    /// Registrations are sorted by node id before any is checked, exactly as Go
    /// sorts them and with the same stable sort, so WHICH duplicate a set is
    /// refused on does not depend on the order a caller happened to build its
    /// input. One bad set is one error, everywhere.
    ///
    /// The input is taken by value. Go clones its slice so the sort cannot be
    /// seen by the caller; here the caller has given the registrations up, so
    /// there is no caller-visible slice to protect and no copy to make.
    ///
    /// Every registration must carry a key: this is the proof path, and Go's
    /// `Register` refuses a keyless one for the same reason. A member this node
    /// holds no key for comes in through [`ValidatorSet::insert_unkeyed`].
    pub fn register(mut registrations: Vec<Registration>) -> Result<Self, CertError> {
        registrations.sort_by_key(|r| r.node);
        let mut set = Self::new();
        for r in &registrations {
            set.insert(r.node, r.weight, &r.public_key, &r.proof)?;
        }
        Ok(set)
    }

    /// Admit a validator: the identity it claims, the key it will sign under,
    /// the proof binding the two, and the weight staked behind it.
    ///
    /// This is the admission rule of the standard — the port of Go's
    /// `validators.Register`, clause for clause and in its order, so a
    /// registration the network admits is admitted here and one it refuses is
    /// refused here *for the same reason*:
    ///
    /// ```text
    ///   NO KEY       a registration with no key wanted `insert_unkeyed`
    ///   ZERO WEIGHT  a keyed signer with no stake is a phantom signer
    ///   ENCODING     the key is a canonical compressed BLS12-381 G1 point
    ///   POSSESSION   a node-bound proof binds THIS key to THIS node
    ///   UNIQUENESS   the key is registered to no node, and the node to no key
    /// ```
    ///
    /// The order is not decoration. A pairing check on bytes that are not a
    /// point is undefined, so encoding precedes possession; and the two O(1)
    /// clauses precede the pairing, so a peer cannot spend this node's time on
    /// a registration that was inadmissible on its face.
    ///
    /// **Uniqueness is a set on both axes, and neither implies the other.**
    /// Possession does not catch one operator registering two proven keys under
    /// two ids — each proof is genuine — and that is N signer indices and N
    /// shares of the weight for one holder. Nor does it catch a node claiming a
    /// key already counted. So both are refused, key first, exactly as Go
    /// iterates them: an identical (node, key) offered twice is a
    /// [`CertError::DuplicateKey`], and the same node under a second key is a
    /// [`CertError::DuplicateNode`].
    ///
    /// A node is therefore admitted exactly once. Re-keying is not a silent
    /// mutation of a live set — it is [`ValidatorSet::remove`] and then a fresh
    /// admission, which is the only spelling that cannot change a member's key
    /// or weight behind a caller that thought it was adding one.
    ///
    /// Nothing is written unless every clause passes: a refused registration
    /// leaves no membership, no key and no ownership behind. That is a promise
    /// about THIS registration and only this one — the set is left exactly as it
    /// was, which for a caller admitting several in a loop means it is left
    /// holding the ones that already went in. A set built from input this node
    /// did not produce wants [`ValidatorSet::register`], which is all-or-nothing
    /// over the whole set; this door is the incremental one, for a membership
    /// change to a set that is already live.
    pub fn insert(
        &mut self,
        node: NodeId,
        weight: u64,
        public_key: &[u8],
        proof: &[u8],
    ) -> Result<(), CertError> {
        // NO KEY. Not a malformed key — no key. A validator with no key cannot
        // sign, so it cannot come through the proof path at all.
        if public_key.is_empty() {
            return Err(CertError::NoKey);
        }
        // ZERO WEIGHT. A keyed validator with no stake counts toward the number
        // of signers and not toward the weight, which is the same disagreement
        // between "how many signed" and "how much signed" that the weighted
        // predicate refuses downstream. Refuse it at the door instead.
        if weight == 0 {
            return Err(CertError::ZeroWeight);
        }
        // ENCODING, then POSSESSION — both inside `pop::verify`, in that order,
        // and it is the SAME function the Go oracle's frozen vectors pin. There
        // is one proof-of-possession implementation in this crate; registration
        // calls it rather than restating it, so the two cannot drift.
        pop::verify(&node, public_key, proof).map_err(|e| match e {
            PopError::Key => CertError::KeyEncoding,
            PopError::Proof | PopError::Possession => CertError::PopInvalid,
        })?;
        // Decoded again here, and deliberately: `pop` owns the proof, this owns
        // the set. The bytes the set keys on are re-derived from the point, so
        // ownership is decided on the one canonical spelling of a key however
        // the caller spelled it.
        let pk = PublicKey::key_validate(public_key).map_err(|_| CertError::KeyEncoding)?;
        let canonical = pk.compress();

        // UNIQUENESS OF KEY. One key, one node.
        if self.owner.contains_key(&canonical) {
            return Err(CertError::DuplicateKey);
        }
        // UNIQUENESS OF NODE. One node, one key — on membership, so a node
        // already admitted without a key cannot be admitted again with one.
        if self.weights.contains_key(&node) {
            return Err(CertError::DuplicateNode);
        }
        // WEIGHT, last — the clause Go runs last, for the reason it runs it at
        // all. `checked_add`, so a set whose weights sum past `u64::MAX` is
        // refused here rather than clamped downstream, where clamping both the
        // total and the votes pins them equal and lets half the stake read as
        // two thirds of it.
        let total = self
            .total
            .checked_add(weight)
            .ok_or(CertError::WeightOverflow)?;

        // `signable` cannot overflow independently: it is a sub-sum of `total`,
        // which the clause above has just proved representable.
        let signable = self.signable + weight;

        self.keys.insert(node, pk);
        self.weights.insert(node, weight);
        self.owner.insert(canonical, node);
        self.total = total;
        self.signable = signable;
        Ok(())
    }

    /// Admit a validator whose signing key this node does not have.
    ///
    /// It is a member and it holds stake, and it can never produce a signature
    /// this set will accept. That is the fail-closed direction: a missing key
    /// withholds a vote, it never admits one. Go's `FlattenValidatorSet` carries
    /// exactly these.
    ///
    /// It moves NO floor. It is absent from `signer_stake` and from
    /// `signer_count`, so it neither raises the stake a quorum must reach nor the
    /// number of signatures it must carry. A spectator that could move either
    /// would be raising a bar it is unable to help clear, and past a third of the
    /// weight it would put the export rung permanently out of reach — the whole
    /// signing set could agree and the certificate would still be refused.
    ///
    /// One node, one admission holds here too, so this cannot quietly de-key a
    /// member that already has one, nor restate its weight.
    pub fn insert_unkeyed(&mut self, node: NodeId, weight: u64) -> Result<(), CertError> {
        if self.weights.contains_key(&node) {
            return Err(CertError::DuplicateNode);
        }
        // The same WEIGHT clause. An unkeyed member cannot sign, but its stake is
        // part of what the set carries, and a set whose weights cannot be held is
        // refused whether or not the seat that overflows it can sign — Go's
        // `FlattenValidatorSet` adds it and checks the same overflow. `signable`
        // is deliberately untouched: representability and quorum are different
        // questions asked of different numbers.
        let total = self
            .total
            .checked_add(weight)
            .ok_or(CertError::WeightOverflow)?;
        self.weights.insert(node, weight);
        self.total = total;
        Ok(())
    }

    /// Retract a validator: its membership, its stake, and its claim on its key,
    /// which is freed for nobody in particular — a second node still has to
    /// prove possession to take it, and the proof it would need is bound to
    /// *its own* id, so freeing a key hands nothing to anyone.
    ///
    /// This is the one retraction, and so the one half of a re-key.
    pub fn remove(&mut self, node: &NodeId) {
        // Whether it could sign has to be read BEFORE the key is dropped, because
        // it decides which of the two sums loses this weight.
        let was_keyed = self.keys.contains_key(node);
        if let Some(old) = self.keys.remove(node) {
            self.owner.remove(&old.compress());
        }
        // Each sum is the weight still present under it, so each loses exactly
        // what its map loses. The weight was added when the member was admitted,
        // so both subtractions are exact and neither can go below zero.
        if let Some(weight) = self.weights.remove(node) {
            self.total -= weight;
            if was_keyed {
                self.signable -= weight;
            }
        }
    }

    /// The number of members, keyed or not.
    ///
    /// This is the membership roll, and it is deliberately NOT the `n` a floor is
    /// read against — that is `StakeSource::signer_count`, over the seats that can
    /// sign. The two differ by exactly the members admitted through
    /// [`ValidatorSet::insert_unkeyed`].
    pub fn len(&self) -> usize {
        self.weights.len()
    }

    /// The summed weight of every member, keyed or not — what the set CARRIES.
    ///
    /// It is the number admission checks for representability: a set whose weights
    /// cannot be held is refused whether or not the seat that overflows it can
    /// sign. It is NOT a floor, and no threshold is read against it — see
    /// `StakeSource::signer_stake` for the one that is.
    pub fn carried(&self) -> u64 {
        self.total
    }

    pub fn is_empty(&self) -> bool {
        self.weights.is_empty()
    }

    /// Whether `node` is a member. Membership decides whose ballot is counted;
    /// it does not decide whose signature verifies.
    pub fn contains(&self, node: &NodeId) -> bool {
        self.weights.contains_key(node)
    }

    /// Whether `node` has a key here, and so can contribute to a certificate.
    pub fn can_verify(&self, node: &NodeId) -> bool {
        self.keys.contains_key(node)
    }

    pub fn public_key(&self, node: &NodeId) -> Option<&PublicKey> {
        self.keys.get(node)
    }
}

/// The set verifies its own members' votes. A voter outside the set is false —
/// there is no key to check against, and an unresolvable key is exactly as good
/// as a bad signature.
impl VoteVerifier for ValidatorSet {
    fn verify_vote(
        &self,
        node: &NodeId,
        message: &[u8],
        signature: &[u8],
        _epoch_height: u64,
    ) -> bool {
        if signature.len() != SIGNATURE_LEN {
            return false;
        }
        let pk = match self.keys.get(node) {
            Some(pk) => pk,
            None => return false,
        };
        let sig = match Signature::uncompress(signature) {
            Ok(s) => s,
            Err(_) => return false,
        };
        sig.verify(true, message, DST, &[], pk, true) == BLST_ERROR::BLST_SUCCESS
    }
}

impl StakeSource for ValidatorSet {
    /// A keyless member weighs nothing here. It is a member — `len` counts it and
    /// `contains` finds it — but it cannot put stake behind a vote, and a tally is
    /// a sum over votes.
    fn weight(&self, node: &NodeId, _epoch_height: u64) -> u64 {
        if !self.keys.contains_key(node) {
            return 0;
        }
        self.weights.get(node).copied().unwrap_or(0)
    }

    /// The stake behind the keys: `total` less whatever came in through
    /// [`ValidatorSet::insert_unkeyed`].
    fn signer_stake(&self, _epoch_height: u64) -> u64 {
        self.signable
    }

    /// The seats that can sign. `len` is the membership and is deliberately not
    /// this: a count floor read over members would demand more signatures than the
    /// set is able to produce.
    fn signer_count(&self, _epoch_height: u64) -> i64 {
        self.keys.len() as i64
    }
}
