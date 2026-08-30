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
//! > canonical position, and the summed stake of those distinct voters strictly
//! > exceeds the tier's floor.
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

use std::collections::HashMap;

use blst::min_pk::{PublicKey, Signature};
use blst::BLST_ERROR;

use crate::finality::{
    canonical_vote_message, half_stake_floor, nova_signer_floor, two_thirds_stake_floor, Finality,
    Id, Position, QC_FINALITY, QUORUM_CERT_VERSION,
};

/// The ciphersuite Lux signs consensus votes under, from `luxfi/crypto/bls`.
/// Public keys are compressed G1 (48 bytes) and signatures compressed G2 (96),
/// which is blst's `min_pk`.
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
    pub node_id: Id,
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
    SignerFloor { have: i64, need: i64, n: i64 },
    StakeZero { epoch_height: u64 },
    StakeBelowMajority { voted: u64, total: u64, need_above: u64 },
    StakeBelowSupermajority { voted: u64, total: u64, need_above: u64 },
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
            CertError::SignerFloor { have, need, n } => {
                write!(f, "cert has {have} distinct voters, need {need} of {n}")
            }
            CertError::StakeZero { epoch_height } => {
                write!(f, "total stake is zero at epoch height {epoch_height}")
            }
            CertError::StakeBelowMajority { voted, total, need_above } => {
                write!(f, "nova voted={voted} total={total}, need > {need_above}")
            }
            CertError::StakeBelowSupermajority { voted, total, need_above } => {
                write!(f, "quasar voted={voted} total={total}, need > {need_above}")
            }
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
    fn verify_vote(&self, node: &Id, message: &[u8], signature: &[u8], epoch_height: u64) -> bool;
}

/// The stake distribution at an epoch. Read at the epoch height the signatures
/// were cast under, never at the value-chain height.
pub trait StakeSource {
    fn weight(&self, node: &Id, epoch_height: u64) -> u64;
    fn total_stake(&self, epoch_height: u64) -> u64;
    fn validator_count(&self, epoch_height: u64) -> i64;
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
    /// This is the whole rule on an equal-stake chain. Where weights differ,
    /// [`Self::verify_weighted`] adds the stake floor, and a count alone is not
    /// an accept.
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
        let mut prev: Option<&Id> = None;
        for (i, v) in self.votes.iter().enumerate() {
            // Clause 4: strictly increasing ids. Distinctness and canonical
            // order in one comparison — this is what stops one validator being
            // counted twice, and stops a cert being re-ordered into a new one.
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

    /// The full predicate: [`Self::verify`], then the tier's stake floor,
    /// recomputed from the live set so a certificate can never declare its own.
    pub fn verify_weighted(
        &self,
        verifier: &dyn VoteVerifier,
        stake: &dyn StakeSource,
        epoch_height: u64,
    ) -> Result<(), CertError> {
        self.verify(verifier, epoch_height)?;
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
        let n = stake.validator_count(epoch_height);
        if n < 1 {
            return Err(CertError::UnresolvedSet { n });
        }
        let floor = nova_signer_floor(n);
        if self.voter_count() < floor {
            return Err(CertError::SignerFloor {
                have: self.voter_count(),
                need: floor,
                n,
            });
        }
        let total = stake.total_stake(epoch_height);
        if total == 0 {
            return Err(CertError::StakeZero { epoch_height });
        }
        let voted = self.voted_stake(stake, epoch_height);
        let half = half_stake_floor(total);
        if voted <= half {
            return Err(CertError::StakeBelowMajority {
                voted,
                total,
                need_above: half,
            });
        }
        Ok(())
    }

    /// Quasar: the summed stake of the distinct voters strictly exceeds
    /// `floor(2·total/3)`. This is the export threshold — the only rung a
    /// bridge, a settlement, or a cross-chain message may read.
    fn verify_quasar_supermajority(
        &self,
        stake: &dyn StakeSource,
        epoch_height: u64,
    ) -> Result<(), CertError> {
        let total = stake.total_stake(epoch_height);
        if total == 0 {
            return Err(CertError::StakeZero { epoch_height });
        }
        let voted = self.voted_stake(stake, epoch_height);
        let floor = two_thirds_stake_floor(total);
        if voted <= floor {
            return Err(CertError::StakeBelowSupermajority {
                voted,
                total,
                need_above: floor,
            });
        }
        Ok(())
    }

    /// Summed stake of the certificate's voters, saturating.
    ///
    /// Saturating and not wrapping: a set whose weights sum past `u64::MAX`
    /// must not wrap to a small number and read as below-threshold, nor wrap
    /// past a floor. It is only reached on a set that cannot exist, and it
    /// stays monotone if it ever is.
    fn voted_stake(&self, stake: &dyn StakeSource, epoch_height: u64) -> u64 {
        self.votes.iter().fold(0u64, |acc, v| {
            acc.saturating_add(stake.weight(&v.node_id, epoch_height))
        })
    }
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
/// **There is no aggregate verification here, and there must not be.** Summing
/// the signers' keys and checking one signature against the sum is only sound
/// when every registrant proved possession of its secret. Registration proves
/// no such thing — see [`ValidatorSet::insert`] — so an attacker who registers
/// `g1·x − Σ pk_others` makes the sum collapse to a key it alone can sign
/// under, and one signature then stands for every named signer. `tests/
/// rogue_key.rs` builds exactly that key and shows it buys nothing here: each
/// signature is checked against exactly one public key, so a rogue registrant
/// cannot even cast its own vote, having no secret for the key it published.
///
/// Go does not aggregate either — `engine/chain/cert.go`: "there is no
/// aggregate field, because nothing is aggregated" — so an aggregate accept
/// rule would also be an accept this network cannot express.
#[derive(Clone, Debug, Default)]
pub struct ValidatorSet {
    keys: HashMap<Id, PublicKey>,
    weights: HashMap<Id, u64>,
}

impl ValidatorSet {
    pub fn new() -> Self {
        Self::default()
    }

    /// Register a validator with the key it signs with.
    ///
    /// The key is decompressed and group-checked once, here, so an invalid key
    /// is refused at registration rather than at every verification — and a
    /// rejected key leaves no membership behind.
    ///
    /// `key_validate` establishes that the bytes decode to a non-identity point
    /// of the right subgroup. It does NOT establish that the registrant holds
    /// the matching secret, and no registration here does: a key chosen as
    /// `g1·x − Σ pk_others` passes every check above. That is harmless while
    /// every signature is checked against exactly one key, and catastrophic the
    /// moment any of them are summed — see [`VoteVerifier`] on this type, and
    /// the note on `ValidatorSet` itself.
    pub fn insert(&mut self, node: Id, weight: u64, public_key: &[u8]) -> Result<(), BLST_ERROR> {
        if public_key.len() != PUBLIC_KEY_LEN {
            return Err(BLST_ERROR::BLST_BAD_ENCODING);
        }
        let pk = PublicKey::key_validate(public_key)?;
        self.keys.insert(node, pk);
        self.weights.insert(node, weight);
        Ok(())
    }

    /// Register a validator whose signing key this node does not have.
    ///
    /// It is a member and it holds stake; it cannot sign anything this set will
    /// accept until a key arrives.
    pub fn insert_unkeyed(&mut self, node: Id, weight: u64) {
        self.weights.insert(node, weight);
    }

    pub fn remove(&mut self, node: &Id) {
        self.keys.remove(node);
        self.weights.remove(node);
    }

    /// The number of members, keyed or not — the `n` every threshold is read
    /// against.
    pub fn len(&self) -> usize {
        self.weights.len()
    }

    pub fn is_empty(&self) -> bool {
        self.weights.is_empty()
    }

    /// Whether `node` is a member. Membership decides whose ballot is counted;
    /// it does not decide whose signature verifies.
    pub fn contains(&self, node: &Id) -> bool {
        self.weights.contains_key(node)
    }

    /// Whether `node` has a key here, and so can contribute to a certificate.
    pub fn can_verify(&self, node: &Id) -> bool {
        self.keys.contains_key(node)
    }

    pub fn public_key(&self, node: &Id) -> Option<&PublicKey> {
        self.keys.get(node)
    }
}

/// The set verifies its own members' votes. A voter outside the set is false —
/// there is no key to check against, and an unresolvable key is exactly as good
/// as a bad signature.
impl VoteVerifier for ValidatorSet {
    fn verify_vote(&self, node: &Id, message: &[u8], signature: &[u8], _epoch_height: u64) -> bool {
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
    fn weight(&self, node: &Id, _epoch_height: u64) -> u64 {
        self.weights.get(node).copied().unwrap_or(0)
    }

    fn total_stake(&self, _epoch_height: u64) -> u64 {
        self.weights.values().fold(0u64, |a, w| a.saturating_add(*w))
    }

    fn validator_count(&self, _epoch_height: u64) -> i64 {
        self.weights.len() as i64
    }
}
