package errors

import "errors"

// Messaging sentinels
var (
	// ErrServiceNotStarted is returned when operations are attempted on a stopped service.
	ErrServiceNotStarted = errors.New("messaging service not started")
	// ErrUnknownContact is returned when trying to message an unknown contact.
	ErrUnknownContact = errors.New("unknown contact")
	// ErrSelfMessage is returned when trying to send a message to oneself.
	ErrSelfMessage = errors.New("cannot send message to self")
)

// Peerstore sentinels
var (
	// ErrPeerNotFound is returned when a peer is not in the address book.
	ErrPeerNotFound = errors.New("peer not found in address book")
)

// Config sentinels
var (
	// ErrInvalidConfig is returned when configuration validation fails.
	ErrInvalidConfig = errors.New("invalid configuration")
)

// Node sentinels
var (
	// ErrNodeNotReady is returned when the IPFS node is not started.
	ErrNodeNotReady = errors.New("node not ready")
)

// Crypto sentinels
var (
	// ErrDecryptionFailed is returned when message decryption fails.
	ErrDecryptionFailed = errors.New("decryption failed")
	// ErrInvalidEnvelope is returned when an envelope is malformed or has an invalid signature.
	ErrInvalidEnvelope = errors.New("invalid envelope")
)

// Mailbox sentinels
var (
	// ErrMailboxFull is returned when a mailbox quota is exceeded.
	ErrMailboxFull = errors.New("mailbox full")
	// ErrRateLimited is returned when a rate limit is hit.
	ErrRateLimited = errors.New("rate limited")
	// ErrQuotaExceeded is returned when a storage quota is exceeded.
	ErrQuotaExceeded = errors.New("quota exceeded")
)
