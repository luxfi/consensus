package qzmq

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
	"time"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if len(kp.X25519Private) != X25519KeySize {
		t.Errorf("X25519Private wrong size: got %d, want %d", len(kp.X25519Private), X25519KeySize)
	}
	if len(kp.X25519Public) != X25519KeySize {
		t.Errorf("X25519Public wrong size: got %d, want %d", len(kp.X25519Public), X25519KeySize)
	}
	if len(kp.MLKEMPublic) != MLKEMKeySize {
		t.Errorf("MLKEMPublic wrong size: got %d, want %d", len(kp.MLKEMPublic), MLKEMKeySize)
	}
	if len(kp.MLDSAPublic) != MLDSAKeySize {
		t.Errorf("MLDSAPublic wrong size: got %d, want %d", len(kp.MLDSAPublic), MLDSAKeySize)
	}
}

func TestSession_EncryptDecrypt(t *testing.T) {
	// Generate keys
	serverKeys, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server keys: %v", err)
	}

	clientKeys, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client keys: %v", err)
	}

	// Create sessions
	serverSession, err := NewSession(serverKeys, true)
	if err != nil {
		t.Fatalf("Create server session: %v", err)
	}

	clientSession, err := NewSession(clientKeys, false)
	if err != nil {
		t.Fatalf("Create client session: %v", err)
	}

	// Generate shared keys for testing
	sharedKey1 := make([]byte, 32)
	sharedKey2 := make([]byte, 32)
	if _, err := rand.Read(sharedKey1); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}
	if _, err := rand.Read(sharedKey2); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	// Set up matching keys for both sessions
	// Server's send key = Client's recv key
	serverSession.sendKey = sharedKey1
	clientSession.recvKey = sharedKey1

	// Client's send key = Server's recv key
	clientSession.sendKey = sharedKey2
	serverSession.recvKey = sharedKey2

	// Initialize ciphers with the keys
	serverSession.suite = SuiteChaCha20Poly1305
	clientSession.suite = SuiteChaCha20Poly1305

	// Initialize ciphers from the existing keys
	if err := serverSession.initCiphers(); err != nil {
		t.Fatalf("Server initCiphers: %v", err)
	}
	if err := clientSession.initCiphers(); err != nil {
		t.Fatalf("Client initCiphers: %v", err)
	}

	// Test encryption/decryption
	tests := []struct {
		name string
		data []byte
	}{
		{"small message", []byte("hello")},
		{"medium message", bytes.Repeat([]byte("test"), 100)},
		{"large message", bytes.Repeat([]byte("data"), 1000)},
		{"binary data", []byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe, 0xfd}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Client encrypts
			ciphertext, err := clientSession.Encrypt(tt.data)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Server decrypts
			plaintext, err := serverSession.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if !bytes.Equal(plaintext, tt.data) {
				t.Errorf("Decrypted data mismatch: got %x, want %x", plaintext, tt.data)
			}

			// Server encrypts response
			response := append([]byte("response: "), tt.data...)
			ciphertext, err = serverSession.Encrypt(response)
			if err != nil {
				t.Fatalf("Server encrypt failed: %v", err)
			}

			// Client decrypts response
			plaintext, err = clientSession.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Client decrypt failed: %v", err)
			}

			if !bytes.Equal(plaintext, response) {
				t.Errorf("Response mismatch: got %x, want %x", plaintext, response)
			}
		})
	}
}

func TestSession_KeyRotation(t *testing.T) {
	keys, _ := GenerateKeyPair()
	session, _ := NewSession(keys, true)

	// Setup initial keys
	session.sendKey = make([]byte, 32)
	session.recvKey = make([]byte, 32)
	if _, err := rand.Read(session.sendKey); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}
	if _, err := rand.Read(session.recvKey); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}
	session.suite = SuiteChaCha20Poly1305
	if err := session.initCiphers(); err != nil {
		t.Fatalf("initCiphers failed: %v", err)
	}

	// Save original keys
	origSendKey := make([]byte, 32)
	origRecvKey := make([]byte, 32)
	copy(origSendKey, session.sendKey)
	copy(origRecvKey, session.recvKey)

	// Test rotation triggers
	tests := []struct {
		name     string
		setup    func()
		needsRot bool
	}{
		{
			name:     "fresh keys",
			setup:    func() {},
			needsRot: false,
		},
		{
			name: "max messages reached",
			setup: func() {
				session.msgCount = MaxMessagesPerKey
			},
			needsRot: true,
		},
		{
			name: "max bytes reached",
			setup: func() {
				session.msgCount = 0
				session.byteCount = MaxBytesPerKey
			},
			needsRot: true,
		},
		{
			name: "max age reached",
			setup: func() {
				session.msgCount = 0
				session.byteCount = 0
				session.keyTime = time.Now().Add(-MaxKeyAge - time.Second)
			},
			needsRot: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			if got := session.needsRotation(); got != tt.needsRot {
				t.Errorf("needsRotation() = %v, want %v", got, tt.needsRot)
			}
		})
	}

	// Test key rotation
	if err := session.RotateKeys(); err != nil {
		t.Fatalf("RotateKeys failed: %v", err)
	}

	// Verify keys changed
	if bytes.Equal(session.sendKey, origSendKey) {
		t.Error("Send key not rotated")
	}
	if bytes.Equal(session.recvKey, origRecvKey) {
		t.Error("Recv key not rotated")
	}

	// Verify counters reset
	if session.msgCount != 0 {
		t.Errorf("msgCount not reset: %d", session.msgCount)
	}
	if session.byteCount != 0 {
		t.Errorf("byteCount not reset: %d", session.byteCount)
	}
	if session.sendNonce != 0 {
		t.Errorf("sendNonce not reset: %d", session.sendNonce)
	}
}

func TestMessages_Serialization(t *testing.T) {
	t.Run("ClientHello", func(t *testing.T) {
		msg := &ClientHello{
			Version:      Version,
			CipherSuites: []byte{SuiteAES256GCM, SuiteChaCha20Poly1305},
			X25519Public: make([]byte, X25519KeySize),
			Random:       make([]byte, 32),
		}
		_, _ = rand.Read(msg.X25519Public)
		_, _ = rand.Read(msg.Random)

		buf := new(bytes.Buffer)
		if err := msg.Write(buf); err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		decoded := &ClientHello{}
		if err := decoded.Read(buf); err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if decoded.Version != msg.Version {
			t.Errorf("Version mismatch: got %d, want %d", decoded.Version, msg.Version)
		}
		if !bytes.Equal(decoded.CipherSuites, msg.CipherSuites) {
			t.Error("CipherSuites mismatch")
		}
		if !bytes.Equal(decoded.X25519Public, msg.X25519Public) {
			t.Error("X25519Public mismatch")
		}
		if !bytes.Equal(decoded.Random, msg.Random) {
			t.Error("Random mismatch")
		}
	})

	t.Run("ServerHello", func(t *testing.T) {
		msg := &ServerHello{
			CipherSuite:  SuiteAES256GCM,
			X25519Public: make([]byte, X25519KeySize),
			MLKEMPublic:  make([]byte, MLKEMKeySize),
			MLDSAPublic:  make([]byte, MLDSAKeySize),
			Random:       make([]byte, 32),
			Signature:    make([]byte, 2420),
		}
		_, _ = rand.Read(msg.X25519Public)
		_, _ = rand.Read(msg.MLKEMPublic)
		_, _ = rand.Read(msg.MLDSAPublic)
		_, _ = rand.Read(msg.Random)
		_, _ = rand.Read(msg.Signature)

		buf := new(bytes.Buffer)
		if err := msg.Write(buf); err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		decoded := &ServerHello{}
		if err := decoded.Read(buf); err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if decoded.CipherSuite != msg.CipherSuite {
			t.Errorf("CipherSuite mismatch: got %d, want %d", decoded.CipherSuite, msg.CipherSuite)
		}
		if !bytes.Equal(decoded.Signature, msg.Signature) {
			t.Error("Signature mismatch")
		}
	})

	t.Run("DataMessage", func(t *testing.T) {
		msg := &DataMessage{
			StreamID: 42,
			SeqNo:    12345,
			Data:     []byte("encrypted consensus message"),
		}

		buf := new(bytes.Buffer)
		if err := msg.Write(buf); err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		decoded := &DataMessage{}
		if err := decoded.Read(buf); err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if decoded.StreamID != msg.StreamID {
			t.Errorf("StreamID mismatch: got %d, want %d", decoded.StreamID, msg.StreamID)
		}
		if decoded.SeqNo != msg.SeqNo {
			t.Errorf("SeqNo mismatch: got %d, want %d", decoded.SeqNo, msg.SeqNo)
		}
		if !bytes.Equal(decoded.Data, msg.Data) {
			t.Error("Data mismatch")
		}
	})
}

func TestCipherSuiteSelection(t *testing.T) {
	tests := []struct {
		name   string
		suites []byte
		want   byte
	}{
		{"AES preferred", []byte{SuiteAES256GCM, SuiteChaCha20Poly1305}, SuiteAES256GCM},
		{"ChaCha only", []byte{SuiteChaCha20Poly1305}, SuiteChaCha20Poly1305},
		{"AES only", []byte{SuiteAES256GCM}, SuiteAES256GCM},
		{"unsupported", []byte{0xFF, 0xFE}, 0},
		{"empty", []byte{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectCipherSuite(tt.suites)
			if got != tt.want {
				t.Errorf("selectCipherSuite() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Benchmark tests
func BenchmarkEncrypt(b *testing.B) {
	keys, _ := GenerateKeyPair()
	session, _ := NewSession(keys, true)
	session.sendKey = make([]byte, 32)
	_, _ = rand.Read(session.sendKey)
	session.recvKey = make([]byte, 32)
	_, _ = rand.Read(session.recvKey)
	session.suite = SuiteChaCha20Poly1305
	if err := session.initCiphers(); err != nil {
		b.Fatalf("initCiphers failed: %v", err)
	}

	data := make([]byte, 1024)
	_, _ = rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := session.Encrypt(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecrypt is temporarily disabled due to nonce handling issues
// TODO: Fix nonce synchronization for benchmarking
// func BenchmarkDecrypt(b *testing.B) {
// 	keys, _ := GenerateKeyPair()
// 	session, _ := NewSession(keys, true)
// 	session.sendKey = make([]byte, 32)
// 	session.recvKey = make([]byte, 32)
// 	_, _ = rand.Read(session.sendKey)
// 	_, _ = rand.Read(session.recvKey)
// 	session.suite = SuiteChaCha20Poly1305
// 	if err := session.initCiphers(); err != nil {
// 		b.Fatalf("initCiphers failed: %v", err)
// 	}
//
// 	data := make([]byte, 1024)
// 	_, _ = rand.Read(data)
//
// 	// Pre-encrypt data
// 	ciphertext, _ := session.Encrypt(data)
// 	session.recvNonce = 0 // Reset for benchmark
//
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		session.recvNonce = 0 // Reset nonce for each iteration
// 		_, err := session.Decrypt(ciphertext)
// 		if err != nil {
// 			b.Fatal(err)
// 		}
// 	}
// }

func BenchmarkKeyRotation(b *testing.B) {
	keys, _ := GenerateKeyPair()
	session, _ := NewSession(keys, true)
	session.sendKey = make([]byte, 32)
	session.recvKey = make([]byte, 32)
	_, _ = rand.Read(session.sendKey)
	_, _ = rand.Read(session.recvKey)
	session.suite = SuiteChaCha20Poly1305
	if err := session.initCiphers(); err != nil {
		b.Fatalf("initCiphers failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := session.RotateKeys()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandshake(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Generate keys (this would be pre-generated in practice)
		serverKeys, _ := GenerateKeyPair()
		clientKeys, _ := GenerateKeyPair()

		// Create bidirectional pipes for communication
		clientToServer, serverFromClient := io.Pipe()
		serverToClient, clientFromServer := io.Pipe()

		// Run handshake in parallel
		done := make(chan error, 2)

		go func() {
			session, _ := NewSession(serverKeys, true)
			transport := struct {
				io.Reader
				io.Writer
			}{serverToClient, serverFromClient}
			done <- session.serverHandshake(transport)
		}()

		go func() {
			session, _ := NewSession(clientKeys, false)
			transport := struct {
				io.Reader
				io.Writer
			}{clientToServer, clientFromServer}
			done <- session.clientHandshake(transport)
		}()

		// Wait for both sides (with timeout in real code)
		<-done
		<-done

		clientToServer.Close()
		serverToClient.Close()
	}
}
