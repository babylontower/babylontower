package messaging

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"babylontower/pkg/crypto"
	bterrors "babylontower/pkg/errors"
	pb "babylontower/pkg/proto"

	"golang.org/x/crypto/curve25519"
	"google.golang.org/protobuf/proto"
)

var (
	// Aliases to canonical sentinels in pkg/errors for cross-package errors.Is() compatibility.
	ErrInvalidSignature = bterrors.ErrInvalidSignature
	ErrInvalidEnvelope  = bterrors.ErrInvalidEnvelope
	ErrDecryptionFailed = bterrors.ErrDecryptionFailed
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
// Returns the envelope, ephemeral private key (for local storage), and error
func BuildEnvelope(plaintext []byte, recipientX25519PubKey []byte) (*pb.Envelope, []byte, error) {
	if len(recipientX25519PubKey) != crypto.SharedSecretSize {
		return nil, nil, fmt.Errorf("invalid recipient public key length: %d", len(recipientX25519PubKey))
	}

	logger.Debugw("building envelope",
		"recipient_pub", hex.EncodeToString(recipientX25519PubKey[:8]),
		"plaintext_len", len(plaintext))

	ephemeralPub, ephemeralPriv, err := generateEphemeralKeyPair()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	sharedSecret, err := crypto.ComputeSharedSecret(ephemeralPriv, recipientX25519PubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	logger.Debugw("computed shared secret", "shared_secret_computed", true)

	nonce, ciphertext, err := crypto.EncryptWithSharedSecret(sharedSecret, plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encrypt: %w", err)
	}

	envelope := &pb.Envelope{
		Ciphertext:      ciphertext,
		EphemeralPubkey: ephemeralPub,
		Nonce:           nonce,
	}

	logger.Debugw("envelope built",
		"ephemeral_pub", hex.EncodeToString(ephemeralPub[:8]),
		"nonce_len", len(nonce),
		"ciphertext_len", len(ciphertext))

	return envelope, ephemeralPriv, nil
}

// SignEnvelope signs an envelope with the sender's Ed25519 private key
func SignEnvelope(envelope *pb.Envelope, senderPrivKey ed25519.PrivateKey) (*pb.SignedEnvelope, error) {
	envelopeBytes, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal envelope: %w", err)
	}

	signature := ed25519.Sign(senderPrivKey, envelopeBytes)
	senderPubKey := senderPrivKey.Public().(ed25519.PublicKey)

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
func VerifyEnvelope(signedEnvelope *pb.SignedEnvelope) (bool, error) {
	if signedEnvelope == nil {
		return false, errors.New("nil envelope")
	}

	if len(signedEnvelope.SenderPubkey) != ed25519.PublicKeySize {
		return false, errors.New("invalid sender public key length")
	}
	senderPubKey := ed25519.PublicKey(signedEnvelope.SenderPubkey)

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
func DecryptEnvelope(envelope *pb.Envelope, recipientX25519PrivKey []byte) ([]byte, error) {
	if envelope == nil {
		return nil, ErrInvalidEnvelope
	}
	if len(recipientX25519PrivKey) != crypto.SharedSecretSize {
		return nil, fmt.Errorf("invalid recipient private key length: %d", len(recipientX25519PrivKey))
	}

	logger.Debugw("decrypting envelope",
		"ephemeral_pub", hex.EncodeToString(envelope.EphemeralPubkey[:8]),
		"nonce_len", len(envelope.Nonce),
		"ciphertext_len", len(envelope.Ciphertext))

	sharedSecret, err := crypto.ComputeSharedSecret(recipientX25519PrivKey, envelope.EphemeralPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	logger.Debugw("computed shared secret", "shared_secret_computed", true)

	plaintext, err := crypto.DecryptWithSharedSecret(sharedSecret, envelope.Nonce, envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	logger.Debugw("decryption successful", "plaintext_len", len(plaintext))

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
func CreateOutgoingMessage(
	text string,
	recipientX25519PubKey []byte,
	senderEd25519PrivKey ed25519.PrivateKey,
) (*pb.SignedEnvelope, error) {
	msg := BuildMessageNow(text)
	plaintext, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	envelope, _, err := BuildEnvelope(plaintext, recipientX25519PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build envelope: %w", err)
	}

	return SignEnvelope(envelope, senderEd25519PrivKey)
}

// BuildMessageForTesting builds a message without sending.
// Useful for testing the encryption/decryption flow.
func BuildMessageForTesting(
	text string,
	recipientX25519PubKey []byte,
	senderPrivKey ed25519.PrivateKey,
) (*pb.SignedEnvelope, *pb.Message, error) {
	msg := BuildMessageNow(text)
	plaintext, err := proto.Marshal(msg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	envelope, _, err := BuildEnvelope(plaintext, recipientX25519PubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build envelope: %w", err)
	}

	signedEnvelope, err := SignEnvelope(envelope, senderPrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign envelope: %w", err)
	}

	return signedEnvelope, msg, nil
}

// ProcessIncomingMessage performs the complete incoming message flow
func ProcessIncomingMessage(
	signedEnvelope *pb.SignedEnvelope,
	recipientX25519PrivKey []byte,
) (*pb.Message, ed25519.PublicKey, error) {
	valid, err := VerifyEnvelope(signedEnvelope)
	if err != nil {
		return nil, nil, fmt.Errorf("signature verification failed: %w", err)
	}
	if !valid {
		return nil, nil, ErrInvalidSignature
	}

	envelope, err := ParseEnvelope(signedEnvelope)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse envelope: %w", err)
	}

	plaintext, err := DecryptEnvelope(envelope, recipientX25519PrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt envelope: %w", err)
	}

	msg, err := ParseMessage(plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse message: %w", err)
	}

	senderPubKey := ed25519.PublicKey(signedEnvelope.SenderPubkey)
	return msg, senderPubKey, nil
}

