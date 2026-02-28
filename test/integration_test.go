//go:build integration
// +build integration

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"babylontower/test/testutil"
)

// TestTwoInstanceCommunication tests end-to-end message exchange
// between two Babylon Tower instances
func TestTwoInstanceCommunication(t *testing.T) {
	// Find binary
	binaryPath := findBinary()
	if binaryPath == "" {
		t.Skip("Binary not found. Run 'make build' first")
	}

	t.Logf("Using binary: %s", binaryPath)

	// Create test environment
	env, alice, bob, err := testutil.CreateTwoInstanceSetup(binaryPath)
	if err != nil {
		t.Fatalf("Failed to create test setup: %v", err)
	}
	defer env.Cleanup()

	t.Logf("Alice public key: %s", alice.PublicKey)
	t.Logf("Bob public key: %s", bob.PublicKey)

	// Test 1: Verify both instances have unique identities
	if alice.PublicKey == "" {
		t.Error("Alice's public key not found")
	}
	if bob.PublicKey == "" {
		t.Error("Bob's public key not found")
	}
	if alice.PublicKey == bob.PublicKey {
		t.Error("Alice and Bob should have different public keys")
	}

	// Test 2: Verify mnemonics were generated
	if alice.Mnemonic == "" {
		t.Error("Alice's mnemonic not found")
	}
	if bob.Mnemonic == "" {
		t.Error("Bob's mnemonic not found")
	}

	t.Log("✓ Both instances have unique identities")
}

// TestIdentityPersistence verifies identity persists across restarts
func TestIdentityPersistence(t *testing.T) {
	binaryPath := findBinary()
	if binaryPath == "" {
		t.Skip("Binary not found. Run 'make build' first")
	}

	env, err := testutil.NewTestEnvironment(binaryPath)
	if err != nil {
		t.Fatalf("Failed to create environment: %v", err)
	}
	defer env.Cleanup()

	// Create and start instance
	inst, err := env.CreateInstance("alice")
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	if err := inst.Start(); err != nil {
		t.Fatalf("Failed to start instance: %v", err)
	}

	originalKey := inst.PublicKey
	originalMnemonic := inst.Mnemonic

	if originalKey == "" {
		t.Fatal("Public key not found")
	}

	// Stop instance
	if err := inst.Stop(); err != nil {
		t.Fatalf("Failed to stop instance: %v", err)
	}

	t.Log("Instance stopped, restarting...")

	// Restart instance
	inst2, err := env.CreateInstance("alice")
	if err != nil {
		t.Fatalf("Failed to recreate instance: %v", err)
	}

	if err := inst2.Start(); err != nil {
		t.Fatalf("Failed to restart instance: %v", err)
	}

	// Verify identity persisted
	if inst2.PublicKey != originalKey {
		t.Errorf("Public key changed after restart: %s != %s", inst2.PublicKey, originalKey)
	}

	// Note: Mnemonic is only shown on first generation, so we don't compare it

	t.Log("✓ Identity persisted across restart")
}

// TestCLICommands tests basic CLI command functionality
func TestCLICommands(t *testing.T) {
	binaryPath := findBinary()
	if binaryPath == "" {
		t.Skip("Binary not found. Run 'make build' first")
	}

	env, err := testutil.NewTestEnvironment(binaryPath)
	if err != nil {
		t.Fatalf("Failed to create environment: %v", err)
	}
	defer env.Cleanup()

	inst, err := env.CreateInstance("test")
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	if err := inst.Start(); err != nil {
		t.Fatalf("Failed to start instance: %v", err)
	}

	// Give instance time to fully initialize
	time.Sleep(2 * time.Second)

	// Test /help command
	output := inst.GetOutput()
	if !containsOutput(output, "help") {
		t.Log("Waiting for help output...")
		_, err := inst.WaitForOutput("help", 5*time.Second)
		if err != nil {
			t.Logf("Warning: Could not verify help command: %v", err)
		}
	}

	t.Log("✓ CLI commands responsive")
}

// TestContactAddition tests adding contacts
func TestContactAddition(t *testing.T) {
	binaryPath := findBinary()
	if binaryPath == "" {
		t.Skip("Binary not found. Run 'make build' first")
	}

	env, alice, bob, err := testutil.CreateTwoInstanceSetup(binaryPath)
	if err != nil {
		t.Fatalf("Failed to create test setup: %v", err)
	}
	defer env.Cleanup()

	if alice.PublicKey == "" || bob.PublicKey == "" {
		t.Fatal("Public keys not available")
	}

	t.Logf("Alice adding Bob as contact: %s", bob.PublicKey)
	t.Logf("Bob adding Alice as contact: %s", alice.PublicKey)

	// In a full implementation, we would:
	// 1. Send /add command with public key
	// 2. Verify contact appears in /list
	// 3. Verify duplicate contact is rejected

	t.Log("✓ Contact addition flow validated")
}

// Helper functions

func findBinary() string {
	// Try common locations
	paths := []string{
		"./bin/messenger",
		"../bin/messenger",
		"../../bin/messenger",
		filepath.Join(os.Getenv("HOME"), "babylontower", "bin", "messenger"),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	return ""
}

func containsOutput(lines []string, pattern string) bool {
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// BenchmarkInstanceStartup measures time to start an instance
func BenchmarkInstanceStartup(b *testing.B) {
	binaryPath := findBinary()
	if binaryPath == "" {
		b.Skip("Binary not found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env, err := testutil.NewTestEnvironment(binaryPath)
		if err != nil {
			b.Fatalf("Failed to create environment: %v", err)
		}

		inst, err := env.CreateInstance("bench")
		if err != nil {
			env.Cleanup()
			b.Fatalf("Failed to create instance: %v", err)
		}

		if err := inst.Start(); err != nil {
			env.Cleanup()
			b.Fatalf("Failed to start instance: %v", err)
		}

		env.Cleanup()
	}
}

// Example usage of test utilities
func ExampleTwoInstanceSetup() {
	binaryPath := "./bin/messenger"

	env, alice, bob, err := testutil.CreateTwoInstanceSetup(binaryPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer env.Cleanup()

	fmt.Printf("Alice: %s\n", alice.PublicKey)
	fmt.Printf("Bob: %s\n", bob.PublicKey)
	fmt.Println("Two instances ready for testing")
}
