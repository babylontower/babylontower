package errors

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

