// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The vote plane: the frame, the slot, and the tally.
//!
//! Two questions are asked here and they are not the same question.
//!
//! The first is whether a vote survives the wire unchanged — whether the bytes
//! a validator signed are the bytes a receiver checks, and whether every
//! near-miss frame is refused rather than repaired. `SignedVote` carries the
//! signed message itself, so this is answered by round-trip and by mutation.
//!
//! The second is whether a tally can hand out a certificate the network would
//! not accept. It must not be able to. A tally holds votes; it states no accept
//! rule; the rule is `cert`'s, and [`Tally::cert`] is the only door out of a
//! tally and runs the whole of `verify_weighted` before it returns. The tests
//! from `a_tally_cannot_declare_its_own_quorum` down are that property, one
//! floor at a time: the stake supermajority, the distinct-signer count, the
//! minimum Byzantine committee, and — through registration — the proof of
//! possession that decides who may be counted at all.

use blst::min_pk::SecretKey;
use lux_consensus::cert::{
    CertError, NodeId, QuorumCert, StakeSource, ValidatorSet, VoteVerifier, DST,
};
use lux_consensus::finality::{
    canonical_vote_message, half_stake_floor, nova_signer_floor, two_thirds_count,
    two_thirds_stake_floor, Finality, Position, EMPTY, VOTE_MESSAGE_LEN,
};
use lux_consensus::pop::{self, NODE_LEN};
use lux_consensus::vote::{
    read_message, SignedVote, Slot, Tally, VoteError, VoteTransport, VOTE, VOTE_PAYLOAD_LEN,
};
use lux_consensus::zap;

/// The epoch these tests register and vote at. One value, so a test that meant
/// to change it has to say so.
const EPOCH: u64 = 9;

/// A validator: a key, an id, and a weight.
struct Signer {
    id: NodeId,
    sk: SecretKey,
    weight: u64,
}

impl Signer {
    /// Deterministic per `n`, so a failure reproduces exactly.
    fn new(n: u8, weight: u64) -> Self {
        let sk = SecretKey::key_gen(&[n; 32], &[]).expect("key_gen");
        let mut id = [0u8; NODE_LEN];
        id[0] = n;
        Signer { id, sk, weight }
    }

    fn public(&self) -> [u8; 48] {
        self.sk.sk_to_pk().compress()
    }

    /// The proof of possession this validator presents at registration.
    fn pop(&self) -> Vec<u8> {
        pop::sign(&self.sk, &self.id, &self.public())
    }

    /// A signed statement about `pos`. The signature is over the canonical
    /// message, which is the only thing a verifier ever checks.
    fn vote(&self, pos: &Position, accept: bool) -> SignedVote {
        let message = canonical_vote_message(pos, accept);
        SignedVote {
            position: pos.clone(),
            accept,
            node: self.id,
            signature: self.sk.sign(&message, DST, &[]).compress().to_vec(),
        }
    }
}

/// A committee of `n` equal-weight validators, admitted through the proof path.
fn committee(n: u8, weight: u64) -> (Vec<Signer>, ValidatorSet) {
    let signers: Vec<Signer> = (1..=n).map(|i| Signer::new(i, weight)).collect();
    let mut set = ValidatorSet::new();
    for s in &signers {
        set.insert(s.id, s.weight, &s.public(), &s.pop())
            .expect("insert");
    }
    (signers, set)
}

/// A position with every slot populated, so the canonical-degrade path is not
/// what is under test.
fn position() -> Position {
    Position {
        chain_id: [0xc1; 32],
        height: 42,
        round: 7,
        block_id: [0xb1; 32],
        parent_id: [0xb2; 32],
        canonical_id: [0xca; 32],
        parent_canonical_id: [0xcb; 32],
        execution_state_root: [0xe5; 32],
        payload_root: [0x70; 32],
        validator_set_root: [0x55; 32],
    }
}

/// A tally at [`EPOCH`], for the rung under test.
fn tally(tier: Finality, threshold: u32) -> Tally {
    Tally::new(position(), tier, threshold, EPOCH).expect("a tally opens")
}

// ── The frame ───────────────────────────────────────────────────────

#[test]
fn a_vote_frame_round_trips() {
    let (s, _) = committee(1, 1);
    let v = s[0].vote(&position(), true);
    let payload = v.encode();
    assert_eq!(payload.len(), VOTE_PAYLOAD_LEN);

    let back = SignedVote::decode(&payload).expect("decode");
    // The transport ids are not signed, so they do not survive — and the signed
    // bytes are identical either way, which is the property that matters.
    assert_eq!(back.message(), v.message());
    assert_eq!(back.node, v.node);
    assert_eq!(back.signature, v.signature);
    assert!(back.accept);
    assert_eq!(back.encode(), payload);
    assert_eq!(
        back.position.block_id, EMPTY,
        "the envelope is not in the signature"
    );
}

#[test]
fn a_position_with_no_inner_outer_split_round_trips_too() {
    // The degrade case: canonical slots unset, so the message binds the
    // transport ids. Re-encoding must produce the same bytes.
    let mut pos = position();
    pos.canonical_id = EMPTY;
    pos.parent_canonical_id = EMPTY;
    let (s, _) = committee(1, 1);
    let v = s[0].vote(&pos, true);
    let back = SignedVote::decode(&v.encode()).expect("decode");
    assert_eq!(back.message(), v.message());
    assert_eq!(back.encode(), v.encode());
}

#[test]
fn trailing_bytes_are_not_a_vote() {
    let (s, _) = committee(1, 1);
    let mut payload = s[0].vote(&position(), true).encode();
    payload.push(0);
    assert_eq!(SignedVote::decode(&payload), Err(VoteError::Wire));
}

#[test]
fn a_wrong_width_field_is_not_a_vote() {
    let (s, _) = committee(1, 1);
    let v = s[0].vote(&position(), true);

    // A 19-byte node id.
    let mut w = zap::Writer::new();
    w.bytes(&v.message())
        .bytes(&v.node[..NODE_LEN - 1])
        .bytes(&v.signature);
    assert_eq!(SignedVote::decode(&w.take()), Err(VoteError::Wire));

    // A 95-byte signature.
    let mut w = zap::Writer::new();
    w.bytes(&v.message())
        .bytes(&v.node)
        .bytes(&v.signature[..95]);
    assert_eq!(SignedVote::decode(&w.take()), Err(VoteError::Wire));
}

#[test]
fn a_truncated_payload_is_not_a_vote() {
    let (s, _) = committee(1, 1);
    let payload = s[0].vote(&position(), true).encode();
    for cut in [0, 1, 4, 100, VOTE_PAYLOAD_LEN - 1] {
        assert_eq!(
            SignedVote::decode(&payload[..cut]),
            Err(VoteError::Wire),
            "cut {cut}"
        );
    }
}

#[test]
fn a_foreign_tag_is_not_this_networks_vote() {
    let (s, _) = committee(1, 1);
    let v = s[0].vote(&position(), true);
    let mut message = v.message();
    message[0] = b'X';
    let mut w = zap::Writer::new();
    w.bytes(&message).bytes(&v.node).bytes(&v.signature);
    assert_eq!(SignedVote::decode(&w.take()), Err(VoteError::Wire));
}

#[test]
fn a_third_accept_value_is_refused() {
    let (s, _) = committee(1, 1);
    let v = s[0].vote(&position(), true);
    let mut message = v.message();
    *message.last_mut().expect("accept byte") = 0x02;
    let mut w = zap::Writer::new();
    w.bytes(&message).bytes(&v.node).bytes(&v.signature);
    assert_eq!(SignedVote::decode(&w.take()), Err(VoteError::Wire));
}

/// A refusal names the clause. The version and the role live in the signed
/// bytes, so a message from another certificate version is refused as THAT, and
/// with the same values `cert` would have refused it with — one vocabulary
/// across both modules.
#[test]
fn a_foreign_version_and_a_foreign_role_are_named() {
    let base = canonical_vote_message(&position(), true);

    let mut wrong_version = base.clone();
    wrong_version[18] = 0;
    wrong_version[19] = 99;
    assert_eq!(
        read_message(&wrong_version),
        Err(VoteError::Cert(CertError::Version { got: 99, want: 3 })),
    );

    let mut wrong_role = base;
    wrong_role[20] = 7;
    assert_eq!(
        read_message(&wrong_role),
        Err(VoteError::Cert(CertError::Type { got: 7, want: 1 })),
    );
}

#[test]
fn the_message_is_the_length_the_standard_fixes() {
    assert_eq!(
        canonical_vote_message(&position(), true).len(),
        VOTE_MESSAGE_LEN
    );
    assert_eq!(read_message(&[]), Err(VoteError::Wire));
    assert_eq!(
        read_message(&[0u8; VOTE_MESSAGE_LEN - 1]),
        Err(VoteError::Wire)
    );
}

#[test]
fn the_vote_type_id_fits_the_id_space() {
    // A type id with bit 6 or 7 set arrives at the far end looking like a flag,
    // so stripping the flags must leave the id untouched.
    assert_eq!(
        zap::strip_flags(VOTE),
        VOTE,
        "a type id must not collide with a flag"
    );
}

// ── The slot ────────────────────────────────────────────────────────

#[test]
fn a_slot_is_the_chain_the_height_and_the_round() {
    let pos = Position {
        chain_id: [7u8; 32],
        height: 42,
        round: 3,
        ..Default::default()
    };
    let slot = Slot::of(&pos);
    assert_eq!(
        slot,
        Slot {
            chain: [7u8; 32],
            height: 42,
            round: 3
        }
    );
    // And it is readable out of the signed bytes, which is where a receiver gets
    // it from.
    assert_eq!(
        Slot::read(&canonical_vote_message(&pos, true)).unwrap(),
        slot
    );
}

#[test]
fn accepting_and_rejecting_share_one_slot() {
    // Two statements a validator must not both make: same point in the chain,
    // opposite answers.
    let pos = Position {
        chain_id: [9u8; 32],
        height: 1,
        round: 0,
        ..Default::default()
    };
    let yes = canonical_vote_message(&pos, true);
    let no = canonical_vote_message(&pos, false);
    assert_ne!(yes, no, "the accept bit is signed");
    assert_eq!(Slot::read(&yes).unwrap(), Slot::read(&no).unwrap());
}

#[test]
fn two_wrappings_of_one_block_share_one_slot() {
    // The transport identity is not signed, so a block wrapped twice is one
    // statement, not two.
    let inner = Position {
        chain_id: [4u8; 32],
        height: 8,
        round: 1,
        canonical_id: [5u8; 32],
        block_id: [0xAA; 32],
        ..Default::default()
    };
    let other = Position {
        block_id: [0xBB; 32],
        ..inner.clone()
    };
    assert_eq!(Slot::of(&inner), Slot::of(&other));
    assert_eq!(
        canonical_vote_message(&inner, true),
        canonical_vote_message(&other, true)
    );
}

#[test]
fn a_message_this_network_disowns_has_no_slot() {
    assert_eq!(Slot::read(&[]), Err(VoteError::Wire));
    let mut message = canonical_vote_message(&Position::default(), true);
    message[0] ^= 0xFF; // the tag
    assert_eq!(Slot::read(&message), Err(VoteError::Wire));
}

// ── What a tally will hold ──────────────────────────────────────────

#[test]
fn a_quorum_of_nobody_is_not_a_tally() {
    assert_eq!(
        Tally::new(position(), Finality::Quasar, 0, EPOCH),
        Err(VoteError::Cert(CertError::ThresholdZero)),
    );
    assert_eq!(
        Tally::new(position(), Finality::Wave, 1, EPOCH),
        Err(VoteError::Cert(CertError::UnknownTier(Finality::Wave))),
    );
}

#[test]
fn one_signer_counts_once() {
    let (s, set) = committee(2, 1);
    let mut t = tally(Finality::Quasar, 2);
    let v = s[0].vote(&position(), true);
    assert!(t.add(&v, &set).expect("first"));
    assert!(
        !t.add(&v, &set).expect("second"),
        "a repeat is not a second signer"
    );
    assert_eq!(t.len(), 1);
}

#[test]
fn a_reject_is_not_a_finality_vote() {
    let (s, set) = committee(1, 1);
    let mut t = tally(Finality::Quasar, 1);
    assert_eq!(
        t.add(&s[0].vote(&position(), false), &set),
        Err(VoteError::NotAccept)
    );
}

#[test]
fn a_vote_for_another_position_does_not_count_here() {
    let (s, set) = committee(1, 1);
    let mut other = position();
    other.height += 1;
    let mut t = tally(Finality::Quasar, 1);
    assert_eq!(
        t.add(&s[0].vote(&other, true), &set),
        Err(VoteError::Position)
    );
}

#[test]
fn a_forged_signature_is_refused() {
    let (s, set) = committee(2, 1);
    // Node 0's identity carrying node 1's signature.
    let mut v = s[1].vote(&position(), true);
    v.node = s[0].id;
    let mut t = tally(Finality::Quasar, 1);
    assert_eq!(t.add(&v, &set), Err(VoteError::Signature));
}

/// A real key over the real message, from a node the set never admitted. There
/// is nothing wrong with the signature; there is no one it stands for.
#[test]
fn a_signer_the_set_never_admitted_is_refused() {
    let (_, set) = committee(1, 1);
    let stranger = Signer::new(0xff, 1);
    let mut t = tally(Finality::Quasar, 1);
    assert_eq!(
        t.add(&stranger.vote(&position(), true), &set),
        Err(VoteError::Signature)
    );
    assert!(t.is_empty());
}

/// A member the chain carries without a key. It counts toward membership and
/// can never put a vote behind its stake — the fail-closed direction, held
/// through the vote plane and not only at the certificate.
#[test]
fn a_keyless_member_cannot_be_counted() {
    let (s, mut set) = committee(4, 1);
    let spectator = Signer::new(0x40, 1_000_000);
    set.insert_unkeyed(spectator.id, spectator.weight)
        .expect("a keyless member is a member");

    let mut t = tally(Finality::Quasar, 1);
    assert_eq!(
        t.add(&spectator.vote(&position(), true), &set),
        Err(VoteError::Signature)
    );
    assert!(t
        .add(&s[0].vote(&position(), true), &set)
        .expect("a keyed member is held"));
    assert_eq!(t.len(), 1);
}

// ── What a tally will NOT certify ───────────────────────────────────

/// THE PROPERTY THIS MODULE EXISTS FOR.
///
/// A tally is handed a threshold of one and one perfectly valid signature. The
/// declared threshold is met; `assemble` is satisfied; the signature verifies.
/// It still gets no certificate, because the floors are recomputed from the LIVE
/// set and one signer of seven is not two thirds of anything.
#[test]
fn a_tally_cannot_declare_its_own_quorum() {
    let (s, set) = committee(7, 1);
    let mut t = tally(Finality::Quasar, 1);
    assert!(t.add(&s[0].vote(&position(), true), &set).expect("held"));

    assert_eq!(
        t.cert(&set, &set),
        Err(VoteError::Cert(CertError::StakeBelowSupermajority {
            voted: 1,
            signer: 7,
            need_above: two_thirds_stake_floor(7),
        })),
        "a threshold a caller declares is not a quorum the set agrees to",
    );
}

/// The rung certifies at the set's floor, and the vote before it is not enough.
/// On an equal-weight set the two Quasar floors are one bar in two units, so the
/// last vote crosses both at once.
///
/// The declared threshold and the set's floor are two different bars, and BOTH
/// are checked. A tally that declares the real floor is refused by its own
/// declaration one vote short; a tally that declares less than the floor is
/// refused by the SET at the same point. The second is the one that matters —
/// the declaration is the caller's and the floor is not.
#[test]
fn a_quasar_tally_certifies_at_the_sets_floor_and_not_before() {
    let (s, set) = committee(7, 1);
    let need = two_thirds_count(7);
    assert_eq!(need, 5);
    let short = need as usize - 1;

    // Declaring the floor: four votes fail the declaration, which is the earlier
    // bar and so the one that names the refusal.
    let mut declared = tally(Finality::Quasar, need as u32);
    for signer in s.iter().take(short) {
        assert!(declared
            .add(&signer.vote(&position(), true), &set)
            .expect("held"));
    }
    assert_eq!(
        declared.cert(&set, &set),
        Err(VoteError::Cert(CertError::BelowThreshold {
            have: 4,
            need: 5
        })),
        "four of seven does not meet a threshold of five",
    );

    // Declaring one: the same four votes clear the declaration and are refused
    // by the set's own floor, recomputed from it.
    let mut undeclared = tally(Finality::Quasar, 1);
    for signer in s.iter().take(short) {
        assert!(undeclared
            .add(&signer.vote(&position(), true), &set)
            .expect("held"));
    }
    assert_eq!(
        undeclared.cert(&set, &set),
        Err(VoteError::Cert(CertError::StakeBelowSupermajority {
            voted: 4,
            signer: 7,
            need_above: two_thirds_stake_floor(7),
        })),
        "four of seven is not an export quorum however little a tally asked for",
    );

    // The fifth vote crosses both bars at once.
    assert!(declared
        .add(&s[short].vote(&position(), true), &set)
        .expect("held"));
    let cert = declared.cert(&set, &set).expect("five of seven exports");
    assert_eq!(cert.votes.len(), 5);
    assert_eq!(cert.tier, Finality::Quasar);
}

/// The distinct-signer floor binds ALONE where the weights are lopsided: one
/// validator holding most of the stake clears the stake clause by itself and is
/// still not a supermajority of signers. This is the clause the stake predicate
/// cannot give, reached through the tally.
#[test]
fn stake_alone_does_not_export() {
    let whale = Signer::new(1, 1_000);
    let rest: Vec<Signer> = (2..=7).map(|i| Signer::new(i, 1)).collect();
    let mut set = ValidatorSet::new();
    for s in std::iter::once(&whale).chain(rest.iter()) {
        set.insert(s.id, s.weight, &s.public(), &s.pop())
            .expect("insert");
    }

    let mut t = tally(Finality::Quasar, 1);
    assert!(t.add(&whale.vote(&position(), true), &set).expect("held"));

    // The whale's 1000 of 1006 is past floor(2·1006/3) = 670, so stake passes
    // and the count clause is what refuses.
    assert!(1_000 > two_thirds_stake_floor(1_006));
    assert_eq!(
        t.cert(&set, &set),
        Err(VoteError::Cert(CertError::SignerFloor {
            have: 1,
            need: two_thirds_count(7),
            n: 7
        })),
        "the stake was there and the signers were not",
    );
}

/// Below four signers the fault budget f = ⌊(n−1)/3⌋ is zero: every signer is
/// load-bearing and a single compromised key forges the export. A unanimous
/// three-signer set clears both other floors and still cannot export.
#[test]
fn a_tally_cannot_export_below_the_minimum_committee() {
    let (s, set) = committee(3, 1);
    let mut t = tally(Finality::Quasar, 3);
    for signer in &s {
        assert!(t.add(&signer.vote(&position(), true), &set).expect("held"));
    }
    assert_eq!(
        t.cert(&set, &set),
        Err(VoteError::Cert(CertError::MinCommittee { n: 3, need: 4 })),
        "unanimity over three is not a Byzantine supermajority",
    );
}

/// Nova is a different rung with different floors, and a tally runs whichever it
/// opened at — it does not carry one rule and label it two.
#[test]
fn a_nova_tally_runs_the_nova_floors() {
    let (s, set) = committee(7, 1);
    assert_eq!(nova_signer_floor(7), 3);
    assert_eq!(half_stake_floor(7), 3);

    let mut t = tally(Finality::Nova, 1);
    for signer in s.iter().take(3) {
        assert!(t.add(&signer.vote(&position(), true), &set).expect("held"));
    }
    // Three clears the signer floor and ties the stake majority, which must be
    // STRICTLY exceeded.
    assert_eq!(
        t.cert(&set, &set),
        Err(VoteError::Cert(CertError::StakeBelowMajority {
            voted: 3,
            signer: 7,
            need_above: 3
        })),
    );

    assert!(t.add(&s[3].vote(&position(), true), &set).expect("held"));
    let cert = t.cert(&set, &set).expect("four of seven ignites");
    assert_eq!(cert.tier, Finality::Nova);

    // And Nova's four is not Quasar's five: the same votes at the export rung
    // are refused.
    let mut q = tally(Finality::Quasar, 1);
    for signer in s.iter().take(4) {
        assert!(q.add(&signer.vote(&position(), true), &set).expect("held"));
    }
    assert!(
        q.cert(&set, &set).is_err(),
        "the export rung is not the ignition rung"
    );
}

/// A rogue key — `g1·x − Σ pk_others`, the key whose holder knows no secret —
/// cannot be registered, so it never becomes a signer and never reaches a tally.
/// The proof of possession is what closes it, and it is closed at the door.
#[test]
fn a_rogue_key_never_reaches_a_tally() {
    let (s, mut set) = committee(4, 1);
    let rogue_id = {
        let mut id = [0u8; NODE_LEN];
        id[0] = 0xAA;
        id
    };

    // No secret is known for this key, so no proof over (node, key) can be made;
    // any bytes offered as one are refused.
    let honest = s[0].public();
    assert_eq!(
        set.insert(rogue_id, 1, &honest, &[0u8; 96]),
        Err(CertError::PopInvalid),
        "a proof that is not a signature is not a proof",
    );
    // Nor can it be smuggled in behind an honest validator's own proof: a proof
    // binds the one (node, key) pair it was made for.
    assert_eq!(
        set.insert(rogue_id, 1, &honest, &s[0].pop()),
        Err(CertError::PopInvalid),
        "a proof made for another node does not travel",
    );

    // So the identity resolves to nothing, and a tally holds nothing for it.
    let mut t = tally(Finality::Quasar, 1);
    let forged = SignedVote {
        position: position(),
        accept: true,
        node: rogue_id,
        signature: vec![0u8; 96],
    };
    assert_eq!(t.add(&forged, &set), Err(VoteError::Signature));
}

// ── What a certificate a tally issues is worth ──────────────────────

/// The certificate a tally hands out is not a private artefact: it passes the
/// same predicate any third party runs, and survives its own encoding.
#[test]
fn the_certificate_a_tally_issues_verifies_for_a_third_party() {
    let (s, set) = committee(7, 1);
    let mut t = tally(Finality::Quasar, two_thirds_count(7) as u32);
    for signer in s.iter().take(5) {
        assert!(t.add(&signer.vote(&position(), true), &set).expect("held"));
    }
    let cert = t.cert(&set, &set).expect("a quorum certifies");

    // A party that never saw the tally, holding only the set.
    cert.verify_weighted(&set, &set, EPOCH)
        .expect("it verifies for anyone");

    // Votes are in canonical order, which is the distinctness clause too.
    let ids: Vec<NodeId> = cert.votes.iter().map(|v| v.node_id).collect();
    let mut sorted = ids.clone();
    sorted.sort_unstable();
    sorted.dedup();
    assert_eq!(ids, sorted, "strictly increasing node ids");
}

/// One tally, one epoch. The height is fixed when the tally opens, and BOTH the
/// signature check and the stake read use it — a tally cannot verify against one
/// set and weigh against another.
#[test]
fn a_tally_reads_one_epoch_on_both_sides() {
    /// A set that exists at exactly one height. Outside it there are no keys and
    /// no stake, which is what an epoch boundary looks like to a reader.
    struct AtEpoch<'a> {
        inner: &'a ValidatorSet,
        epoch: u64,
    }
    impl VoteVerifier for AtEpoch<'_> {
        fn verify_vote(&self, n: &NodeId, m: &[u8], s: &[u8], h: u64) -> bool {
            h == self.epoch && self.inner.verify_vote(n, m, s, h)
        }
    }
    impl StakeSource for AtEpoch<'_> {
        fn weight(&self, n: &NodeId, h: u64) -> u64 {
            if h == self.epoch {
                self.inner.weight(n, h)
            } else {
                0
            }
        }
        fn signer_stake(&self, h: u64) -> u64 {
            if h == self.epoch {
                self.inner.signer_stake(h)
            } else {
                0
            }
        }
        fn signer_count(&self, h: u64) -> i64 {
            if h == self.epoch {
                self.inner.signer_count(h)
            } else {
                0
            }
        }
    }

    let (s, set) = committee(7, 1);
    let live = AtEpoch {
        inner: &set,
        epoch: EPOCH,
    };

    // At the epoch the tally opened at, everything resolves.
    let mut here = Tally::new(position(), Finality::Quasar, 5, EPOCH).expect("tally");
    for signer in s.iter().take(5) {
        assert!(here
            .add(&signer.vote(&position(), true), &live)
            .expect("held"));
    }
    here.cert(&live, &live).expect("a quorum at its own epoch");

    // A tally opened at another height reads that height on the SIGNATURE side:
    // no key resolves there, so nothing is even held, and the refusal is the
    // signature clause rather than a quorum one.
    let mut elsewhere = Tally::new(position(), Finality::Quasar, 5, EPOCH + 1).expect("tally");
    assert_eq!(elsewhere.epoch_height(), EPOCH + 1);
    assert_eq!(
        elsewhere.add(&s[0].vote(&position(), true), &live),
        Err(VoteError::Signature),
        "a signature is checked at the tally's epoch",
    );

    // And it reads that height on the STAKE side, which the same tally cannot
    // show — it holds nothing to weigh. So: a tally that DID hold votes, handed
    // a source alive only at some other epoch. The stake read is the tally's
    // epoch, so the source answers zero and the certificate fails closed.
    let dead = AtEpoch {
        inner: &set,
        epoch: EPOCH + 1,
    };
    assert_eq!(
        here.cert(&live, &dead),
        Err(VoteError::Cert(CertError::StakeZero {
            epoch_height: EPOCH
        })),
        "the stake is read at the tally's epoch, not at the source's",
    );
}

// ── The transport seam ──────────────────────────────────────────────

/// Consensus states the shape and opens no socket. A test carries a vote over a
/// vector; a node carries it over a mesh; the tally cannot tell.
#[test]
fn the_transport_is_a_seam_the_caller_fills() {
    use std::cell::RefCell;

    struct Echo(RefCell<Vec<SignedVote>>);
    impl VoteTransport for Echo {
        fn broadcast(&self, vote: &SignedVote) {
            self.0.borrow_mut().push(vote.clone());
        }
    }

    let (s, set) = committee(4, 1);
    let wire = Echo(RefCell::new(Vec::new()));
    for signer in &s {
        wire.broadcast(&signer.vote(&position(), true));
    }

    // An implementation may echo a vote back to its origin; the tally
    // deduplicates by signer, so a self-echo costs nothing.
    let mut t = tally(Finality::Quasar, 4);
    let sent = wire.0.borrow();
    for v in sent.iter().chain(sent.iter()) {
        t.add(v, &set).expect("held or already held");
    }
    assert_eq!(t.len(), 4, "four signers, however many times each spoke");
}

/// A certificate assembled outside a tally is subject to the same rule. There is
/// no second door: `QuorumCert::assemble` still produces something, and it is
/// `verify_weighted` that refuses it — which is exactly what `Tally::cert` runs.
#[test]
fn the_rule_is_the_same_rule_outside_a_tally() {
    let (s, set) = committee(7, 1);
    let pos = position();
    let votes: Vec<_> = s
        .iter()
        .take(2)
        .map(|x| x.vote(&pos, true).record())
        .collect();
    let loose = QuorumCert::assemble(Finality::Quasar, pos, 2, &votes).expect("assembles");
    assert!(
        loose.verify_weighted(&set, &set, EPOCH).is_err(),
        "assembling is not accepting, here as anywhere",
    );
}
