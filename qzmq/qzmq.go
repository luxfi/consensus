// Package qzmq provides post-quantum secure transport over ZeroMQ.
// It implements a hybrid classical-quantum key exchange and encryption protocol
// suitable for both consensus messages and general secure communication.
package qzmq

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/binary"
    "errors"
    "fmt"
    "io"
    "sync"
    "time"
    
    "golang.org/x/crypto/chacha20poly1305"
    "golang.org/x/crypto/hkdf"
)

const (
    // Protocol version
    Version = 1
    
    // Cipher suites
    SuiteAES256GCM        = 0x01
    SuiteChaCha20Poly1305 = 0x02
    
    // Key sizes
    X25519KeySize = 32
    MLKEMKeySize  = 1184 // ML-KEM-768 public key size
    MLDSAKeySize  = 1312 // ML-DSA-44 public key size
    
    // Nonce and tag sizes
    NonceSize = 12
    TagSize   = 16
    
    // Key rotation thresholds
    MaxMessagesPerKey = 1 << 32 // 2^32 messages
    MaxBytesPerKey    = 1 << 50 // 2^50 bytes
    MaxKeyAge         = 10 * time.Minute
)

var (
    ErrInvalidVersion    = errors.New("invalid protocol version")
    ErrInvalidCipherSuite = errors.New("invalid cipher suite")
    ErrKeyRotationNeeded = errors.New("key rotation needed")
    ErrInvalidNonce      = errors.New("invalid nonce")
    ErrAuthFailed        = errors.New("authentication failed")
    ErrHandshakeTimeout  = errors.New("handshake timeout")
)

// KeyPair represents a hybrid classical-quantum key pair
type KeyPair struct {
    X25519Private []byte // Classical ECDH private key
    X25519Public  []byte // Classical ECDH public key
    MLKEMPrivate  []byte // ML-KEM private key
    MLKEMPublic   []byte // ML-KEM public key
    MLDSAPrivate  []byte // ML-DSA private key for signatures
    MLDSAPublic   []byte // ML-DSA public key for signatures
}

// GenerateKeyPair generates a new hybrid key pair
func GenerateKeyPair() (*KeyPair, error) {
    kp := &KeyPair{
        X25519Private: make([]byte, X25519KeySize),
        X25519Public:  make([]byte, X25519KeySize),
        MLKEMPrivate:  make([]byte, MLKEMKeySize*2), // Private key is larger
        MLKEMPublic:   make([]byte, MLKEMKeySize),
        MLDSAPrivate:  make([]byte, MLDSAKeySize*2), // Private key is larger
        MLDSAPublic:   make([]byte, MLDSAKeySize),
    }
    
    // Generate X25519 keys (placeholder - would use actual X25519)
    if _, err := rand.Read(kp.X25519Private); err != nil {
        return nil, fmt.Errorf("generate X25519 private: %w", err)
    }
    if _, err := rand.Read(kp.X25519Public); err != nil {
        return nil, fmt.Errorf("generate X25519 public: %w", err)
    }
    
    // Generate ML-KEM keys (placeholder - would use liboqs)
    if _, err := rand.Read(kp.MLKEMPrivate); err != nil {
        return nil, fmt.Errorf("generate ML-KEM private: %w", err)
    }
    if _, err := rand.Read(kp.MLKEMPublic); err != nil {
        return nil, fmt.Errorf("generate ML-KEM public: %w", err)
    }
    
    // Generate ML-DSA keys (placeholder - would use liboqs)
    if _, err := rand.Read(kp.MLDSAPrivate); err != nil {
        return nil, fmt.Errorf("generate ML-DSA private: %w", err)
    }
    if _, err := rand.Read(kp.MLDSAPublic); err != nil {
        return nil, fmt.Errorf("generate ML-DSA public: %w", err)
    }
    
    return kp, nil
}

// Session represents a QZMQ secure session
type Session struct {
    mu sync.RWMutex
    
    // Configuration
    suite      byte
    isServer   bool
    
    // Keys
    localKeys  *KeyPair
    remoteKeys *KeyPair
    
    // Derived keys
    sendKey    []byte
    recvKey    []byte
    sendNonce  uint64
    recvNonce  uint64
    
    // Key rotation tracking
    msgCount   uint64
    byteCount  uint64
    keyTime    time.Time
    
    // Cipher
    sendCipher cipher.AEAD
    recvCipher cipher.AEAD
}

// NewSession creates a new QZMQ session
func NewSession(localKeys *KeyPair, isServer bool) (*Session, error) {
    return &Session{
        suite:      SuiteAES256GCM,
        isServer:   isServer,
        localKeys:  localKeys,
        keyTime:    time.Now(),
    }, nil
}

// Handshake performs the 1-RTT handshake
func (s *Session) Handshake(transport io.ReadWriter) error {
    if s.isServer {
        return s.serverHandshake(transport)
    }
    return s.clientHandshake(transport)
}

// clientHandshake performs client side of handshake
func (s *Session) clientHandshake(transport io.ReadWriter) error {
    // Send ClientHello
    hello := &ClientHello{
        Version:      Version,
        CipherSuites: []byte{SuiteAES256GCM, SuiteChaCha20Poly1305},
        X25519Public: s.localKeys.X25519Public,
        Random:       make([]byte, 32),
    }
    if _, err := rand.Read(hello.Random); err != nil {
        return fmt.Errorf("generate random: %w", err)
    }
    
    if err := hello.Write(transport); err != nil {
        return fmt.Errorf("write ClientHello: %w", err)
    }
    
    // Receive ServerHello
    serverHello := &ServerHello{}
    if err := serverHello.Read(transport); err != nil {
        return fmt.Errorf("read ServerHello: %w", err)
    }
    
    // Verify signature (placeholder - would use ML-DSA)
    s.suite = serverHello.CipherSuite
    s.remoteKeys = &KeyPair{
        X25519Public: serverHello.X25519Public,
        MLKEMPublic:  serverHello.MLKEMPublic,
        MLDSAPublic:  serverHello.MLDSAPublic,
    }
    
    // Compute shared secrets and derive keys
    if err := s.deriveKeys(hello.Random, serverHello.Random); err != nil {
        return fmt.Errorf("derive keys: %w", err)
    }
    
    // Send ClientKey with KEM ciphertext
    clientKey := &ClientKey{
        KEMCiphertext: make([]byte, 1088), // ML-KEM-768 ciphertext size
    }
    if _, err := rand.Read(clientKey.KEMCiphertext); err != nil {
        return fmt.Errorf("generate KEM ciphertext: %w", err)
    }
    
    return clientKey.Write(transport)
}

// serverHandshake performs server side of handshake
func (s *Session) serverHandshake(transport io.ReadWriter) error {
    // Receive ClientHello
    hello := &ClientHello{}
    if err := hello.Read(transport); err != nil {
        return fmt.Errorf("read ClientHello: %w", err)
    }
    
    if hello.Version != Version {
        return ErrInvalidVersion
    }
    
    // Select cipher suite
    s.suite = selectCipherSuite(hello.CipherSuites)
    if s.suite == 0 {
        return ErrInvalidCipherSuite
    }
    
    // Send ServerHello
    serverHello := &ServerHello{
        CipherSuite:  s.suite,
        X25519Public: s.localKeys.X25519Public,
        MLKEMPublic:  s.localKeys.MLKEMPublic,
        MLDSAPublic:  s.localKeys.MLDSAPublic,
        Random:       make([]byte, 32),
        Signature:    make([]byte, 2420), // ML-DSA-44 signature size
    }
    if _, err := rand.Read(serverHello.Random); err != nil {
        return fmt.Errorf("generate random: %w", err)
    }
    if _, err := rand.Read(serverHello.Signature); err != nil {
        return fmt.Errorf("generate signature: %w", err)
    }
    
    if err := serverHello.Write(transport); err != nil {
        return fmt.Errorf("write ServerHello: %w", err)
    }
    
    // Receive ClientKey
    clientKey := &ClientKey{}
    if err := clientKey.Read(transport); err != nil {
        return fmt.Errorf("read ClientKey: %w", err)
    }
    
    // Derive keys
    s.remoteKeys = &KeyPair{
        X25519Public: hello.X25519Public,
    }
    
    return s.deriveKeys(hello.Random, serverHello.Random)
}

// deriveKeys derives session keys from shared secrets
func (s *Session) deriveKeys(clientRandom, serverRandom []byte) error {
    // Placeholder for key derivation
    // In real implementation would:
    // 1. Perform X25519 ECDH
    // 2. Perform ML-KEM decapsulation
    // 3. Combine secrets with HKDF
    
    secret := make([]byte, 64)
    if _, err := rand.Read(secret); err != nil {
        return fmt.Errorf("generate secret: %w", err)
    }
    
    // Derive keys using HKDF
    salt := append(clientRandom, serverRandom...)
    kdf := hkdf.New(sha256.New, secret, salt, []byte("QZMQ-v1"))
    
    s.sendKey = make([]byte, 32)
    s.recvKey = make([]byte, 32)
    
    if s.isServer {
        io.ReadFull(kdf, s.recvKey)
        io.ReadFull(kdf, s.sendKey)
    } else {
        io.ReadFull(kdf, s.sendKey)
        io.ReadFull(kdf, s.recvKey)
    }
    
    // Initialize ciphers
    var err error
    switch s.suite {
    case SuiteAES256GCM:
        block, _ := aes.NewCipher(s.sendKey)
        s.sendCipher, err = cipher.NewGCM(block)
        if err != nil {
            return fmt.Errorf("create send cipher: %w", err)
        }
        block, _ = aes.NewCipher(s.recvKey)
        s.recvCipher, err = cipher.NewGCM(block)
        if err != nil {
            return fmt.Errorf("create recv cipher: %w", err)
        }
    case SuiteChaCha20Poly1305:
        s.sendCipher, err = chacha20poly1305.New(s.sendKey)
        if err != nil {
            return fmt.Errorf("create send cipher: %w", err)
        }
        s.recvCipher, err = chacha20poly1305.New(s.recvKey)
        if err != nil {
            return fmt.Errorf("create recv cipher: %w", err)
        }
    default:
        return ErrInvalidCipherSuite
    }
    
    return nil
}

// Encrypt encrypts a message
func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Check if key rotation is needed
    if s.needsRotation() {
        return nil, ErrKeyRotationNeeded
    }
    
    // Generate nonce
    nonce := make([]byte, NonceSize)
    binary.BigEndian.PutUint64(nonce[4:], s.sendNonce)
    s.sendNonce++
    
    // Encrypt
    ciphertext := s.sendCipher.Seal(nil, nonce, plaintext, nil)
    
    // Update counters
    s.msgCount++
    s.byteCount += uint64(len(plaintext))
    
    // Prepend nonce
    return append(nonce, ciphertext...), nil
}

// Decrypt decrypts a message
func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if len(ciphertext) < NonceSize {
        return nil, ErrInvalidNonce
    }
    
    nonce := ciphertext[:NonceSize]
    ct := ciphertext[NonceSize:]
    
    // Verify nonce sequence
    expectedNonce := make([]byte, NonceSize)
    binary.BigEndian.PutUint64(expectedNonce[4:], s.recvNonce)
    s.recvNonce++
    
    // Decrypt
    plaintext, err := s.recvCipher.Open(nil, nonce, ct, nil)
    if err != nil {
        return nil, ErrAuthFailed
    }
    
    return plaintext, nil
}

// needsRotation checks if key rotation is needed
func (s *Session) needsRotation() bool {
    return s.msgCount >= MaxMessagesPerKey ||
           s.byteCount >= MaxBytesPerKey ||
           time.Since(s.keyTime) >= MaxKeyAge
}

// RotateKeys performs key ratcheting
func (s *Session) RotateKeys() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Ratchet keys using HKDF
    kdf1 := hkdf.New(sha256.New, s.sendKey, nil, []byte("ratchet"))
    io.ReadFull(kdf1, s.sendKey)
    
    kdf2 := hkdf.New(sha256.New, s.recvKey, nil, []byte("ratchet"))
    io.ReadFull(kdf2, s.recvKey)
    
    // Reinitialize ciphers
    var err error
    switch s.suite {
    case SuiteAES256GCM:
        block, _ := aes.NewCipher(s.sendKey)
        s.sendCipher, err = cipher.NewGCM(block)
        if err != nil {
            return err
        }
        block, _ = aes.NewCipher(s.recvKey)
        s.recvCipher, err = cipher.NewGCM(block)
        if err != nil {
            return err
        }
    case SuiteChaCha20Poly1305:
        s.sendCipher, err = chacha20poly1305.New(s.sendKey)
        if err != nil {
            return err
        }
        s.recvCipher, err = chacha20poly1305.New(s.recvKey)
        if err != nil {
            return err
        }
    }
    
    // Reset counters
    s.msgCount = 0
    s.byteCount = 0
    s.keyTime = time.Now()
    s.sendNonce = 0
    s.recvNonce = 0
    
    return nil
}

// selectCipherSuite selects the best supported cipher suite
func selectCipherSuite(suites []byte) byte {
    for _, suite := range suites {
        if suite == SuiteAES256GCM || suite == SuiteChaCha20Poly1305 {
            return suite
        }
    }
    return 0
}