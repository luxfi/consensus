// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! A vote on the wire, and the rule that turns votes into a certificate.
//!
//! Four things live here and nothing else:
//!
//! * [`SignedVote`] — one validator's signed statement about one position,
//!   and its ZAP frame payload.
//! * [`Slot`] — the one place a validator may make one statement, read out of
//!   the signed bytes so equivocation is visible before a key is used.
//! * [`VoteTransport`] — the seam a node disseminates over. Consensus states
//!   the shape and never opens a socket; the node supplies the mesh.
//! * [`Tally`] — verify, deduplicate, and issue a [`QuorumCert`] that has
//!   passed the whole of [`QuorumCert::verify_weighted`].
//!
//! THE PAYLOAD CARRIES THE SIGNED BYTES. A vote frame holds the exact
//! [`canonical_vote_message`] its signer signed, not the fields to rebuild it
//! from. A receiver that re-derives the preimage is a second implementation of
//! the derivation, and two derivations are how one node verifies a message the
//! other never made. The position is recovered *from* the message, so the
//! signed bytes and the read position cannot disagree. The C++ lane sends the
//! block id alone and re-derives, which is the shape this one refuses; [`VOTE`]
//! spells that divergence out and says which side owes the reconciliation.
//!
//! The transport identity — `block_id`, `parent_id` — is not signed and so is
//! not carried; a decoded position leaves those empty, and re-encoding is
//! byte-identical because [`Position::signed_identity`] resolves the degrade in
//! one place.
//!
//! THIS MODULE STATES NO ACCEPT RULE. A tally counts what it can verify and
//! nothing more; whether the votes it holds amount to finality is
//! [`crate::cert`]'s question, asked through [`QuorumCert::verify_weighted`] and
//! answered against the LIVE validator set — the signer floor, the stake floor
//! and the minimum Byzantine committee, recomputed every time. There is no door
//! out of here that skips it: [`Tally::cert`] is the only way to obtain a
//! certificate from a tally, and it runs the full predicate before it returns
//! one. A quorum a caller merely declares is not a quorum.

use std::collections::BTreeMap;

use crate::cert::{CertError, NodeId, QuorumCert, StakeSource, Vote, VoteVerifier, SIGNATURE_LEN};
use crate::finality::{
    canonical_vote_message, Finality, Id, Position, EMPTY, QC_FINALITY, QUORUM_CERT_VERSION,
    VOTE_MESSAGE_LEN, VOTE_TAG,
};
use crate::pop::NODE_LEN;
use crate::zap;

/// The ZAP message type a consensus vote travels under.
///
/// It sits in the block Go reserves for Quasar — the finality tier a vote
/// carries — and is the next free id there. `luxfi/api/zap` (`wire.go`) spells
/// 60 `MsgSetQuasarFinalized` and 61 `MsgQuasarHeight` of the 60..63 range and
/// stops, so 62 is unclaimed in the registry the plugin boundary reads.
///
/// It is deliberately NOT `0x11`. The C++ lane's `vote_codec.hpp` uses `0x11`,
/// which is 17, which the Go registry already holds as `MsgResponse`. One id
/// space, one meaning: a link that ever carried both would dispatch a vote to
/// the response path.
///
/// THE TWO LANES DO NOT YET CARRY EACH OTHER'S VOTES, and the id is the smaller
/// half of why. C++ frames `block_id(32) ‖ pubkey(48) ‖ sig(96)` — 188 payload
/// bytes naming its signer by its 48-byte key — where this lane frames
/// `signed_message(226) ‖ node(20) ‖ sig(96)`, 354 bytes naming its signer by
/// the 20-byte NodeID a proof of possession binds. C++ signs the same
/// [`canonical_vote_message`] and then sends only the block id, so its receiver
/// rebuilds the preimage it checks — the second derivation this module refuses
/// to have.
///
/// Two of the three have a standard to be held to, and this side is the one
/// holding it: 0x11 is `MsgResponse` in the Go registry, and Go names a
/// validator by the 20-byte `ids.NodeID` its proof of possession signs over.
/// The third is this module's own rule. So the reconciliation is owed on the C++
/// side — and until it is paid the two exchange frames neither can read, since
/// each refuses the other's type id and, past that, the other's field widths.
/// Nothing here should be read as saying they interoperate today.
///
/// 62 IS NOT UNCLAIMED EVERYWHERE. `github.com/luxfi/mpc`
/// (`pkg/transport/wire.go`) extends the same frame with its own 60..69 —
/// `MsgMPCReady` is also 62 — so the id is free in the registry and taken on the
/// MPC ring. Nothing here is wrong on the validator mesh, which carries votes
/// and no MPC traffic and is where these frames travel; and the MPC ring cannot
/// be reached by accident either, since it listens on its own port behind
/// mandatory PQ TLS 1.3 and closes any connection whose FIRST frame is not a
/// `MsgMPCReady` carrying JSON — which a vote payload is not. But "62 is free"
/// is true of `api/zap` and not of every link, and a reader deciding where else
/// to send a vote should know which. That extension also spells 64..69, above
/// the `TYPE_MASK` ceiling, so its 64 is a bare [`zap::ERROR_FLAG`] — a
/// registry-wide reconciliation is owed, and this end is not the place to spend
/// it.
pub const VOTE: u8 = 62;

/// The exact size of a vote frame payload: three length-prefixed fields.
pub const VOTE_PAYLOAD_LEN: usize = 4 + VOTE_MESSAGE_LEN + 4 + NODE_LEN + 4 + SIGNATURE_LEN;

/// Why a vote was not counted. Every variant means the vote does not happen;
/// none of them means "probably fine".
///
/// The clauses a CERTIFICATE owns are not respelled here — they are carried in
/// [`VoteError::Cert`] as the very values [`crate::cert`] names, so the version,
/// the role, the tier and every floor have one vocabulary across both modules
/// and a caller matching on them matches on one set of facts.
///
/// A vote plane that grows a refusal must be able to say so without breaking
/// every caller that matches on this, so the enum is closed to exhaustive
/// matching from outside: a downstream `match` carries a wildcard, and a new
/// clause reaches it rather than a compile error. Nothing here is added lightly
/// — but it is added before publication, because it can never be added after.
#[non_exhaustive]
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum VoteError {
    /// The bytes are not a vote: a field of the wrong width, a length that
    /// outruns the buffer, trailing bytes past the last field, a foreign tag,
    /// or an accept byte that is neither 0 nor 1.
    Wire,
    /// A vote over some other position, offered to this tally. Decided on the
    /// SIGNED BYTES, so any position that produces this tally's message is this
    /// tally's statement and no other is.
    Position,
    /// A reject. A finality certificate carries accepts, so a reject is not a
    /// vote a tally can hold — it is a statement about the same slot that a
    /// caller must handle somewhere else.
    NotAccept,
    /// The signer does not resolve in the set, or the signature does not verify
    /// under the key that does. An unregistered signer is refused here and not
    /// skipped: a key the set never admitted through
    /// [`crate::cert::ValidatorSet::insert`] — and so never proved possession of
    /// — can put nothing behind a vote.
    Signature,
    /// A certificate clause refused it, including the two quorum floors and the
    /// minimum committee.
    Cert(CertError),
}

impl From<CertError> for VoteError {
    fn from(e: CertError) -> Self {
        VoteError::Cert(e)
    }
}

impl std::fmt::Display for VoteError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            VoteError::Wire => f.write_str("not a vote frame"),
            VoteError::Position => f.write_str("vote is over another position"),
            VoteError::NotAccept => f.write_str("a reject is not a finality vote"),
            VoteError::Signature => f.write_str("signer does not resolve, or signature is bad"),
            VoteError::Cert(e) => write!(f, "{e}"),
        }
    }
}

impl std::error::Error for VoteError {}

/// One validator's signed statement about one position.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SignedVote {
    pub position: Position,
    pub accept: bool,
    pub node: NodeId,
    pub signature: Vec<u8>,
}

impl SignedVote {
    /// The bytes this vote's signature is over.
    pub fn message(&self) -> Vec<u8> {
        canonical_vote_message(&self.position, self.accept)
    }

    /// Does the signature verify, under the key the set resolves for this node
    /// at this epoch? An unresolvable signer is a false, not a skip.
    pub fn verify(&self, verifier: &dyn VoteVerifier, epoch_height: u64) -> bool {
        verifier.verify_vote(&self.node, &self.message(), &self.signature, epoch_height)
    }

    /// The certificate record this vote becomes.
    pub fn record(&self) -> Vote {
        Vote {
            node_id: self.node,
            accept: self.accept,
            signature: self.signature.clone(),
        }
    }

    /// The ZAP frame payload: the signed message, the signer, the signature.
    pub fn encode(&self) -> Vec<u8> {
        let mut w = zap::Writer::with_capacity(VOTE_PAYLOAD_LEN);
        w.bytes(&self.message())
            .bytes(&self.node)
            .bytes(&self.signature);
        w.take()
    }

    /// Read a vote out of a frame payload.
    ///
    /// Every width is exact and trailing bytes are refused, so `vote ‖ garbage`
    /// is not a vote and a peer cannot smuggle bytes past the decoder.
    pub fn decode(payload: &[u8]) -> Result<Self, VoteError> {
        let mut r = zap::Reader::new(payload);
        let message = r.bytes().ok_or(VoteError::Wire)?;
        let node = r.bytes().ok_or(VoteError::Wire)?;
        let signature = r.bytes().ok_or(VoteError::Wire)?;
        if r.remaining() != 0 {
            return Err(VoteError::Wire);
        }
        if node.len() != NODE_LEN || signature.len() != SIGNATURE_LEN {
            return Err(VoteError::Wire);
        }
        let (position, accept) = read_message(message)?;
        Ok(SignedVote {
            position,
            accept,
            node: node.try_into().map_err(|_| VoteError::Wire)?,
            signature: signature.to_vec(),
        })
    }
}

/// Recover the position and the accept bit from the bytes that were signed.
///
/// The inverse of [`canonical_vote_message`], and checked against it by
/// round-trip: the tag, version and role are the network's, or these are not
/// this network's signed bytes.
pub fn read_message(message: &[u8]) -> Result<(Position, bool), VoteError> {
    if message.len() != VOTE_MESSAGE_LEN {
        return Err(VoteError::Wire);
    }
    // Offsets are the layout `canonical_vote_message` writes, in order.
    let tag = VOTE_TAG.len(); // 18
    if &message[..tag] != VOTE_TAG {
        return Err(VoteError::Wire);
    }
    let version = u16::from_be_bytes([message[tag], message[tag + 1]]);
    if version != QUORUM_CERT_VERSION {
        return Err(CertError::Version {
            got: version,
            want: QUORUM_CERT_VERSION,
        }
        .into());
    }
    let role = message[tag + 2];
    if role != QC_FINALITY {
        return Err(CertError::Type {
            got: role,
            want: QC_FINALITY,
        }
        .into());
    }
    let id = |at: usize| -> Id { message[at..at + 32].try_into().expect("32 bytes") };
    let chain_id = id(21);
    let height = u64::from_be_bytes(message[53..61].try_into().expect("8 bytes"));
    let round = u32::from_be_bytes(message[61..65].try_into().expect("4 bytes"));
    let canonical_id = id(65);
    let parent_canonical_id = id(97);
    let execution_state_root = id(129);
    let payload_root = id(161);
    let validator_set_root = id(193);
    let accept = match message[225] {
        0x00 => false,
        0x01 => true,
        // The accept byte is bound into the signature, so a third value is a
        // message no signer produced. Refuse it rather than coerce it to true.
        _ => return Err(VoteError::Wire),
    };
    Ok((
        Position {
            chain_id,
            height,
            round,
            block_id: EMPTY,
            parent_id: EMPTY,
            canonical_id,
            parent_canonical_id,
            execution_state_root,
            payload_root,
            validator_set_root,
        },
        accept,
    ))
}

/// The one place a validator may make one statement.
///
/// A chain, a height, and a round. Two messages that share a slot and differ in
/// any other byte are the same validator saying two things about one point in
/// the chain — an equivocation, which is what a slashing rule is for and what a
/// fork is made of. Naming the slot is what lets a signer refuse to be the
/// second one.
///
/// It is derived from the SIGNED bytes, never from the envelope: the transport
/// identity is not signed, so two wrappings of one inner block share a slot, as
/// they must.
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub struct Slot {
    pub chain: Id,
    pub height: u64,
    pub round: u32,
}

impl Slot {
    /// The slot a position falls in.
    pub fn of(position: &Position) -> Slot {
        Slot {
            chain: position.chain_id,
            height: position.height,
            round: position.round,
        }
    }

    /// The slot a signed message falls in, read out of the message itself.
    ///
    /// Refuses anything that is not this network's signed bytes, so a slot can
    /// never be taken from a message the tag, version or role disowns.
    pub fn read(message: &[u8]) -> Result<Slot, VoteError> {
        read_message(message).map(|(position, _)| Slot::of(&position))
    }
}

/// How a node disseminates its own vote.
///
/// Consensus states the seam and never opens a socket: a node carries this over
/// a validator mesh, and a test carries it over a channel. An implementation may
/// echo the vote back to its origin — [`Tally`] deduplicates by signer, so a
/// self-echo costs nothing and is how an originator's own vote reaches its own
/// tally.
pub trait VoteTransport {
    fn broadcast(&self, vote: &SignedVote);
}

/// Votes for one position at one epoch, and the certificate they add up to.
///
/// A vote is HELD when, and only when, it is an accept, it is over THIS tally's
/// position, and its signature verifies under the key the set resolves for its
/// signer. Everything else is refused with a reason.
///
/// Holding is not accepting. Whether the held votes are a quorum is decided by
/// [`QuorumCert::verify_weighted`] against the live set, inside [`Tally::cert`],
/// which is the only way out — see the module note. The `threshold` a tally
/// carries is the certificate's own declared field and a caller's stopping
/// condition; it is a floor UNDER the real floors and can never stand in for
/// them.
///
/// ONE TALLY, ONE EPOCH. The epoch height is fixed when the tally opens and both
/// the signature check and the stake read use it. A tally that verified under
/// one epoch's keys and weighed under another's would be checking a signature
/// against one set and counting it in another, which is the disagreement
/// [`StakeSource`] exists to prevent.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Tally {
    position: Position,
    tier: Finality,
    threshold: u32,
    epoch_height: u64,
    message: Vec<u8>,
    votes: BTreeMap<NodeId, Vec<u8>>,
}

impl Tally {
    /// A tally for one position at one rung, at one epoch.
    ///
    /// A threshold of zero is a quorum of nobody, and a rung that is not Nova or
    /// Quasar is not a rung a certificate can claim; both are refused here so no
    /// later step has to, and both are refused with the clause
    /// [`QuorumCert::verify`] would have refused them with.
    pub fn new(
        position: Position,
        tier: Finality,
        threshold: u32,
        epoch_height: u64,
    ) -> Result<Self, VoteError> {
        if tier != Finality::Nova && tier != Finality::Quasar {
            return Err(CertError::UnknownTier(tier).into());
        }
        if threshold == 0 {
            return Err(CertError::ThresholdZero.into());
        }
        let message = canonical_vote_message(&position, true);
        Ok(Tally {
            position,
            tier,
            threshold,
            epoch_height,
            message,
            votes: BTreeMap::new(),
        })
    }

    /// The bytes a validator must sign to be counted here.
    pub fn message(&self) -> &[u8] {
        &self.message
    }

    pub fn position(&self) -> &Position {
        &self.position
    }

    pub fn tier(&self) -> Finality {
        self.tier
    }

    pub fn threshold(&self) -> u32 {
        self.threshold
    }

    /// The epoch every signature here was checked under, and the epoch its
    /// certificate is weighed at.
    pub fn epoch_height(&self) -> u64 {
        self.epoch_height
    }

    /// Distinct signers held so far.
    pub fn len(&self) -> usize {
        self.votes.len()
    }

    pub fn is_empty(&self) -> bool {
        self.votes.is_empty()
    }

    /// Hold one vote. `Ok(true)` when it was newly recorded, `Ok(false)` when
    /// that signer was already held — the first signature from a signer stands,
    /// so an equivocating peer cannot displace what it already said.
    pub fn add(
        &mut self,
        vote: &SignedVote,
        verifier: &dyn VoteVerifier,
    ) -> Result<bool, VoteError> {
        if !vote.accept {
            return Err(VoteError::NotAccept);
        }
        // Comparing the SIGNED BYTES, not the struct: what the signer committed
        // to is exactly this message, and any position that produces it is the
        // same statement.
        if vote.message() != self.message {
            return Err(VoteError::Position);
        }
        if !verifier.verify_vote(
            &vote.node,
            &self.message,
            &vote.signature,
            self.epoch_height,
        ) {
            return Err(VoteError::Signature);
        }
        if self.votes.contains_key(&vote.node) {
            return Ok(false);
        }
        self.votes.insert(vote.node, vote.signature.clone());
        Ok(true)
    }

    /// The certificate these votes make — or the clause that refuses it.
    ///
    /// The whole predicate runs here: [`QuorumCert::verify_weighted`] over the
    /// live set at this tally's epoch, which is [`QuorumCert::verify`] plus the
    /// rung's floors — for Quasar the stake supermajority, the distinct-signer
    /// count and [`crate::finality::MIN_BFT_COMMITTEE`]; for Nova the stake
    /// majority and `nova_signer_floor`. Every one of them is recomputed from
    /// the set, so a tally can no more declare its own quorum than a certificate
    /// can.
    ///
    /// Signatures are checked twice — once as each vote is held, once as the
    /// certificate is verified — and that is the point. The certificate a caller
    /// receives has passed the same check any other party will run on it, so a
    /// tally cannot issue one that only it believes.
    pub fn cert(
        &self,
        verifier: &dyn VoteVerifier,
        stake: &dyn StakeSource,
    ) -> Result<QuorumCert, VoteError> {
        let votes: Vec<Vote> = self
            .votes
            .iter()
            .map(|(node, signature)| Vote {
                node_id: *node,
                accept: true,
                signature: signature.clone(),
            })
            .collect();
        let cert = QuorumCert::assemble(self.tier, self.position.clone(), self.threshold, &votes)?;
        cert.verify_weighted(verifier, stake, self.epoch_height)?;
        Ok(cert)
    }
}
