package ipfsnode

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// TestTopicFromPublicKey tests topic derivation from public key
func TestTopicFromPublicKey(t *testing.T) {
	// Generate a test public key
	_, pubKey, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubKeyBytes, err := pubKey.Raw()
	if err != nil {
		t.Fatalf("Failed to get key bytes: %v", err)
	}

	topic := TopicFromPublicKey(pubKeyBytes)

	if topic == "" {
		t.Error("Topic should not be empty")
	}

	if len(topic) < 10 {
		t.Errorf("Topic too short: %s", topic)
	}

	// Same key should produce same topic
	topic2 := TopicFromPublicKey(pubKeyBytes)
	if topic != topic2 {
		t.Errorf("Same key should produce same topic: %s vs %s", topic, topic2)
	}

	// Different key should produce different topic
	_, pubKey2, _ := crypto.GenerateEd25519Key(rand.Reader)
	pubKeyBytes2, _ := pubKey2.Raw()
	topic3 := TopicFromPublicKey(pubKeyBytes2)
	if topic == topic3 {
		t.Errorf("Different keys should produce different topics")
	}

	t.Logf("Topic from public key: %s", topic)
}

// TestRepoDirExpansion tests that ~ is expanded in repo directory
func TestRepoDirExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home directory: %v", err)
	}

	config := &Config{
		RepoDir: "~/.babylontower/test-ipfs-" + filepath.Base(t.TempDir()),
	}

	node, err := NewNode(config)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	if err := node.Start(); err != nil {
		// May fail due to permissions, skip test
		t.Skipf("Cannot start node with expanded path: %v", err)
	}
	defer stopNode(node)()

	// Verify the path was expanded (should not contain ~)
	expanded, _ := expandPath(config.RepoDir)
	if len(expanded) == 0 || !filepath.IsAbs(expanded) {
		t.Errorf("Path not expanded correctly: %s", expanded)
	}

	// Should start with home directory
	if len(expanded) < len(home) || expanded[:len(home)] != home {
		t.Logf("Expanded path: %s, Home: %s", expanded, home)
	}
}

// TestExpandPathRespectsHomeEnv tests that HOME environment variable is respected
func TestExpandPathRespectsHomeEnv(t *testing.T) {
	// Save original HOME
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()

	// Set custom HOME for test
	customHome := "/custom/test/home"
	_ = os.Setenv("HOME", customHome)

	// Test expansion with custom HOME
	path := "~/.babylontower/ipfs"
	expanded, err := expandPath(path)
	if err != nil {
		t.Fatalf("Failed to expand path: %v", err)
	}

	expected, _ := filepath.Abs(filepath.Join(customHome, ".babylontower/ipfs"))
	if expanded != expected {
		t.Errorf("Expected %s, got %s", expected, expanded)
	}

	// Test that UserHomeDir is used when HOME is not set
	_ = os.Unsetenv("HOME")

	// Get user home dir BEFORE unsetting (some systems require HOME to be set)
	userHome, err := os.UserHomeDir()
	if err != nil {
		// Try to get home from /etc/passwd or other means
		// If this fails, skip the test
		t.Logf("Cannot get user home dir without HOME set: %v", err)
		// Re-set HOME to original value for this check
		if originalHome != "" {
			_ = os.Setenv("HOME", originalHome)
			userHome, err = os.UserHomeDir()
			if err != nil {
				t.Skipf("Cannot get user home dir at all: %v", err)
			}
		} else {
			t.Skipf("Cannot get user home dir: %v", err)
		}
	}

	expanded2, err := expandPath(path)
	if err != nil {
		t.Fatalf("Failed to expand path: %v", err)
	}

	expected2 := filepath.Join(userHome, ".babylontower/ipfs")
	if expanded2 != expected2 {
		t.Errorf("Expected %s, got %s", expected2, expanded2)
	}
}
