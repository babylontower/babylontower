// Package errors provides domain error types, sentinel errors, and goroutine
// panic recovery for the Babylon Tower project.
package errors

import (
	"fmt"
)

// Domain represents an error domain within the application.
type Domain string

const (
	DomainMessaging  Domain = "messaging"
	DomainStorage    Domain = "storage"
	DomainNetwork    Domain = "network"
	DomainCrypto     Domain = "crypto"
	DomainConfig     Domain = "config"
	DomainMailbox    Domain = "mailbox"
	DomainIdentity   Domain = "identity"
	DomainPeerstore  Domain = "peerstore"
	DomainProtocol   Domain = "protocol"
	DomainReputation Domain = "reputation"
	DomainRTC        Domain = "rtc"
)

// BabylonError is a structured error type with domain, code, and cause chain.
type BabylonError struct {
	Domain  Domain
	Code    string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *BabylonError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s/%s] %s: %v", e.Domain, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s/%s] %s", e.Domain, e.Code, e.Message)
}

// Unwrap returns the underlying cause for errors.Is / errors.As support.
func (e *BabylonError) Unwrap() error {
	return e.Cause
}

// New creates a new BabylonError without a cause.
func New(domain Domain, code, message string) *BabylonError {
	return &BabylonError{
		Domain:  domain,
		Code:    code,
		Message: message,
	}
}

// Wrap creates a new BabylonError that wraps an existing error.
func Wrap(domain Domain, code, message string, cause error) *BabylonError {
	return &BabylonError{
		Domain:  domain,
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}
