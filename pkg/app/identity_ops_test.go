package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateNewIdentity(t *testing.T) {
	result, err := GenerateNewIdentity()
	require.NoError(t, err)

	assert.NotEmpty(t, result.Mnemonic)
	assert.NotEmpty(t, result.PublicKeyBase58)
	assert.NotEmpty(t, result.X25519KeyBase58)
	assert.NotEmpty(t, result.Fingerprint)
	assert.NotNil(t, result.Identity)

	// Mnemonic should be 12 words
	words := splitWords(result.Mnemonic)
	assert.Len(t, words, 12)
}

func TestRestoreIdentityFromMnemonic(t *testing.T) {
	// Generate first
	original, err := GenerateNewIdentity()
	require.NoError(t, err)

	// Restore from mnemonic
	restored, err := RestoreIdentityFromMnemonic(original.Mnemonic)
	require.NoError(t, err)

	// Keys must be identical (deterministic derivation)
	assert.Equal(t, original.PublicKeyBase58, restored.PublicKeyBase58)
	assert.Equal(t, original.X25519KeyBase58, restored.X25519KeyBase58)
	assert.Equal(t, original.Fingerprint, restored.Fingerprint)
}

func TestRestoreIdentityFromMnemonic_Invalid(t *testing.T) {
	_, err := RestoreIdentityFromMnemonic("not a valid mnemonic")
	assert.Error(t, err)
}

func TestValidateMnemonic(t *testing.T) {
	result, err := GenerateNewIdentity()
	require.NoError(t, err)

	assert.True(t, ValidateMnemonic(result.Mnemonic))
	assert.False(t, ValidateMnemonic("invalid mnemonic words"))
	assert.False(t, ValidateMnemonic(""))
	// Whitespace should be trimmed
	assert.True(t, ValidateMnemonic("  "+result.Mnemonic+"  "))
}

func TestSaveAndLoadIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "identity.json")

	original, err := GenerateNewIdentity()
	require.NoError(t, err)

	err = SaveIdentityToFile(original.Identity, path)
	require.NoError(t, err)

	assert.True(t, IdentityFileExists(path))

	loaded, err := LoadIdentityFromFile(path)
	require.NoError(t, err)

	assert.Equal(t, original.PublicKeyBase58, loaded.PublicKeyBase58)
	assert.Equal(t, original.X25519KeyBase58, loaded.X25519KeyBase58)
	assert.Equal(t, original.Fingerprint, loaded.Fingerprint)
}

func TestIdentityFileExists_NotFound(t *testing.T) {
	assert.False(t, IdentityFileExists("/nonexistent/path/identity.json"))
}

func TestSaveIdentity_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "identity.json")

	result, err := GenerateNewIdentity()
	require.NoError(t, err)

	err = SaveIdentityToFile(result.Identity, path)
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestGenerateContactLink(t *testing.T) {
	result, err := GenerateNewIdentity()
	require.NoError(t, err)

	link := GenerateContactLink(result.Identity.Ed25519PubKey, result.Identity.X25519PubKey, "Alice")
	assert.Contains(t, link, "btower://")
	assert.Contains(t, link, result.PublicKeyBase58)
	assert.Contains(t, link, "name=Alice")
	assert.Contains(t, link, "x25519=")

	// Without name — should still have x25519 key
	link2 := GenerateContactLink(result.Identity.Ed25519PubKey, result.Identity.X25519PubKey, "")
	assert.Contains(t, link2, "btower://")
	assert.Contains(t, link2, "x25519=")
	assert.NotContains(t, link2, "name=")
}

func TestParseContactLink(t *testing.T) {
	result, err := GenerateNewIdentity()
	require.NoError(t, err)

	link := GenerateContactLink(result.Identity.Ed25519PubKey, result.Identity.X25519PubKey, "Alice")
	parsed, err := ParseContactLink(link)
	require.NoError(t, err)

	assert.Equal(t, result.PublicKeyBase58, parsed.PublicKeyBase58)
	assert.Equal(t, "Alice", parsed.DisplayName)
}

func TestParseContactLink_NoName(t *testing.T) {
	result, err := GenerateNewIdentity()
	require.NoError(t, err)

	link := GenerateContactLink(result.Identity.Ed25519PubKey, result.Identity.X25519PubKey, "")
	parsed, err := ParseContactLink(link)
	require.NoError(t, err)

	assert.Equal(t, result.PublicKeyBase58, parsed.PublicKeyBase58)
	assert.Empty(t, parsed.DisplayName)
}

func TestParseContactLink_Invalid(t *testing.T) {
	_, err := ParseContactLink("http://invalid")
	assert.Error(t, err)

	_, err = ParseContactLink("btower://")
	assert.Error(t, err)

	_, err = ParseContactLink("btower://invalidbase58!!!")
	assert.Error(t, err)
}

func TestParseContactLink_SpecialCharsInName(t *testing.T) {
	result, err := GenerateNewIdentity()
	require.NoError(t, err)

	link := GenerateContactLink(result.Identity.Ed25519PubKey, result.Identity.X25519PubKey, "Alice & Bob")
	parsed, err := ParseContactLink(link)
	require.NoError(t, err)

	assert.Equal(t, "Alice & Bob", parsed.DisplayName)
}

func TestContactLinkRoundTrip(t *testing.T) {
	result, err := GenerateNewIdentity()
	require.NoError(t, err)

	names := []string{"Alice", "", "Bob Jones", "user_123"}
	for _, name := range names {
		link := GenerateContactLink(result.Identity.Ed25519PubKey, result.Identity.X25519PubKey,name)
		parsed, err := ParseContactLink(link)
		require.NoError(t, err)
		assert.Equal(t, result.PublicKeyBase58, parsed.PublicKeyBase58)
		assert.Equal(t, name, parsed.DisplayName)
	}
}

func splitWords(s string) []string {
	var words []string
	for _, w := range splitBySpace(s) {
		if w != "" {
			words = append(words, w)
		}
	}
	return words
}

func splitBySpace(s string) []string {
	result := make([]string, 0)
	current := ""
	for _, c := range s {
		if c == ' ' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
