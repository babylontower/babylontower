package messaging

import (
	"crypto/ed25519"
	"testing"

	"babylontower/pkg/crypto"
	"babylontower/pkg/identity"
	pb "babylontower/pkg/proto"
	"babylontower/pkg/storage"
	"google.golang.org/protobuf/proto"
)

// Test helpers

// generateTestIdentity creates a test identity for Alice
func generateTestIdentity(t *testing.T) (*identity.Identity, ed25519.PrivateKey, []byte) {
	t.Helper()
	id, err := identity.GenerateIdentity()
	if err != nil {
		t.Fatalf("failed to generate identity: %v", err)
	}
	privKey := ed25519.PrivateKey(id.Ed25519PrivKey)
	return id, privKey, id.X25519PrivKey
}

// Test BuildMessage
func TestBuildMessage(t *testing.T) {
	text := "Hello, World!"
	timestamp := uint64(1234567890)

	msg := BuildMessage(text, timestamp)

	if msg.Text != text {
		t.Errorf("expected text %q, got %q", text, msg.Text)
	}
	if msg.Timestamp != timestamp {
		t.Errorf("expected timestamp %d, got %d", timestamp, msg.Timestamp)
	}
}

// Test BuildMessageNow
func TestBuildMessageNow(t *testing.T) {
	text := "Hello!"
	msg := BuildMessageNow(text)

	if msg.Text != text {
		t.Errorf("expected text %q, got %q", text, msg.Text)
	}
	if msg.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
}

// Test BuildEnvelope
func TestBuildEnvelope(t *testing.T) {
	// Generate recipient's X25519 key pair
	recipientX25519Pub, recipientX25519Priv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("failed to generate X25519 keys: %v", err)
	}

	plaintext := []byte("secret message")

	envelope, err := BuildEnvelope(plaintext, recipientX25519Pub)
	if err != nil {
		t.Fatalf("failed to build envelope: %v", err)
	}

	if len(envelope.Ciphertext) == 0 {
		t.Error("expected non-empty ciphertext")
	}
	if len(envelope.EphemeralPubkey) != crypto.SharedSecretSize {
		t.Errorf("expected ephemeral pubkey length %d, got %d", crypto.SharedSecretSize, len(envelope.EphemeralPubkey))
	}
	if len(envelope.Nonce) != crypto.NonceSize {
		t.Errorf("expected nonce length %d, got %d", crypto.NonceSize, len(envelope.Nonce))
	}

	// Test decryption
	sharedSecret, err := crypto.ComputeSharedSecret(recipientX25519Priv, envelope.EphemeralPubkey)
	if err != nil {
		t.Fatalf("failed to compute shared secret: %v", err)
	}

	decrypted, err := crypto.DecryptWithSharedSecret(sharedSecret, envelope.Nonce, envelope.Ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("expected plaintext %q, got %q", string(plaintext), string(decrypted))
	}
}

// Test BuildEnvelope with invalid key length
func TestBuildEnvelope_InvalidKeyLength(t *testing.T) {
	plaintext := []byte("test")

	_, err := BuildEnvelope(plaintext, []byte("invalid"))
	if err == nil {
		t.Error("expected error for invalid key length")
	}
}

// Test SignEnvelope
func TestSignEnvelope(t *testing.T) {
	envelope := &pb.Envelope{
		Ciphertext:      []byte("ciphertext"),
		EphemeralPubkey: []byte("ephemeral_pubkey"),
		Nonce:           []byte("nonce"),
	}

	_, privKey, _ := generateTestIdentity(t)

	signedEnvelope, err := SignEnvelope(envelope, privKey)
	if err != nil {
		t.Fatalf("failed to sign envelope: %v", err)
	}

	if len(signedEnvelope.Signature) != crypto.SignatureSize {
		t.Errorf("expected signature length %d, got %d", crypto.SignatureSize, len(signedEnvelope.Signature))
	}
	if len(signedEnvelope.SenderPubkey) != ed25519.PublicKeySize {
		t.Errorf("expected sender pubkey length %d, got %d", ed25519.PublicKeySize, len(signedEnvelope.SenderPubkey))
	}
}

// Test ParseSignedEnvelope
func TestParseSignedEnvelope(t *testing.T) {
	envelope := &pb.Envelope{
		Ciphertext:      []byte("ciphertext"),
		EphemeralPubkey: []byte("ephemeral_pubkey"),
		Nonce:           []byte("nonce"),
	}

	_, privKey, _ := generateTestIdentity(t)

	signedEnvelope, err := SignEnvelope(envelope, privKey)
	if err != nil {
		t.Fatalf("failed to sign envelope: %v", err)
	}

	data, err := proto.Marshal(signedEnvelope)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	parsed, err := ParseSignedEnvelope(data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if string(parsed.Envelope) != string(signedEnvelope.Envelope) {
		t.Error("envelope mismatch")
	}
	if string(parsed.Signature) != string(signedEnvelope.Signature) {
		t.Error("signature mismatch")
	}
	if string(parsed.SenderPubkey) != string(signedEnvelope.SenderPubkey) {
		t.Error("sender pubkey mismatch")
	}
}

// Test ParseSignedEnvelope invalid data
func TestParseSignedEnvelope_InvalidData(t *testing.T) {
	_, err := ParseSignedEnvelope([]byte("invalid data"))
	if err == nil {
		t.Error("expected error for invalid data")
	}
}

// Test ParseSignedEnvelope empty envelope
func TestParseSignedEnvelope_EmptyEnvelope(t *testing.T) {
	// Create a valid-looking but empty envelope structure
	data := []byte{0x0a, 0x00} // protobuf field 1, length 0
	_, err := ParseSignedEnvelope(data)
	if err == nil {
		t.Error("expected error for empty envelope")
	}
}

// Test ParseSignedEnvelope invalid signature length
func TestParseSignedEnvelope_InvalidSignatureLength(t *testing.T) {
	signedEnvelope := &pb.SignedEnvelope{
		Envelope:     []byte("envelope"),
		Signature:    []byte("short"), // Too short
		SenderPubkey: make([]byte, 32),
	}
	data, _ := proto.Marshal(signedEnvelope)
	_, err := ParseSignedEnvelope(data)
	if err == nil {
		t.Error("expected error for invalid signature length")
	}
}

// Test ParseSignedEnvelope invalid pubkey length
func TestParseSignedEnvelope_InvalidPubkeyLength(t *testing.T) {
	signedEnvelope := &pb.SignedEnvelope{
		Envelope:     []byte("envelope"),
		Signature:    make([]byte, 64),
		SenderPubkey: []byte("short"), // Too short
	}
	data, _ := proto.Marshal(signedEnvelope)
	_, err := ParseSignedEnvelope(data)
	if err == nil {
		t.Error("expected error for invalid pubkey length")
	}
}

// Test VerifyEnvelope
func TestVerifyEnvelope(t *testing.T) {
	envelope := &pb.Envelope{
		Ciphertext:      []byte("ciphertext"),
		EphemeralPubkey: []byte("ephemeral_pubkey"),
		Nonce:           []byte("nonce"),
	}

	_, privKey, _ := generateTestIdentity(t)

	signedEnvelope, err := SignEnvelope(envelope, privKey)
	if err != nil {
		t.Fatalf("failed to sign envelope: %v", err)
	}

	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		t.Fatalf("verification error: %v", err)
	}
	if !valid {
		t.Error("expected valid signature")
	}
}

// Test VerifyEnvelope with tampered data
func TestVerifyEnvelope_Tampered(t *testing.T) {
	envelope := &pb.Envelope{
		Ciphertext:      []byte("ciphertext"),
		EphemeralPubkey: []byte("ephemeral_pubkey"),
		Nonce:           []byte("nonce"),
	}

	_, privKey, _ := generateTestIdentity(t)

	signedEnvelope, err := SignEnvelope(envelope, privKey)
	if err != nil {
		t.Fatalf("failed to sign envelope: %v", err)
	}

	// Tamper with envelope
	signedEnvelope.Envelope = []byte("tampered")

	valid, err := VerifyEnvelope(signedEnvelope)
	if err != ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
	if valid {
		t.Error("expected invalid signature")
	}
}

// Test ParseEnvelope
func TestParseEnvelope(t *testing.T) {
	original := &pb.Envelope{
		Ciphertext:      []byte("ciphertext"),
		EphemeralPubkey: make([]byte, 32),
		Nonce:           make([]byte, 24),
	}

	signedEnvelope := &pb.SignedEnvelope{
		Envelope:     mustMarshal(t, original),
		Signature:    make([]byte, 64),
		SenderPubkey: make([]byte, 32),
	}

	parsed, err := ParseEnvelope(signedEnvelope)
	if err != nil {
		t.Fatalf("failed to parse envelope: %v", err)
	}

	if string(parsed.Ciphertext) != string(original.Ciphertext) {
		t.Error("ciphertext mismatch")
	}
}

// Test DecryptEnvelope
func TestDecryptEnvelope(t *testing.T) {
	// Generate key pairs
	_, senderEdPriv, _ := generateTestIdentity(t)
	recipientX25519Pub, recipientX25519Priv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("failed to generate X25519 keys: %v", err)
	}

	// Create message
	msg := BuildMessageNow("secret message")
	plaintext, _ := proto.Marshal(msg)

	// Build and sign envelope
	envelope, err := BuildEnvelope(plaintext, recipientX25519Pub)
	if err != nil {
		t.Fatalf("failed to build envelope: %v", err)
	}

	_, err = SignEnvelope(envelope, senderEdPriv)
	if err != nil {
		t.Fatalf("failed to sign envelope: %v", err)
	}

	// Decrypt
	decrypted, err := DecryptEnvelope(envelope, recipientX25519Priv)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	// Parse message
	decryptedMsg, err := ParseMessage(decrypted)
	if err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	if decryptedMsg.Text != "secret message" {
		t.Errorf("expected text %q, got %q", "secret message", decryptedMsg.Text)
	}
}

// Test CreateOutgoingMessage
func TestCreateOutgoingMessage(t *testing.T) {
	_, senderEdPriv, _ := generateTestIdentity(t)
	recipientX25519Pub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("failed to generate X25519 keys: %v", err)
	}

	signedEnvelope, err := CreateOutgoingMessage("Hello!", recipientX25519Pub, senderEdPriv)
	if err != nil {
		t.Fatalf("failed to create outgoing message: %v", err)
	}

	// Verify the envelope
	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		t.Fatalf("verification error: %v", err)
	}
	if !valid {
		t.Error("expected valid signature")
	}
}

// Test ProcessIncomingMessage
func TestProcessIncomingMessage(t *testing.T) {
	// Generate identities
	_, senderEdPriv, _ := generateTestIdentity(t)
	recipientX25519Pub, recipientX25519Priv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("failed to generate X25519 keys: %v", err)
	}

	// Create outgoing message
	signedEnvelope, err := CreateOutgoingMessage("Hello!", recipientX25519Pub, senderEdPriv)
	if err != nil {
		t.Fatalf("failed to create outgoing message: %v", err)
	}

	// Process incoming message
	msg, senderPubKey, err := ProcessIncomingMessage(signedEnvelope, recipientX25519Priv)
	if err != nil {
		t.Fatalf("failed to process incoming message: %v", err)
	}

	if msg.Text != "Hello!" {
		t.Errorf("expected text %q, got %q", "Hello!", msg.Text)
	}

	expectedSenderPubKey := senderEdPriv.Public().(ed25519.PublicKey)
	if string(senderPubKey) != string(expectedSenderPubKey) {
		t.Error("sender public key mismatch")
	}
}

// Test ProcessIncomingMessage with invalid signature
func TestProcessIncomingMessage_InvalidSignature(t *testing.T) {
	recipientX25519Pub, recipientX25519Priv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("failed to generate X25519 keys: %v", err)
	}
	_ = recipientX25519Pub // unused but kept for clarity

	// Create tampered envelope
	signedEnvelope := &pb.SignedEnvelope{
		Envelope:     []byte("tampered"),
		Signature:    make([]byte, 64),
		SenderPubkey: make([]byte, 32),
	}

	_, _, err = ProcessIncomingMessage(signedEnvelope, recipientX25519Priv)
	if err == nil {
		t.Error("expected error for invalid signature")
	}
}

// Test end-to-end message flow
func TestEndToEndMessageFlow(t *testing.T) {
	// Generate Alice's identity
	aliceID, aliceEdPriv, aliceX25519Priv := generateTestIdentity(t)
	aliceX25519Pub := aliceID.X25519PubKey

	// Generate Bob's identity
	bobID, bobEdPriv, bobX25519Priv := generateTestIdentity(t)
	bobX25519Pub := bobID.X25519PubKey

	// Alice sends message to Bob
	signedEnvelope, _, err := BuildMessageForTesting("Hello Bob!", bobX25519Pub, aliceEdPriv)
	if err != nil {
		t.Fatalf("Alice failed to build message: %v", err)
	}

	// Bob receives and processes message
	// First verify signature
	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		t.Fatalf("Bob failed to verify envelope: %v", err)
	}
	if !valid {
		t.Fatal("Bob got invalid signature")
	}

	// Parse envelope
	envelope, err := ParseEnvelope(signedEnvelope)
	if err != nil {
		t.Fatalf("Bob failed to parse envelope: %v", err)
	}

	// Decrypt envelope
	plaintext, err := DecryptEnvelope(envelope, bobX25519Priv)
	if err != nil {
		t.Fatalf("Bob failed to decrypt: %v", err)
	}

	// Parse message
	msg, err := ParseMessage(plaintext)
	if err != nil {
		t.Fatalf("Bob failed to parse message: %v", err)
	}

	if msg.Text != "Hello Bob!" {
		t.Errorf("Bob expected %q, got %q", "Hello Bob!", msg.Text)
	}

	// Verify sender is Alice
	senderPubKey := ed25519.PublicKey(signedEnvelope.SenderPubkey)
	expectedAlicePubKey := aliceEdPriv.Public().(ed25519.PublicKey)
	if string(senderPubKey) != string(expectedAlicePubKey) {
		t.Error("sender public key mismatch")
	}

	// Bob replies to Alice
	replyEnvelope, _, err := BuildMessageForTesting("Hi Alice!", aliceX25519Pub, bobEdPriv)
	if err != nil {
		t.Fatalf("Bob failed to build reply: %v", err)
	}

	// Alice receives and processes reply
	valid, err = VerifyEnvelope(replyEnvelope)
	if err != nil {
		t.Fatalf("Alice failed to verify reply: %v", err)
	}
	if !valid {
		t.Fatal("Alice got invalid signature")
	}

	replyEnv, err := ParseEnvelope(replyEnvelope)
	if err != nil {
		t.Fatalf("Alice failed to parse reply envelope: %v", err)
	}

	replyPlaintext, err := DecryptEnvelope(replyEnv, aliceX25519Priv)
	if err != nil {
		t.Fatalf("Alice failed to decrypt reply: %v", err)
	}

	replyMsg, err := ParseMessage(replyPlaintext)
	if err != nil {
		t.Fatalf("Alice failed to parse reply message: %v", err)
	}

	if replyMsg.Text != "Hi Alice!" {
		t.Errorf("Alice expected %q, got %q", "Hi Alice!", replyMsg.Text)
	}
}

// Test message serialization
func TestSerializeDeserializeEnvelope(t *testing.T) {
	envelope := &pb.SignedEnvelope{
		Envelope:     []byte("envelope data"),
		Signature:    make([]byte, 64),
		SenderPubkey: make([]byte, 32),
	}

	data, err := SerializeEnvelope(envelope)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := DeserializeEnvelope(data)
	if err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if string(parsed.Envelope) != string(envelope.Envelope) {
		t.Error("envelope mismatch")
	}
}

// Helper function
func mustMarshal(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return data
}

// Test Service with mock storage
func TestServiceWithMockStorage(t *testing.T) {
	// Create mock storage
	memStorage := storage.NewMemoryStorage()

	// Create test identity
	id, edPriv, x25519Priv := generateTestIdentity(t)
	x25519Pub := id.X25519PubKey

	// Create config
	config := &Config{
		OwnEd25519PrivKey: edPriv,
		OwnEd25519PubKey:  edPriv.Public().(ed25519.PublicKey),
		OwnX25519PrivKey:  x25519Priv,
		OwnX25519PubKey:   x25519Pub,
	}

	// Note: We can't fully test the service without a running IPFS node
	// This test verifies the service can be created
	service := NewService(config, memStorage, nil)
	if service == nil {
		t.Fatal("failed to create service")
	}

	// Verify initial state
	if service.IsStarted() {
		t.Error("service should not be started initially")
	}
}

// Test GetTopic
func TestGetTopic(t *testing.T) {
	memStorage := storage.NewMemoryStorage()
	id, edPriv, x25519Priv := generateTestIdentity(t)
	x25519Pub := id.X25519PubKey

	config := &Config{
		OwnEd25519PrivKey: edPriv,
		OwnEd25519PubKey:  edPriv.Public().(ed25519.PublicKey),
		OwnX25519PrivKey:  x25519Priv,
		OwnX25519PubKey:   x25519Pub,
	}

	service := NewService(config, memStorage, nil)
	topic := service.GetTopic()

	if topic == "" {
		t.Error("expected non-empty topic")
	}
}

// Test ReceiveMessageDirect
func TestReceiveMessageDirect(t *testing.T) {
	memStorage := storage.NewMemoryStorage()

	// Generate identities
	_, senderEdPriv, _ := generateTestIdentity(t)
	recipientID, recipientEdPriv, recipientX25519Priv := generateTestIdentity(t)
	recipientX25519Pub := recipientID.X25519PubKey

	// Create config for recipient
	config := &Config{
		OwnEd25519PrivKey: recipientEdPriv,
		OwnEd25519PubKey:  recipientEdPriv.Public().(ed25519.PublicKey),
		OwnX25519PrivKey:  recipientX25519Priv,
		OwnX25519PubKey:   recipientX25519Pub,
	}

	service := NewService(config, memStorage, nil)

	// Create a message from sender
	signedEnvelope, _, err := BuildMessageForTesting("Hello!", recipientX25519Pub, senderEdPriv)
	if err != nil {
		t.Fatalf("failed to build message: %v", err)
	}

	// Serialize envelope
	envelopeBytes, err := SerializeEnvelope(signedEnvelope)
	if err != nil {
		t.Fatalf("failed to serialize envelope: %v", err)
	}

	// Try to receive (will fail because service not started)
	_, err = service.ReceiveMessage(envelopeBytes)
	if err != ErrServiceNotStarted {
		t.Errorf("expected ErrServiceNotStarted, got %v", err)
	}
}

// Test GetMessages
func TestGetMessages(t *testing.T) {
	memStorage := storage.NewMemoryStorage()
	id, edPriv, x25519Priv := generateTestIdentity(t)
	x25519Pub := id.X25519PubKey

	config := &Config{
		OwnEd25519PrivKey: edPriv,
		OwnEd25519PubKey:  edPriv.Public().(ed25519.PublicKey),
		OwnX25519PrivKey:  x25519Priv,
		OwnX25519PubKey:   x25519Pub,
	}

	service := NewService(config, memStorage, nil)

	// Try to get messages (will fail because service not started)
	_, err := service.GetMessages([]byte("contact"), 10, 0)
	if err != ErrServiceNotStarted {
		t.Errorf("expected ErrServiceNotStarted, got %v", err)
	}
}

// Test GetDecryptedMessages
func TestGetDecryptedMessages(t *testing.T) {
	memStorage := storage.NewMemoryStorage()
	id, edPriv, x25519Priv := generateTestIdentity(t)
	x25519Pub := id.X25519PubKey

	config := &Config{
		OwnEd25519PrivKey: edPriv,
		OwnEd25519PubKey:  edPriv.Public().(ed25519.PublicKey),
		OwnX25519PrivKey:  x25519Priv,
		OwnX25519PubKey:   x25519Pub,
	}

	service := NewService(config, memStorage, nil)

	// Try to get decrypted messages (will fail because service not started)
	_, err := service.GetDecryptedMessages([]byte("contact"), 10, 0)
	if err != ErrServiceNotStarted {
		t.Errorf("expected ErrServiceNotStarted, got %v", err)
	}
}

// Test SendMessage without IPFS
func TestSendMessage_NoIPFS(t *testing.T) {
	memStorage := storage.NewMemoryStorage()
	id, edPriv, x25519Priv := generateTestIdentity(t)
	x25519Pub := id.X25519PubKey

	config := &Config{
		OwnEd25519PrivKey: edPriv,
		OwnEd25519PubKey:  edPriv.Public().(ed25519.PublicKey),
		OwnX25519PrivKey:  x25519Priv,
		OwnX25519PubKey:   x25519Pub,
	}

	service := NewService(config, memStorage, nil)

	// Generate recipient keys
	recipientX25519Pub, _, _ := crypto.GenerateX25519KeyPair()
	recipientEdPub, _, _ := crypto.GenerateEd25519KeyPair()

	// Try to send (will fail because service not started)
	_, err := service.SendMessage("Hello!", recipientEdPub, recipientX25519Pub)
	if err != ErrServiceNotStarted {
		t.Errorf("expected ErrServiceNotStarted, got %v", err)
	}
}

// Test SerializeDeserializeEnvelope roundtrip
func TestSerializeDeserializeEnvelopeRoundtrip(t *testing.T) {
	original := &pb.SignedEnvelope{
		Envelope:     []byte("test envelope data"),
		Signature:    make([]byte, 64),
		SenderPubkey: make([]byte, 32),
	}

	data, err := SerializeEnvelope(original)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	parsed, err := DeserializeEnvelope(data)
	if err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if string(parsed.Envelope) != string(original.Envelope) {
		t.Error("envelope mismatch")
	}
	if string(parsed.Signature) != string(original.Signature) {
		t.Error("signature mismatch")
	}
	if string(parsed.SenderPubkey) != string(original.SenderPubkey) {
		t.Error("sender pubkey mismatch")
	}
}

// Test helper functions
func TestBuildMessageForTesting(t *testing.T) {
	_, senderEdPriv, _ := generateTestIdentity(t)
	recipientX25519Pub, _, _ := crypto.GenerateX25519KeyPair()

	signedEnvelope, msg, err := BuildMessageForTesting("Test message", recipientX25519Pub, senderEdPriv)
	if err != nil {
		t.Fatalf("failed to build message: %v", err)
	}

	if msg.Text != "Test message" {
		t.Errorf("expected text %q, got %q", "Test message", msg.Text)
	}

	// Verify the envelope
	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		t.Fatalf("verification error: %v", err)
	}
	if !valid {
		t.Error("expected valid signature")
	}
}
