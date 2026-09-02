// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The ZAP transport frame, in Rust.
//!
//! This is layer A only — how a message is put on a stream and taken off it.
//! Nothing here knows what a vote is, and nothing here knows what a chain is.
//!
//! It is the same wire the other two lanes already speak. Go writes it in
//! `luxfi/api/zap` (`transport.go`: `WriteMessage`/`ReadMessage`, `Buffer`,
//! `Reader`); C++ reads and writes it in `luxcpp/zap`
//! (`include/lux/zap/wire.hpp`), which names the Go file as its own reference.
//! Rust had no such reader, so the Rust lane could not carry a consensus vote
//! between validators at all. One frame:
//!
//! ```text
//! [4-byte BE length][1-byte type][`length` bytes of payload]
//! ```
//!
//! Big-endian everywhere, because that is what Go writes. A field written with
//! [`Writer::bytes`] is a 4-byte BE length followed by the raw bytes, matching
//! Go's `Buffer.WriteBytes` and C++'s `Writer::write_bytes` byte for byte.
//!
//! WHY NOT `hanzo-zap`: that crate speaks a different protocol that shares the
//! name — the arena message format of `luxfi/zap` (magic `ZAP\0`, a 16-byte
//! header, relative-offset slots) framed by a **little-endian** length and no
//! type byte. It is not this wire, and a vote written with it is not a frame
//! any Go or C++ peer can read.

use std::io::{self, Read, Write};

/// Set on every response frame (bit 7).
pub const RESPONSE_FLAG: u8 = 0x80;

/// Set on a response frame that carries an application error (bit 6).
pub const ERROR_FLAG: u8 = 0x40;

/// The bits a service-defined type id may use.
///
/// A type id with bit 6 or 7 set arrives at the far end looking like a flag, so
/// the id space is the low six bits and nothing above them.
pub const TYPE_MASK: u8 = 0x3F;

/// The largest payload a frame may announce: 16 MiB, as Go and C++ both cap it.
pub const MAX_MESSAGE: u32 = 16 * 1024 * 1024;

/// Length prefix plus type byte.
pub const HEADER_LEN: usize = 5;

/// The base type id, with the response and error flags removed.
pub const fn strip_flags(raw: u8) -> u8 {
    raw & TYPE_MASK
}

/// Whether the response flag is set.
pub const fn is_response(raw: u8) -> bool {
    raw & RESPONSE_FLAG != 0
}

/// Whether the error flag is set.
pub const fn is_error(raw: u8) -> bool {
    raw & ERROR_FLAG != 0
}

/// Big-endian encoder for a frame payload.
#[derive(Debug, Default, Clone)]
pub struct Writer {
    buf: Vec<u8>,
}

impl Writer {
    pub fn new() -> Self {
        Writer { buf: Vec::new() }
    }

    pub fn with_capacity(n: usize) -> Self {
        Writer {
            buf: Vec::with_capacity(n),
        }
    }

    pub fn u8(&mut self, v: u8) -> &mut Self {
        self.buf.push(v);
        self
    }

    pub fn u32(&mut self, v: u32) -> &mut Self {
        self.buf.extend_from_slice(&v.to_be_bytes());
        self
    }

    pub fn u64(&mut self, v: u64) -> &mut Self {
        self.buf.extend_from_slice(&v.to_be_bytes());
        self
    }

    /// A length-prefixed field: 4-byte BE length, then the bytes.
    pub fn bytes(&mut self, b: &[u8]) -> &mut Self {
        self.u32(b.len() as u32);
        self.buf.extend_from_slice(b);
        self
    }

    pub fn take(self) -> Vec<u8> {
        self.buf
    }

    pub fn as_slice(&self) -> &[u8] {
        &self.buf
    }
}

/// Big-endian decoder over a borrowed payload.
///
/// Every read is checked and returns `None` on a short buffer; there is no
/// panicking accessor, so a hostile payload cannot take the process down.
#[derive(Debug, Clone)]
pub struct Reader<'a> {
    data: &'a [u8],
    pos: usize,
}

impl<'a> Reader<'a> {
    pub fn new(data: &'a [u8]) -> Self {
        Reader { data, pos: 0 }
    }

    pub fn remaining(&self) -> usize {
        self.data.len() - self.pos
    }

    pub fn u8(&mut self) -> Option<u8> {
        let v = *self.data.get(self.pos)?;
        self.pos += 1;
        Some(v)
    }

    pub fn u32(&mut self) -> Option<u32> {
        let s: [u8; 4] = self.data.get(self.pos..self.pos + 4)?.try_into().ok()?;
        self.pos += 4;
        Some(u32::from_be_bytes(s))
    }

    pub fn u64(&mut self) -> Option<u64> {
        let s: [u8; 8] = self.data.get(self.pos..self.pos + 8)?.try_into().ok()?;
        self.pos += 8;
        Some(u64::from_be_bytes(s))
    }

    /// A length-prefixed field, borrowed out of the payload.
    ///
    /// The length is checked against what is left before anything is taken, so
    /// an announced length larger than the payload is a refusal and never an
    /// allocation.
    ///
    /// The end of the field is computed with `checked_add` because the length is
    /// a peer's 32-bit number and the position is ours. Where `usize` is 32 bits
    /// the sum of the two can overflow, and an overflow is a PANIC wherever
    /// overflow checks are on — so a peer that can announce a length can halt
    /// the process, which is the attack.
    ///
    /// It is NOT an over-read. A wrapped end lands BELOW the cursor, and a range
    /// whose start is past its end is one `get` answers `None` to, so the
    /// unchecked version refuses the same bytes wherever it survives them at
    /// all. The fix buys the refusal on a target where the alternative is
    /// dying — the width of a pointer is not a thing a wire format gets to
    /// depend on.
    pub fn bytes(&mut self) -> Option<&'a [u8]> {
        let n = self.u32()? as usize;
        let end = self.pos.checked_add(n)?;
        let s = self.data.get(self.pos..end)?;
        self.pos = end;
        Some(s)
    }
}

/// Write one frame: header then payload, in a single call to the stream.
///
/// One write, not two, because two writes on a shared stream can interleave with
/// another frame's header and desynchronise the reader for good.
pub fn write_frame<W: Write>(w: &mut W, msg_type: u8, payload: &[u8]) -> io::Result<()> {
    if payload.len() as u64 > MAX_MESSAGE as u64 {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "zap: frame too large",
        ));
    }
    let mut frame = Vec::with_capacity(HEADER_LEN + payload.len());
    frame.extend_from_slice(&(payload.len() as u32).to_be_bytes());
    frame.push(msg_type);
    frame.extend_from_slice(payload);
    w.write_all(&frame)
}

/// Read one frame, refusing an announced length above `max`.
///
/// `max` is the caller's statement of what this link carries. A link that only
/// ever carries a vote says so, and a peer that announces 16 MiB on it is
/// refused before a byte is allocated.
pub fn read_frame<R: Read>(r: &mut R, max: u32) -> io::Result<(u8, Vec<u8>)> {
    let mut header = [0u8; HEADER_LEN];
    r.read_exact(&mut header)?;
    let len = u32::from_be_bytes([header[0], header[1], header[2], header[3]]);
    if len > max.min(MAX_MESSAGE) {
        return Err(io::Error::new(
            io::ErrorKind::InvalidData,
            "zap: frame too large",
        ));
    }
    let mut payload = vec![0u8; len as usize];
    r.read_exact(&mut payload)?;
    Ok((header[4], payload))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_frame_is_a_be_length_then_a_type_then_the_payload() {
        let mut out = Vec::new();
        write_frame(&mut out, 0x2a, &[1, 2, 3]).expect("write");
        assert_eq!(out, vec![0, 0, 0, 3, 0x2a, 1, 2, 3]);
    }

    #[test]
    fn a_written_frame_reads_back() {
        let mut out = Vec::new();
        write_frame(&mut out, 62, b"hello").expect("write");
        let (t, p) = read_frame(&mut out.as_slice(), 1024).expect("read");
        assert_eq!(t, 62);
        assert_eq!(p, b"hello");
    }

    #[test]
    fn a_field_is_a_be_length_then_the_bytes() {
        let mut w = Writer::new();
        w.bytes(&[0xaa, 0xbb]);
        assert_eq!(w.as_slice(), &[0, 0, 0, 2, 0xaa, 0xbb]);
    }

    #[test]
    fn a_field_round_trips() {
        let mut w = Writer::new();
        w.bytes(b"one").bytes(b"").bytes(b"three");
        let buf = w.take();
        let mut r = Reader::new(&buf);
        assert_eq!(r.bytes(), Some(&b"one"[..]));
        assert_eq!(r.bytes(), Some(&b""[..]));
        assert_eq!(r.bytes(), Some(&b"three"[..]));
        assert_eq!(r.remaining(), 0);
    }

    #[test]
    fn an_over_long_length_is_refused_not_allocated() {
        // A four-gigabyte announcement over a two-byte payload.
        let hostile = [0xffu8, 0xff, 0xff, 0xff, 0x01, 0x02];
        let mut r = Reader::new(&hostile);
        assert_eq!(r.bytes(), None);
    }

    /// A length near `u32::MAX` over a five-byte payload is refused.
    ///
    /// On a 64-bit target — which is every target this suite runs on — that is
    /// the whole of what this can say: the sum cannot overflow there, so what is
    /// held is the bounds check and not the `checked_add`, and dropping the
    /// `checked_add` leaves this green. The overflow it guards is a 32-bit
    /// target's, no such target is in CI, and this test is the one that would
    /// carry the property the day one is.
    #[test]
    fn a_length_that_would_wrap_the_cursor_is_refused() {
        for announced in [u32::MAX, u32::MAX - 1, 1 << 31, 1 << 24] {
            let mut hostile = announced.to_be_bytes().to_vec();
            hostile.extend_from_slice(b"short");
            let mut r = Reader::new(&hostile);
            assert_eq!(r.bytes(), None, "announced {announced}");
            // And a refusal leaves the cursor where a caller can still see the
            // remaining bytes, rather than past the end of the buffer.
            assert!(r.remaining() <= hostile.len());
        }
    }

    #[test]
    fn a_frame_over_the_links_limit_is_refused() {
        let mut framed = Vec::new();
        write_frame(&mut framed, 62, &vec![0u8; 300]).expect("write");
        let err = read_frame(&mut framed.as_slice(), 188).expect_err("must refuse");
        assert_eq!(err.kind(), io::ErrorKind::InvalidData);
    }

    #[test]
    fn a_truncated_frame_is_an_error_not_a_short_payload() {
        let framed = [0u8, 0, 0, 8, 62, 1, 2, 3];
        let err = read_frame(&mut framed.as_slice(), 1024).expect_err("must refuse");
        assert_eq!(err.kind(), io::ErrorKind::UnexpectedEof);
    }

    /// The frame's constants are not this crate's to choose. Every one of them
    /// is read off `luxfi/api/zap` (`wire.go`) and matched by `luxcpp/zap`
    /// (`include/lux/zap/wire.hpp`); a frame written under any other value is
    /// one no Go or C++ peer can read. Pinning them here means a change to one
    /// of them stops being a silent fork of the wire.
    #[test]
    fn the_constants_are_the_ones_go_writes_and_cpp_reads() {
        // Go: MsgResponseFlag = 0x80, MsgErrorFlag = 0x40.
        // C++: MsgResponseFlag = 0x80, MsgErrorFlag = 0x40.
        assert_eq!(RESPONSE_FLAG, 0x80);
        assert_eq!(ERROR_FLAG, 0x40);
        // C++: MsgTypeMask = 0x3F. Go has no such constant and open-codes the
        // strip; `strip_flags_is_gos_own_expression` below is what holds them
        // together.
        assert_eq!(TYPE_MASK, 0x3F);
        // Go: MaxMessageSize = 16 * 1024 * 1024. C++: MaxMessageSize, same.
        assert_eq!(MAX_MESSAGE, 16 * 1024 * 1024);
        // Go: HeaderSize = 5. C++: HeaderSize = 5.
        assert_eq!(HEADER_LEN, 5);
    }

    /// Go names no mask: its call sites clear the two flag bits with `&^`, as in
    /// `respType &^ (MsgResponseFlag | MsgErrorFlag)`. Masking with `TYPE_MASK`
    /// is the same operation given a name, and this is the proof — over every
    /// byte, not over a sample.
    #[test]
    fn strip_flags_is_gos_own_expression() {
        for raw in 0..=u8::MAX {
            assert_eq!(
                strip_flags(raw),
                raw & !(RESPONSE_FLAG | ERROR_FLAG),
                "raw {raw}"
            );
        }
    }

    /// The header Go builds is `BigEndian.PutUint32(header[0:4], len(payload))`
    /// then `header[4] = byte(msgType)`. Written out, so an endianness that ever
    /// flipped would be caught by the bytes and not by a peer.
    #[test]
    fn the_header_is_big_endian_length_then_type() {
        let mut out = Vec::new();
        write_frame(&mut out, VOTE_LIKE, &[0xde, 0xad]).expect("write");
        assert_eq!(
            &out[..4],
            &[0x00, 0x00, 0x00, 0x02],
            "length is 4-byte big-endian"
        );
        assert_eq!(out[4], VOTE_LIKE, "then the type byte");
        assert_eq!(&out[5..], &[0xde, 0xad]);

        // A length that needs more than one byte, so a little-endian writer
        // could not pass by accident.
        let mut out = Vec::new();
        write_frame(&mut out, VOTE_LIKE, &vec![0u8; 258]).expect("write");
        assert_eq!(&out[..4], &[0x00, 0x00, 0x01, 0x02]);
    }

    /// `Buffer.WriteBytes` in Go writes a 4-byte big-endian length and then the
    /// raw bytes; `Writer::write_bytes` in C++ writes the same. A field is the
    /// same shape as a frame's length prefix and must not drift from it.
    #[test]
    fn a_field_is_big_endian_too() {
        let mut w = Writer::new();
        w.bytes(&vec![0u8; 300]);
        assert_eq!(&w.as_slice()[..4], &[0x00, 0x00, 0x01, 0x2c]);
    }

    /// A service id fit for this wire: the 60..63 block Go reserves for Quasar.
    const VOTE_LIKE: u8 = 62;

    #[test]
    fn flags_ride_above_the_id_space() {
        assert_eq!(strip_flags(62 | RESPONSE_FLAG | ERROR_FLAG), 62);
        assert!(is_response(62 | RESPONSE_FLAG));
        assert!(is_error(62 | ERROR_FLAG));
        assert!(!is_response(62));
    }
}
