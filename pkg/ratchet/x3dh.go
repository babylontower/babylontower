// Package ratchet implements the X3DH key agreement and Double Ratchet protocol
// for forward-secret and post-compromise secure messaging.
package ratchet

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// Cipher suite constants
const (
	// CipherSuiteXChaCha20Poly1305 is the mandatory cipher suite for Babylon Tower v1
	CipherSuiteXChaCha20Poly1305 uint32 = 0x0001

	// CipherSuiteAES256GCM is an optional cipher suite (not yet implemented)
	CipherSuiteAES256GCM uint32 = 0x0002

	// Key sizes
	KeySize32 = 32
	KeySize64 = 64

	// Ratchet constants
	MaxSkippedKeys = 256

	// SPKMaxAge is the maximum age for a signed prekey (21 days per §2.2)
	SPKMaxAge = 21 * 24 * time.Hour
)

// SupportedCipherSuites lists cipher suites in preference order (highest first).
// Per §2.1: Initiator selects highest mutually supported suite.
var SupportedCipherSuites = []uint32{
	CipherSuiteXChaCha20Poly1305, // 0x0001 — mandatory
}

// NegotiateCipherSuite selects the highest mutually supported cipher suite.
// Per §2.1: Initiator selects highest mutually supported suite.
// Returns 0 if no common suite exists.
func NegotiateCipherSuite(localSuites, remoteSuites []uint32) uint32 {
	remoteSet := make(map[uint32]bool, len(remoteSuites))
	for _, s := range remoteSuites {
		remoteSet[s] = true
	}
	// localSuites is ordered by preference (highest first)
	for _, s := range localSuites {
		if remoteSet[s] {
			return s
		}
	}
	return 0
}

// ValidateSPKAge checks whether a signed prekey is within the acceptable age.
// Per §2.2: Reject SPK with created_at older than 21 days.
func ValidateSPKAge(spkCreatedAt uint64) error {
	created := time.Unix(int64(spkCreatedAt), 0)
	age := time.Since(created)
	if age > SPKMaxAge {
		return fmt.Errorf("signed prekey expired: age %s exceeds maximum %s", age.Round(time.Hour), SPKMaxAge)
	}
	return nil
}

// X3DHResult holds the result of an X3DH key exchange
type X3DHResult struct {
	// SharedSecret is the derived shared secret from X3DH
	SharedSecret []byte
	// AuthenticatedData is the associated data for encryption
	AuthenticatedData []byte
	// UsedOPKID is the ID of the consumed one-time prekey (0 if not used)
	UsedOPKID uint64
	// CipherSuite is the negotiated cipher suite
	CipherSuite uint32
	// EphemeralPub is the ephemeral public key sent by initiator (32 bytes)
	EphemeralPub *[32]byte
	// RemoteSPKPub is the remote party's signed prekey public key (for Double Ratchet init)
	RemoteSPKPub *[32]byte
	// UsedOPKPub is the one-time prekey public key that was used (nil if not used)
	UsedOPKPub *[32]byte
}

// X3DHInitiator performs the X3DH key exchange as the initiator (Alice)
// Returns the shared secret and authenticated data
// Parameters:
//   - localIKDHPriv/ localIKDHPub: Local identity DH key pair
//   - localIKSignPub: Local identity signing public key (for AD construction)
//   - remoteIKDHPub: Remote identity DH public key
//   - remoteIKSignPub: Remote identity signing public key (for AD construction)
//   - remoteSPKPub: Remote signed prekey public key
//   - remoteOPKPub: Remote one-time prekey public key (may be nil)
func X3DHInitiator(
	localIKDHPriv *[32]byte,
	localIKDHPub *[32]byte,
	localIKSignPub ed25519.PublicKey,
	remoteIKDHPub *[32]byte,
	remoteIKSignPub ed25519.PublicKey,
	remoteSPKPub *[32]byte,
	remoteOPKPub *[32]byte, // May be nil if no OPK available
) (*X3DHResult, error) {
	// Step 1: Generate ephemeral key pair
	ekPriv, ekPub, err := generateX25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Step 2: Compute DH values
	dh1, err := curve25519.X25519(localIKDHPriv[:], remoteSPKPub[:])
	if err != nil {
		return nil, fmt.Errorf("DH1 computation failed: %w", err)
	}

	dh2, err := curve25519.X25519(ekPriv[:], remoteIKDHPub[:])
	if err != nil {
		return nil, fmt.Errorf("DH2 computation failed: %w", err)
	}

	dh3, err := curve25519.X25519(ekPriv[:], remoteSPKPub[:])
	if err != nil {
		return nil, fmt.Errorf("DH3 computation failed: %w", err)
	}

	// Step 3: Compute DH4 if OPK is available
	var dh4 []byte
	var usedOPKID uint64
	if remoteOPKPub != nil {
		dh4, err = curve25519.X25519(ekPriv[:], remoteOPKPub[:])
		if err != nil {
			return nil, fmt.Errorf("DH4 computation failed: %w", err)
		}
	}

	// Step 4: Derive shared secret
	var input []byte
	if remoteOPKPub != nil {
		// 4-DH: DH1 || DH2 || DH3 || DH4 (128 bytes)
		input = append(append(append(dh1, dh2...), dh3...), dh4...)
	} else {
		// 3-DH: DH1 || DH2 || DH3 (96 bytes)
		input = append(append(dh1, dh2...), dh3...)
	}

	// HKDF-SHA256
	info := append([]byte("BabylonTowerX3DH"), localIKDHPub[:]...)
	info = append(info, remoteIKDHPub[:]...)

	sk := make([]byte, KeySize32)
	hkdfReader := hkdf.New(sha256.New, input, make([]byte, KeySize32), info)
	if _, err := io.ReadFull(hkdfReader, sk); err != nil {
		return nil, fmt.Errorf("failed to derive shared secret: %w", err)
	}

	// Step 5: Associated data (per spec: AD = sender.IK_sign.pub ‖ recipient.IK_sign.pub)
	ad := append(localIKSignPub[:], remoteIKSignPub[:]...)

	// Step 6: Clean up sensitive data
	zeroBytes(ekPriv[:])
	zeroBytes(dh1)
	zeroBytes(dh2)
	zeroBytes(dh3)
	if dh4 != nil {
		zeroBytes(dh4)
	}

	return &X3DHResult{
		SharedSecret:      sk,
		AuthenticatedData: ad,
		UsedOPKID:         usedOPKID,
		CipherSuite:       CipherSuiteXChaCha20Poly1305,
		EphemeralPub:      ekPub,
		RemoteSPKPub:      remoteSPKPub,
		UsedOPKPub:        func() *[32]byte { if remoteOPKPub != nil { var result [32]byte; copy(result[:], remoteOPKPub[:]); return &result }; return nil }(),
	}, nil
}

// X3DHResponder performs the X3DH key exchange as the responder (Bob)
// Parameters:
//   - localIKDHPriv/ localIKDHPub: Local identity DH key pair
//   - localIKSignPub: Local identity signing public key (for AD construction)
//   - localSPKPriv: Local signed prekey private key
//   - localOPKPriv: Local one-time prekey private key (may be nil)
//   - remoteIKDHPub: Remote identity DH public key
//   - remoteIKSignPub: Remote identity signing public key (for AD construction)
//   - ekPub: Remote ephemeral public key
func X3DHResponder(
	localIKDHPriv *[32]byte,
	localIKDHPub *[32]byte,
	localIKSignPub ed25519.PublicKey,
	localSPKPriv *[32]byte,
	localOPKPriv *[32]byte, // May be nil if OPK was not used
	remoteIKDHPub *[32]byte,
	remoteIKSignPub ed25519.PublicKey,
	ekPub *[32]byte,
) (*X3DHResult, error) {
	// Step 1: Compute DH values (mirror of initiator)
	dh1, err := curve25519.X25519(localSPKPriv[:], remoteIKDHPub[:])
	if err != nil {
		return nil, fmt.Errorf("DH1 computation failed: %w", err)
	}

	dh2, err := curve25519.X25519(localIKDHPriv[:], ekPub[:])
	if err != nil {
		return nil, fmt.Errorf("DH2 computation failed: %w", err)
	}

	dh3, err := curve25519.X25519(localSPKPriv[:], ekPub[:])
	if err != nil {
		return nil, fmt.Errorf("DH3 computation failed: %w", err)
	}

	// Step 2: Compute DH4 if OPK was used
	var dh4 []byte
	if localOPKPriv != nil {
		dh4, err = curve25519.X25519(localOPKPriv[:], ekPub[:])
		if err != nil {
			return nil, fmt.Errorf("DH4 computation failed: %w", err)
		}
	}

	// Step 3: Derive shared secret
	var input []byte
	if localOPKPriv != nil {
		input = append(append(append(dh1, dh2...), dh3...), dh4...)
	} else {
		input = append(append(dh1, dh2...), dh3...)
	}

	info := append([]byte("BabylonTowerX3DH"), remoteIKDHPub[:]...)
	info = append(info, localIKDHPub[:]...)

	sk := make([]byte, KeySize32)
	hkdfReader := hkdf.New(sha256.New, input, make([]byte, KeySize32), info)
	if _, err := io.ReadFull(hkdfReader, sk); err != nil {
		return nil, fmt.Errorf("failed to derive shared secret: %w", err)
	}

	// Step 4: Associated data (per spec: AD = sender.IK_sign.pub ‖ recipient.IK_sign.pub)
	// For responder, sender is the remote party (initiator)
	ad := append(remoteIKSignPub[:], localIKSignPub[:]...)

	// Step 5: Clean up
	zeroBytes(dh1)
	zeroBytes(dh2)
	zeroBytes(dh3)
	if dh4 != nil {
		zeroBytes(dh4)
	}

	return &X3DHResult{
		SharedSecret:      sk,
		AuthenticatedData: ad,
		UsedOPKID:         0, // Responder doesn't track OPK ID here
		CipherSuite:       CipherSuiteXChaCha20Poly1305,
	}, nil
}

// DoubleRatchetState holds the state of a Double Ratchet session
type DoubleRatchetState struct {
	// Session identification
	SessionID string

	// Identity keys
	LocalIdentityPub  ed25519.PublicKey
	RemoteIdentityPub ed25519.PublicKey

	// DH ratchet keys
	DHSendingKeyPriv  *[32]byte
	DHSendingKeyPub   *[32]byte
	DHReceivingKeyPub *[32]byte

	// Root and chain keys
	RootKey                 []byte
	SendingChainKey         []byte
	SendingChainCount       uint32
	PreviousSendingChainLen uint32 // Length of previous sending chain (for header)

	ReceivingChainKey   []byte
	ReceivingChainCount uint32

	// Skipped keys for out-of-order delivery
	SkippedKeys map[string][]byte // key: fmt.Sprintf("%x:%d", dhRatchetPub, counter)

	// Metadata
	CreatedAt     int64
	LastUsedAt    int64
	CipherSuiteID uint32

	// Role: true if we initiated the X3DH
	IsInitiator bool
}

// NewDoubleRatchetStateInitiator creates a new Double Ratchet state for the X3DH initiator
func NewDoubleRatchetStateInitiator(
	sessionID string,
	localIdentityPub ed25519.PublicKey,
	remoteIdentityPub ed25519.PublicKey,
	sharedSecret []byte,
	remoteSPKPub *[32]byte,
) (*DoubleRatchetState, error) {
	// Generate initial ratchet key pair
	dhSendingPriv, dhSendingPub, err := generateX25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ratchet key pair: %w", err)
	}

	// Perform initial DH ratchet step
	dhOutput, err := curve25519.X25519(dhSendingPriv[:], remoteSPKPub[:])
	if err != nil {
		return nil, fmt.Errorf("initial DH ratchet failed: %w", err)
	}

	// KDF_RK to get root key and sending chain key
	newRootKey, newChainKey := KDF_RK(sharedSecret, dhOutput)
	zeroBytes(dhOutput)

	return &DoubleRatchetState{
		SessionID:           sessionID,
		LocalIdentityPub:    localIdentityPub,
		RemoteIdentityPub:   remoteIdentityPub,
		DHSendingKeyPriv:    dhSendingPriv,
		DHSendingKeyPub:     dhSendingPub,
		DHReceivingKeyPub:   remoteSPKPub, // Initially set to recipient's SPK
		RootKey:             newRootKey,
		SendingChainKey:     newChainKey,
		SendingChainCount:   0,
		ReceivingChainKey:   nil, // Not set until first message received
		ReceivingChainCount: 0,
		SkippedKeys:         make(map[string][]byte),
		CreatedAt:           time.Now().Unix(),
		LastUsedAt:          time.Now().Unix(),
		CipherSuiteID:       CipherSuiteXChaCha20Poly1305,
		IsInitiator:         true,
	}, nil
}

// NewDoubleRatchetStateResponder creates a new Double Ratchet state for the X3DH responder
func NewDoubleRatchetStateResponder(
	sessionID string,
	localIdentityPub ed25519.PublicKey,
	remoteIdentityPub ed25519.PublicKey,
	sharedSecret []byte,
	localSPKPriv *[32]byte,
	localSPKPub *[32]byte,
) (*DoubleRatchetState, error) {
	return &DoubleRatchetState{
		SessionID:           sessionID,
		LocalIdentityPub:    localIdentityPub,
		RemoteIdentityPub:   remoteIdentityPub,
		DHSendingKeyPriv:    localSPKPriv, // Reuse SPK for first ratchet
		DHSendingKeyPub:     localSPKPub,
		DHReceivingKeyPub:   nil, // Not set until first message received
		RootKey:             sharedSecret,
		SendingChainKey:     nil, // Not set until first send
		SendingChainCount:   0,
		ReceivingChainKey:   nil,
		ReceivingChainCount: 0,
		SkippedKeys:         make(map[string][]byte),
		CreatedAt:           time.Now().Unix(),
		LastUsedAt:          time.Now().Unix(),
		CipherSuiteID:       CipherSuiteXChaCha20Poly1305,
		IsInitiator:         false,
	}, nil
}

// KDF_RK performs the root key KDF
// Returns (new_root_key, new_chain_key)
func KDF_RK(rootKey, dhOutput []byte) ([]byte, []byte) {
	// HKDF-SHA256 with root key as salt
	hkdfReader := hkdf.New(sha256.New, dhOutput, rootKey, []byte("BabylonTowerRatchet"))
	output := make([]byte, 64)
	if _, err := io.ReadFull(hkdfReader, output); err != nil {
		// This should never fail as hkdfReader is an infinite reader
		return make([]byte, 32), make([]byte, 32)
	}

	newRootKey := output[:32]
	newChainKey := output[32:64]

	return newRootKey, newChainKey
}

// KDF_CK performs the chain key KDF
// Returns (new_chain_key, message_key)
func KDF_CK(chainKey []byte) ([]byte, []byte) {
	// HMAC-SHA256
	newChainKey := hmacSHA256(chainKey, []byte{0x01})
	messageKey := hmacSHA256(chainKey, []byte{0x02})

	return newChainKey, messageKey
}

// DeriveNonce derives a nonce from the message key and counter
func DeriveNonce(messageKey []byte, counter uint32) ([]byte, error) {
	counterBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(counterBytes, counter)

	hkdfReader := hkdf.New(sha256.New, messageKey, []byte("nonce"), counterBytes)
	nonce := make([]byte, 24) // XChaCha20-Poly1305 nonce size
	if _, err := io.ReadFull(hkdfReader, nonce); err != nil {
		return nil, fmt.Errorf("failed to derive nonce: %w", err)
	}

	return nonce, nil
}

// generateX25519KeyPair generates a new X25519 key pair
func generateX25519KeyPair() (*[32]byte, *[32]byte, error) {
	priv := new([32]byte)
	pub := new([32]byte)

	if _, err := io.ReadFull(rand.Reader, priv[:]); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random seed: %w", err)
	}

	result, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("X25519 derivation failed: %w", err)
	}
	copy(pub[:], result)

	return priv, pub, nil
}

// hmacSHA256 computes HMAC-SHA256
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// zeroBytes zeros out a byte slice
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
