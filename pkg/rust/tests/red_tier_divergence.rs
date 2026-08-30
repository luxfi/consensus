//! RED: where the three legs refuse a tier byte.
//!
//! Go's UnmarshalQuorumCert CARRIES any tier byte and refuses at Verify with
//! ErrQCUnknownTier. The C++ Cert::decode deliberately does the same
//! ("The tier byte is CARRIED, not judged, exactly as Go's decoder carries it").
//! Rust's Cert::decode JUDGES it: `_ => return Err(Refusal::Tier)`.
//! No corpus row carries a tier outside 0..=4, so nothing catches the difference.
use lux_consensus::finality::*;

#[test]
fn tier_byte_is_judged_at_decode_not_carried() {
    let raw = std::fs::read_to_string("/home/z/work/lux/consensus2/vectors/cert_verify.json")
        .expect("corpus");
    let rows: serde_json::Value = serde_json::from_str(&raw).unwrap();
    let wire_hex = rows
        .as_array()
        .unwrap()
        .iter()
        .find(|r| r["name"] == "verify/ok_4")
        .expect("ok_4")["wire"]
        .as_str()
        .unwrap()
        .to_string();
    let wire = hex::decode(wire_hex).unwrap();

    for tier in [0u8, 1, 2, 3, 4, 5, 7, 255] {
        let mut w = wire.clone();
        w[3] = tier; // version:2 role:1 -> tier at offset 3
        match Cert::decode(&w) {
            Ok(c) => println!("tier={tier:3}  Rust decode=OK   carried tier={:?}", c.tier),
            Err(e) => println!("tier={tier:3}  Rust decode=REFUSED {e:?}   <-- Go and C++ decode OK here"),
        }
    }
}
