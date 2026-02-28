package errors

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBabylonError_Error(t *testing.T) {
	err := New(DomainMessaging, "SEND_FAIL", "failed to send message")
	assert.Equal(t, "[messaging/SEND_FAIL] failed to send message", err.Error())
}

func TestBabylonError_ErrorWithCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := Wrap(DomainNetwork, "CONN_FAIL", "connection failed", cause)
	assert.Contains(t, err.Error(), "[network/CONN_FAIL] connection failed: connection refused")
}

func TestBabylonError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := Wrap(DomainStorage, "DB_ERR", "database error", cause)

	unwrapped := err.Unwrap()
	assert.Equal(t, cause, unwrapped)
}

func TestBabylonError_ErrorsIs(t *testing.T) {
	// Wrap a sentinel in a BabylonError and verify errors.Is still matches.
	err := Wrap(DomainMessaging, "CONTACT", "contact lookup failed", ErrUnknownContact)
	assert.True(t, errors.Is(err, ErrUnknownContact))
}

func TestBabylonError_ErrorsAs(t *testing.T) {
	err := New(DomainConfig, "VALIDATION", "invalid port")
	wrapped := fmt.Errorf("startup failed: %w", err)

	var target *BabylonError
	require.True(t, errors.As(wrapped, &target))
	assert.Equal(t, DomainConfig, target.Domain)
	assert.Equal(t, "VALIDATION", target.Code)
}

func TestBabylonError_NilCause(t *testing.T) {
	err := New(DomainCrypto, "DECRYPT", "decryption failed")
	assert.Nil(t, err.Unwrap())
}

func TestBabylonError_ChainedUnwrap(t *testing.T) {
	root := ErrServiceNotStarted
	mid := Wrap(DomainMessaging, "SERVICE", "service not running", root)
	outer := fmt.Errorf("operation failed: %w", mid)

	assert.True(t, errors.Is(outer, ErrServiceNotStarted))
}

func TestSentinelErrors(t *testing.T) {
	// Verify sentinels are distinct.
	sentinels := []error{
		ErrServiceNotStarted, ErrUnknownContact, ErrSelfMessage,
		ErrPeerNotFound, ErrInvalidConfig, ErrNodeNotReady,
		ErrDecryptionFailed, ErrInvalidEnvelope,
		ErrMailboxFull, ErrRateLimited, ErrQuotaExceeded,
	}

	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j {
				assert.False(t, errors.Is(a, b), "sentinel %d should not match sentinel %d", i, j)
			}
		}
	}
}

func TestSafeGo_NoPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	executed := false
	SafeGo("test-no-panic", func() {
		defer wg.Done()
		executed = true
	})

	wg.Wait()
	assert.True(t, executed)
}

func TestSafeGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	SafeGo("test-panic", func() {
		defer wg.Done()
		panic("test panic")
	})

	// If SafeGo did not recover, the test process would crash.
	wg.Wait()
}

func TestSafeGo_RecoversPanicWithError(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	SafeGo("test-panic-error", func() {
		defer wg.Done()
		panic(errors.New("something broke"))
	})

	wg.Wait()
}

func TestNew(t *testing.T) {
	err := New(DomainRTC, "TIMEOUT", "call timed out")
	assert.Equal(t, DomainRTC, err.Domain)
	assert.Equal(t, "TIMEOUT", err.Code)
	assert.Equal(t, "call timed out", err.Message)
	assert.Nil(t, err.Cause)
}

func TestWrap(t *testing.T) {
	cause := errors.New("io timeout")
	err := Wrap(DomainMailbox, "DEPOSIT", "deposit failed", cause)
	assert.Equal(t, DomainMailbox, err.Domain)
	assert.Equal(t, "DEPOSIT", err.Code)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, strings.Contains(err.Error(), "io timeout"))
}
