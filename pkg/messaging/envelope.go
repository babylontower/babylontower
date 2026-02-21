package messaging

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"babylontower/pkg/crypto"
	pb "babylontower/pkg/proto"
	"golang.org/x/crypto/curve25519"
	"google.golang.org/protobuf/proto"
)

var (
	// ErrInvalidSignature is returned when signature verification fails
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrInvalidEnvelope is returned when envelope parsing fails
	ErrInvalidEnvelope = errors.New("invalid envelope")
	// ErrDecryptionFailed is returned when message decryption fails
	ErrDecryptionFailed = errors.New("decryption failed")
)

// BuildMessage creates a new Message protobuf with the given text and timestamp
func BuildMessage(text string, timestamp uint64) *pb.Message {
	return &pb.Message{
		Text:      text,
		Timestamp: timestamp,
	}
}

// BuildMessageNow creates a new Message protobuf with the current timestamp
func BuildMessageNow(text string) *pb.Message {
	return BuildMessage(text, uint64(time.Now().Unix()))
}

// BuildEnvelope encrypts a message for the recipient and creates an Envelope
// Returns the envelope, ephemeral private key (for testing), and error
func BuildEnvelope(plaintext []byte, recipientX25519PubKey []byte) (*pb.Envelope, error) {
	if len(recipientX25519PubKey) != crypto.SharedSecretSize {
		return nil, fmt.Errorf("invalid recipient public key length: %d", len(recipientX25519PubKey))
	}

	// Generate ephemeral X25519 key pair
	ephemeralPub, ephemeralPriv, err := generateEphemeralKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Compute shared secret: X25519(ephemeral_priv, recipient_static_pub)
	sharedSecret, err := crypto.ComputeSharedSecret(ephemeralPriv, recipientX25519PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	// Encrypt with shared secret
	nonce, ciphertext, err := crypto.EncryptWithSharedSecret(sharedSecret, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt: %w", err)
	}

	// Build envelope
	envelope := &pb.Envelope{
		Ciphertext:      ciphertext,
		EphemeralPubkey: ephemeralPub,
		Nonce:           nonce,
	}

	return envelope, nil
}

// SignEnvelope signs an envelope with the sender's Ed25519 private key
// Returns a SignedEnvelope containing the serialized envelope, signature, and sender's public key
func SignEnvelope(envelope *pb.Envelope, senderPrivKey ed25519.PrivateKey) (*pb.SignedEnvelope, error) {
	// Serialize envelope
	envelopeBytes, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal envelope: %w", err)
	}

	// Sign the serialized envelope
	signature := ed25519.Sign(senderPrivKey, envelopeBytes)

	// Get sender's public key
	senderPubKey := senderPrivKey.Public().(ed25519.PublicKey)

	// Build signed envelope
	signedEnvelope := &pb.SignedEnvelope{
		Envelope:     envelopeBytes,
		Signature:    signature,
		SenderPubkey: senderPubKey,
	}

	return signedEnvelope, nil
}

// ParseSignedEnvelope parses a SignedEnvelope from bytes
func ParseSignedEnvelope(data []byte) (*pb.SignedEnvelope, error) {
	var signedEnvelope pb.SignedEnvelope
	if err := proto.Unmarshal(data, &signedEnvelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal signed envelope: %w", err)
	}

	// Basic validation
	if len(signedEnvelope.Envelope) == 0 {
		return nil, fmt.Errorf("%w: empty envelope", ErrInvalidEnvelope)
	}
	if len(signedEnvelope.Signature) != crypto.SignatureSize {
		return nil, fmt.Errorf("%w: invalid signature length", ErrInvalidEnvelope)
	}
	if len(signedEnvelope.SenderPubkey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: invalid sender public key length", ErrInvalidEnvelope)
	}

	return &signedEnvelope, nil
}

// VerifyEnvelope verifies the signature on a SignedEnvelope
// Returns true if the signature is valid, false otherwise
func VerifyEnvelope(signedEnvelope *pb.SignedEnvelope) (bool, error) {
	if signedEnvelope == nil {
		return false, fmt.Errorf("nil envelope")
	}

	// Parse sender's public key
	if len(signedEnvelope.SenderPubkey) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid sender public key length")
	}
	senderPubKey := ed25519.PublicKey(signedEnvelope.SenderPubkey)

	// Verify signature
	valid := ed25519.Verify(senderPubKey, signedEnvelope.Envelope, signedEnvelope.Signature)
	if !valid {
		return false, ErrInvalidSignature
	}

	return true, nil
}

// ParseEnvelope parses an Envelope from bytes within a SignedEnvelope
func ParseEnvelope(signedEnvelope *pb.SignedEnvelope) (*pb.Envelope, error) {
	if signedEnvelope == nil || len(signedEnvelope.Envelope) == 0 {
		return nil, ErrInvalidEnvelope
	}

	var envelope pb.Envelope
	if err := proto.Unmarshal(signedEnvelope.Envelope, &envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}

	// Basic validation
	if len(envelope.Ciphertext) == 0 {
		return nil, fmt.Errorf("%w: empty ciphertext", ErrInvalidEnvelope)
	}
	if len(envelope.EphemeralPubkey) != crypto.SharedSecretSize {
		return nil, fmt.Errorf("%w: invalid ephemeral pubkey length", ErrInvalidEnvelope)
	}
	if len(envelope.Nonce) != crypto.NonceSize {
		return nil, fmt.Errorf("%w: invalid nonce length", ErrInvalidEnvelope)
	}

	return &envelope, nil
}

// DecryptEnvelope decrypts an envelope using the recipient's X25519 private key
// Returns the plaintext message bytes
func DecryptEnvelope(envelope *pb.Envelope, recipientX25519PrivKey []byte) ([]byte, error) {
	if envelope == nil {
		return nil, ErrInvalidEnvelope
	}
	if len(recipientX25519PrivKey) != crypto.SharedSecretSize {
		return nil, fmt.Errorf("invalid recipient private key length")
	}

	// Compute shared secret: X25519(recipient_static_priv, ephemeral_pub)
	sharedSecret, err := crypto.ComputeSharedSecret(recipientX25519PrivKey, envelope.EphemeralPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	// Decrypt ciphertext
	plaintext, err := crypto.DecryptWithSharedSecret(sharedSecret, envelope.Nonce, envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}

// ParseMessage parses a Message protobuf from plaintext bytes
func ParseMessage(data []byte) (*pb.Message, error) {
	var msg pb.Message
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}
	return &msg, nil
}

// generateEphemeralKeyPair generates a new X25519 key pair for ephemeral use
func generateEphemeralKeyPair() (pubKey, privKey []byte, err error) {
	pubKey = make([]byte, crypto.SharedSecretSize)
	privKey = make([]byte, crypto.SharedSecretSize)

	if _, err := rand.Read(privKey); err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	var privArray [32]byte
	copy(privArray[:], privKey)

	result, err := curve25519.X25519(privArray[:], curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute public key: %w", err)
	}

	copy(pubKey, result)
	return pubKey, privKey, nil
}

// CreateOutgoingMessage performs the complete outgoing message flow
// Returns the signed envelope and CID string
func CreateOutgoingMessage(
	text string,
	recipientX25519PubKey []byte,
	senderEd25519PrivKey ed25519.PrivateKey,
) (*pb.SignedEnvelope, error) {
	// Build plaintext message
	msg := BuildMessageNow(text)
	plaintext, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Build encrypted envelope
	envelope, err := BuildEnvelope(plaintext, recipientX25519PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build envelope: %w", err)
	}

	// Sign envelope
	signedEnvelope, err := SignEnvelope(envelope, senderEd25519PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign envelope: %w", err)
	}

	return signedEnvelope, nil
}

// ProcessIncomingMessage performs the complete incoming message flow
// Returns the decrypted message and sender's public key
func ProcessIncomingMessage(
	signedEnvelope *pb.SignedEnvelope,
	recipientX25519PrivKey []byte,
) (*pb.Message, ed25519.PublicKey, error) {
	// Verify signature
	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		return nil, nil, fmt.Errorf("signature verification failed: %w", err)
	}
	if !valid {
		return nil, nil, ErrInvalidSignature
	}

	// Parse envelope
	envelope, err := ParseEnvelope(signedEnvelope)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse envelope: %w", err)
	}

	// Decrypt envelope
	plaintext, err := DecryptEnvelope(envelope, recipientX25519PrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt envelope: %w", err)
	}

	// Parse message
	msg, err := ParseMessage(plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse message: %w", err)
	}

	// Extract sender's public key
	senderPubKey := ed25519.PublicKey(signedEnvelope.SenderPubkey)

	return msg, senderPubKey, nil
}
