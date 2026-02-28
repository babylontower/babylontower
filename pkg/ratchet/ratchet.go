package ratchet

import (
	"crypto/ed25519"
	"crypto/hmac"
	"fmt"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

// RatchetHeader is included with each encrypted message
type RatchetHeader struct {
	DHRatchetPub     *[32]byte // Current DH ratchet public key
	PreviousChainLen uint32    // Length of previous sending chain
	MessageNumber    uint32    // Index in current sending chain
}

// EncryptedMessage is the output of Encrypt
type EncryptedMessage struct {
	Header     *RatchetHeader
	Ciphertext []byte
	Nonce      []byte
}

// Encrypt encrypts a message using the Double Ratchet
func (s *DoubleRatchetState) Encrypt(plaintext, associatedData []byte) (*EncryptedMessage, error) {
	// Check if we need to perform a DH ratchet step (first message or new ratchet)
	if s.SendingChainKey == nil {
		if err := s.dhRatchetSend(); err != nil {
			return nil, fmt.Errorf("DH ratchet failed: %w", err)
		}
	}

	// Derive message key
	newChainKey, messageKey := KDF_CK(s.SendingChainKey)
	s.SendingChainKey = newChainKey

	// Derive nonce
	nonce, err := DeriveNonce(messageKey, s.SendingChainCount)
	if err != nil {
		zeroBytes(messageKey)
		return nil, err
	}

	// Encrypt using XChaCha20-Poly1305
	aead, err := chacha20poly1305.New(messageKey)
	if err != nil {
		zeroBytes(messageKey)
		zeroBytes(nonce)
		return nil, fmt.Errorf("failed to create AEAD: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, associatedData)

	// Create header
	header := &RatchetHeader{
		DHRatchetPub:     s.DHSendingKeyPub,
		PreviousChainLen: s.PreviousSendingChainLen,
		MessageNumber:    s.SendingChainCount,
	}

	// Increment counter
	s.SendingChainCount++
	s.LastUsedAt = time.Now().Unix()

	// Build result before cleanup
	result := &EncryptedMessage{
		Header:     header,
		Ciphertext: ciphertext,
		Nonce:      make([]byte, len(nonce)),
	}
	copy(result.Nonce, nonce)

	// Clean up
	zeroBytes(messageKey)
	zeroBytes(nonce)

	return result, nil
}

// Decrypt decrypts a message using the Double Ratchet
func (s *DoubleRatchetState) Decrypt(header *RatchetHeader, ciphertext, associatedData []byte) ([]byte, error) {
	// Check if we need to perform a DH ratchet step
	if header.DHRatchetPub != nil && s.DHReceivingKeyPub != nil {
		if !equalKeys(header.DHRatchetPub[:], s.DHReceivingKeyPub[:]) {
			// New ratchet public key - perform DH ratchet step
			if err := s.dhRatchetReceive(header); err != nil {
				return nil, fmt.Errorf("DH ratchet failed: %w", err)
			}
		}
	} else if header.DHRatchetPub != nil && s.DHReceivingKeyPub == nil {
		// First message from sender - set receiving key and derive receiving chain
		s.DHReceivingKeyPub = header.DHRatchetPub

		// Derive receiving chain from DH output
		dhOutput, err := curve25519.X25519(s.DHSendingKeyPriv[:], header.DHRatchetPub[:])
		if err != nil {
			return nil, fmt.Errorf("initial DH ratchet failed: %w", err)
		}
		newRootKey, newChainKey := KDF_RK(s.RootKey, dhOutput)
		zeroBytes(dhOutput)
		s.RootKey = newRootKey
		s.ReceivingChainKey = newChainKey
		s.ReceivingChainCount = 0
	}

	// Check if this is a skipped message
	skippedKey := s.getSkippedKey(header)
	if skippedKey != nil {
		// Decrypt with skipped key
		plaintext, err := decryptWithKey(skippedKey, header.MessageNumber, ciphertext, associatedData)
		if err == nil {
			return plaintext, nil
		}
	}

	// Check if message is out of order
	if header.MessageNumber > s.ReceivingChainCount {
		// Skip messages to catch up
		if err := s.skipMessages(header); err != nil {
			return nil, fmt.Errorf("failed to skip messages: %w", err)
		}
	}

	// Advance chain to current message
	if err := s.advanceReceivingChain(header.MessageNumber); err != nil {
		return nil, fmt.Errorf("failed to advance chain: %w", err)
	}

	// Derive message key
	newChainKey, messageKey := KDF_CK(s.ReceivingChainKey)
	s.ReceivingChainKey = newChainKey
	s.ReceivingChainCount++
	s.LastUsedAt = time.Now().Unix()

	// Decrypt
	plaintext, err := decryptWithKey(messageKey, header.MessageNumber, ciphertext, associatedData)
	zeroBytes(messageKey)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// dhRatchetSend performs a DH ratchet step for sending
func (s *DoubleRatchetState) dhRatchetSend() error {
	if s.DHReceivingKeyPub == nil {
		return fmt.Errorf("no receiving key set")
	}

	// Generate new ratchet key pair
	newPriv, newPub, err := generateX25519KeyPair()
	if err != nil {
		return err
	}

	// Compute DH output
	dhOutput, err := curve25519.X25519(newPriv[:], s.DHReceivingKeyPub[:])
	if err != nil {
		zeroBytes(newPriv[:])
		return err
	}

	// KDF_RK
	newRootKey, newChainKey := KDF_RK(s.RootKey, dhOutput)
	zeroBytes(dhOutput)

	// Update state
	if s.DHSendingKeyPriv != nil {
		zeroBytes(s.DHSendingKeyPriv[:])
	}
	s.PreviousSendingChainLen = s.SendingChainCount
	s.DHSendingKeyPriv = newPriv
	s.DHSendingKeyPub = newPub
	s.RootKey = newRootKey
	s.SendingChainKey = newChainKey
	s.SendingChainCount = 0

	return nil
}

// dhRatchetReceive performs a DH ratchet step for receiving
func (s *DoubleRatchetState) dhRatchetReceive(header *RatchetHeader) error {
	// Cache remaining keys from the current receiving chain before switching
	s.cacheSkippedKeys(header.PreviousChainLen)

	// DH ratchet step
	dhOutput, err := curve25519.X25519(s.DHSendingKeyPriv[:], header.DHRatchetPub[:])
	if err != nil {
		return err
	}

	newRootKey, newChainKey := KDF_RK(s.RootKey, dhOutput)
	zeroBytes(dhOutput)

	s.RootKey = newRootKey
	s.ReceivingChainKey = newChainKey
	s.ReceivingChainCount = 0
	s.DHReceivingKeyPub = header.DHRatchetPub

	// New sending ratchet
	if err := s.dhRatchetSend(); err != nil {
		return err
	}

	return nil
}

// advanceReceivingChain advances the receiving chain to the specified message number
func (s *DoubleRatchetState) advanceReceivingChain(targetNum uint32) error {
	for s.ReceivingChainCount < targetNum {
		newChainKey, skippedKey := KDF_CK(s.ReceivingChainKey)
		s.ReceivingChainKey = newChainKey

		// Cache skipped key
		key := s.skippedKeyString(s.DHReceivingKeyPub, s.ReceivingChainCount)
		if len(s.SkippedKeys) < MaxSkippedKeys {
			s.SkippedKeys[key] = skippedKey
		}

		s.ReceivingChainCount++
	}
	return nil
}

// skipMessages handles out-of-order messages by caching skipped keys
func (s *DoubleRatchetState) skipMessages(header *RatchetHeader) error {
	if s.ReceivingChainKey == nil {
		return fmt.Errorf("receiving chain not initialized")
	}

	currentCount := s.ReceivingChainCount
	currentChainKey := s.ReceivingChainKey

	// Advance to target message number
	for currentCount < header.MessageNumber {
		newChainKey, skippedKey := KDF_CK(currentChainKey)
		currentChainKey = newChainKey

		// Cache skipped key
		key := s.skippedKeyString(s.DHReceivingKeyPub, currentCount)
		if len(s.SkippedKeys) < MaxSkippedKeys {
			s.SkippedKeys[key] = skippedKey
		}

		currentCount++
	}

	return nil
}

// cacheSkippedKeys caches remaining keys in the current receiving chain
// before a DH ratchet switch. prevChainLen is the number of messages the
// sender indicated it sent on the previous chain (from PreviousChainLen header).
func (s *DoubleRatchetState) cacheSkippedKeys(prevChainLen uint32) {
	if s.ReceivingChainKey == nil {
		return
	}
	// Cache keys from current receiving chain position up to prevChainLen
	for s.ReceivingChainCount < prevChainLen {
		newChainKey, skippedKey := KDF_CK(s.ReceivingChainKey)
		s.ReceivingChainKey = newChainKey

		key := s.skippedKeyString(s.DHReceivingKeyPub, s.ReceivingChainCount)
		if len(s.SkippedKeys) < MaxSkippedKeys {
			s.SkippedKeys[key] = skippedKey
		} else {
			zeroBytes(skippedKey)
		}

		s.ReceivingChainCount++
	}
}

// getSkippedKey retrieves a cached skipped key
func (s *DoubleRatchetState) getSkippedKey(header *RatchetHeader) []byte {
	key := s.skippedKeyString(header.DHRatchetPub, header.MessageNumber)
	skippedKey, ok := s.SkippedKeys[key]
	if ok {
		// Remove used key
		delete(s.SkippedKeys, key)
	}
	return skippedKey
}

// skippedKeyString creates a map key for skipped keys
func (s *DoubleRatchetState) skippedKeyString(dhPub *[32]byte, counter uint32) string {
	if dhPub == nil {
		return fmt.Sprintf("nil:%d", counter)
	}
	return fmt.Sprintf("%x:%d", dhPub[:], counter)
}

// decryptWithKey decrypts using a specific message key
func decryptWithKey(messageKey []byte, counter uint32, ciphertext, associatedData []byte) ([]byte, error) {
	nonce, err := DeriveNonce(messageKey, counter)
	if err != nil {
		return nil, err
	}

	aead, err := chacha20poly1305.New(messageKey)
	if err != nil {
		return nil, err
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// equalKeys compares two byte slices for equality
func equalKeys(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return hmac.Equal(a, b)
}

// GetSessionState returns a serializable session state
func (s *DoubleRatchetState) GetSessionState() *SessionState {
	state := &SessionState{
		SessionID:             s.SessionID,
		LocalIdentityPub:      s.LocalIdentityPub,
		RemoteIdentityPub:     s.RemoteIdentityPub,
		RootKey:               s.RootKey,
		SendingChainKey:       s.SendingChainKey,
		SendingChainCounter:   s.SendingChainCount,
		ReceivingChainKey:     s.ReceivingChainKey,
		ReceivingChainCounter: s.ReceivingChainCount,
		CreatedAt:             s.CreatedAt,
		LastUsedAt:            s.LastUsedAt,
		CipherSuiteID:         s.CipherSuiteID,
		IsInitiator:           s.IsInitiator,
	}

	if s.DHSendingKeyPub != nil {
		state.DHSendingKeyPub = s.DHSendingKeyPub[:]
	}
	if s.DHReceivingKeyPub != nil {
		state.DHReceivingPub = s.DHReceivingKeyPub[:]
	}

	// Convert skipped keys
	state.SkippedKeys = make([]SkippedKey, 0, len(s.SkippedKeys))
	for _, key := range s.SkippedKeys {
		state.SkippedKeys = append(state.SkippedKeys, SkippedKey{
			Key: key,
		})
	}

	return state
}

// SessionState is the serializable form of DoubleRatchetState
type SessionState struct {
	SessionID             string
	LocalIdentityPub      ed25519.PublicKey
	RemoteIdentityPub     ed25519.PublicKey
	DHSendingKeyPub       []byte
	DHReceivingPub        []byte
	RootKey               []byte
	SendingChainKey       []byte
	SendingChainCounter   uint32
	ReceivingChainKey     []byte
	ReceivingChainCounter uint32
	SkippedKeys           []SkippedKey
	CreatedAt             int64
	LastUsedAt            int64
	CipherSuiteID         uint32
	IsInitiator           bool
}

// SkippedKey represents a cached skipped message key
type SkippedKey struct {
	DHRatchetPub []byte
	Counter      uint32
	Key          []byte
}
