package qzmq

import (
    "encoding/binary"
    "fmt"
    "io"
)

// Message types
const (
    TypeClientHello = 0x01
    TypeServerHello = 0x02
    TypeClientKey   = 0x03
    TypeFinished    = 0x04
    TypeData        = 0x05
    TypeKeyUpdate   = 0x06
)

// ClientHello initiates the handshake
type ClientHello struct {
    Version      byte
    CipherSuites []byte
    X25519Public []byte
    Random       []byte
}

// Write serializes ClientHello
func (m *ClientHello) Write(w io.Writer) error {
    // Type
    if err := binary.Write(w, binary.BigEndian, TypeClientHello); err != nil {
        return err
    }
    // Version
    if err := binary.Write(w, binary.BigEndian, m.Version); err != nil {
        return err
    }
    // CipherSuites length and data
    if err := binary.Write(w, binary.BigEndian, uint16(len(m.CipherSuites))); err != nil {
        return err
    }
    if _, err := w.Write(m.CipherSuites); err != nil {
        return err
    }
    // X25519 public key
    if _, err := w.Write(m.X25519Public); err != nil {
        return err
    }
    // Random
    if _, err := w.Write(m.Random); err != nil {
        return err
    }
    return nil
}

// Read deserializes ClientHello
func (m *ClientHello) Read(r io.Reader) error {
    var msgType uint8
    if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
        return err
    }
    if msgType != TypeClientHello {
        return fmt.Errorf("expected ClientHello, got type %d", msgType)
    }
    
    if err := binary.Read(r, binary.BigEndian, &m.Version); err != nil {
        return err
    }
    
    var suitesLen uint16
    if err := binary.Read(r, binary.BigEndian, &suitesLen); err != nil {
        return err
    }
    m.CipherSuites = make([]byte, suitesLen)
    if _, err := io.ReadFull(r, m.CipherSuites); err != nil {
        return err
    }
    
    m.X25519Public = make([]byte, X25519KeySize)
    if _, err := io.ReadFull(r, m.X25519Public); err != nil {
        return err
    }
    
    m.Random = make([]byte, 32)
    if _, err := io.ReadFull(r, m.Random); err != nil {
        return err
    }
    
    return nil
}

// ServerHello responds to ClientHello
type ServerHello struct {
    CipherSuite  byte
    X25519Public []byte
    MLKEMPublic  []byte
    MLDSAPublic  []byte
    Random       []byte
    Signature    []byte
}

// Write serializes ServerHello
func (m *ServerHello) Write(w io.Writer) error {
    if err := binary.Write(w, binary.BigEndian, TypeServerHello); err != nil {
        return err
    }
    if err := binary.Write(w, binary.BigEndian, m.CipherSuite); err != nil {
        return err
    }
    if _, err := w.Write(m.X25519Public); err != nil {
        return err
    }
    if _, err := w.Write(m.MLKEMPublic); err != nil {
        return err
    }
    if _, err := w.Write(m.MLDSAPublic); err != nil {
        return err
    }
    if _, err := w.Write(m.Random); err != nil {
        return err
    }
    
    // Signature length and data
    if err := binary.Write(w, binary.BigEndian, uint16(len(m.Signature))); err != nil {
        return err
    }
    if _, err := w.Write(m.Signature); err != nil {
        return err
    }
    return nil
}

// Read deserializes ServerHello
func (m *ServerHello) Read(r io.Reader) error {
    var msgType uint8
    if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
        return err
    }
    if msgType != TypeServerHello {
        return fmt.Errorf("expected ServerHello, got type %d", msgType)
    }
    
    if err := binary.Read(r, binary.BigEndian, &m.CipherSuite); err != nil {
        return err
    }
    
    m.X25519Public = make([]byte, X25519KeySize)
    if _, err := io.ReadFull(r, m.X25519Public); err != nil {
        return err
    }
    
    m.MLKEMPublic = make([]byte, MLKEMKeySize)
    if _, err := io.ReadFull(r, m.MLKEMPublic); err != nil {
        return err
    }
    
    m.MLDSAPublic = make([]byte, MLDSAKeySize)
    if _, err := io.ReadFull(r, m.MLDSAPublic); err != nil {
        return err
    }
    
    m.Random = make([]byte, 32)
    if _, err := io.ReadFull(r, m.Random); err != nil {
        return err
    }
    
    var sigLen uint16
    if err := binary.Read(r, binary.BigEndian, &sigLen); err != nil {
        return err
    }
    m.Signature = make([]byte, sigLen)
    if _, err := io.ReadFull(r, m.Signature); err != nil {
        return err
    }
    
    return nil
}

// ClientKey contains KEM ciphertext
type ClientKey struct {
    KEMCiphertext []byte
    PSKBinder     []byte // Optional for resumption
    AuthSignature []byte // Optional for client auth
}

// Write serializes ClientKey
func (m *ClientKey) Write(w io.Writer) error {
    if err := binary.Write(w, binary.BigEndian, TypeClientKey); err != nil {
        return err
    }
    
    // KEM ciphertext length and data
    if err := binary.Write(w, binary.BigEndian, uint16(len(m.KEMCiphertext))); err != nil {
        return err
    }
    if _, err := w.Write(m.KEMCiphertext); err != nil {
        return err
    }
    
    // PSK binder (optional)
    if err := binary.Write(w, binary.BigEndian, uint16(len(m.PSKBinder))); err != nil {
        return err
    }
    if len(m.PSKBinder) > 0 {
        if _, err := w.Write(m.PSKBinder); err != nil {
            return err
        }
    }
    
    // Auth signature (optional)
    if err := binary.Write(w, binary.BigEndian, uint16(len(m.AuthSignature))); err != nil {
        return err
    }
    if len(m.AuthSignature) > 0 {
        if _, err := w.Write(m.AuthSignature); err != nil {
            return err
        }
    }
    
    return nil
}

// Read deserializes ClientKey
func (m *ClientKey) Read(r io.Reader) error {
    var msgType uint8
    if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
        return err
    }
    if msgType != TypeClientKey {
        return fmt.Errorf("expected ClientKey, got type %d", msgType)
    }
    
    var kemLen uint16
    if err := binary.Read(r, binary.BigEndian, &kemLen); err != nil {
        return err
    }
    m.KEMCiphertext = make([]byte, kemLen)
    if _, err := io.ReadFull(r, m.KEMCiphertext); err != nil {
        return err
    }
    
    var pskLen uint16
    if err := binary.Read(r, binary.BigEndian, &pskLen); err != nil {
        return err
    }
    if pskLen > 0 {
        m.PSKBinder = make([]byte, pskLen)
        if _, err := io.ReadFull(r, m.PSKBinder); err != nil {
            return err
        }
    }
    
    var authLen uint16
    if err := binary.Read(r, binary.BigEndian, &authLen); err != nil {
        return err
    }
    if authLen > 0 {
        m.AuthSignature = make([]byte, authLen)
        if _, err := io.ReadFull(r, m.AuthSignature); err != nil {
            return err
        }
    }
    
    return nil
}

// DataMessage carries encrypted application data
type DataMessage struct {
    StreamID uint32
    SeqNo    uint64
    Data     []byte // Encrypted data with AEAD tag
}

// Write serializes DataMessage
func (m *DataMessage) Write(w io.Writer) error {
    if err := binary.Write(w, binary.BigEndian, TypeData); err != nil {
        return err
    }
    if err := binary.Write(w, binary.BigEndian, m.StreamID); err != nil {
        return err
    }
    if err := binary.Write(w, binary.BigEndian, m.SeqNo); err != nil {
        return err
    }
    if err := binary.Write(w, binary.BigEndian, uint32(len(m.Data))); err != nil {
        return err
    }
    if _, err := w.Write(m.Data); err != nil {
        return err
    }
    return nil
}

// Read deserializes DataMessage
func (m *DataMessage) Read(r io.Reader) error {
    var msgType uint8
    if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
        return err
    }
    if msgType != TypeData {
        return fmt.Errorf("expected DataMessage, got type %d", msgType)
    }
    
    if err := binary.Read(r, binary.BigEndian, &m.StreamID); err != nil {
        return err
    }
    if err := binary.Read(r, binary.BigEndian, &m.SeqNo); err != nil {
        return err
    }
    
    var dataLen uint32
    if err := binary.Read(r, binary.BigEndian, &dataLen); err != nil {
        return err
    }
    
    m.Data = make([]byte, dataLen)
    if _, err := io.ReadFull(r, m.Data); err != nil {
        return err
    }
    
    return nil
}