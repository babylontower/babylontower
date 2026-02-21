package cli

import (
	"testing"

	pb "babylontower/pkg/proto"
	"github.com/stretchr/testify/assert"
)

func TestFormatPublicKey(t *testing.T) {
	tests := []struct {
		name     string
		pubKey   []byte
		expected string
	}{
		{
			name:     "short key",
			pubKey:   []byte{0x01, 0x02, 0x03},
			expected: "010203",
		},
		{
			name:     "long key truncated",
			pubKey:   []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11},
			expected: "0102030405060708...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatPublicKey(tt.pubKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatContact(t *testing.T) {
	tests := []struct {
		name        string
		index       int
		displayName string
		pubKey      []byte
		expected    string
	}{
		{
			name:        "with name",
			index:       1,
			displayName: "Alice",
			pubKey:      []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			expected:    "[1] Alice - 3jYEF...",
		},
		{
			name:        "without name",
			index:       2,
			displayName: "",
			pubKey:      []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			expected:    "[2] (no name) - 3jYEF...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contact := &pb.Contact{
				DisplayName: tt.displayName,
				PublicKey:   tt.pubKey,
			}
			result := FormatContact(tt.index, contact)
			assert.Contains(t, result, tt.expected[:5]) // Check start of expected string
		})
	}
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "simple error",
			err:      assert.AnError,
			expected: "❌ Error: assert.AnError general error for testing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatErrorString(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
		{
			name:     "simple message",
			message:  "Something went wrong",
			expected: "❌ Error: Something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatErrorString(tt.message)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSuccess(t *testing.T) {
	result := FormatSuccess("Operation completed")
	assert.Equal(t, "✅ Operation completed", result)
}

func TestFormatInfo(t *testing.T) {
	result := FormatInfo("Here is some info")
	assert.Equal(t, "ℹ️  Here is some info", result)
}

func TestFormatHelp(t *testing.T) {
	result := FormatHelp()
	assert.Contains(t, result, "Babylon Tower Commands")
	assert.Contains(t, result, "/help")
	assert.Contains(t, result, "/myid")
	assert.Contains(t, result, "/add")
	assert.Contains(t, result, "/list")
	assert.Contains(t, result, "/chat")
	assert.Contains(t, result, "/history")
	assert.Contains(t, result, "/exit")
}

func TestFormatBanner(t *testing.T) {
	result := FormatBanner("0.1.0", "3jYEF...")
	assert.Contains(t, result, "Babylon Tower v0.1.0")
	assert.Contains(t, result, "3jYEF...")
	assert.Contains(t, result, "/help")
}

func TestFormatChatHeader(t *testing.T) {
	result := FormatChatHeader("Alice", "3jYEF...")
	assert.Contains(t, result, "Chat with Alice")
	assert.Contains(t, result, "Public key: 3jYEF...")
}

func TestFormatChatExit(t *testing.T) {
	result := FormatChatExit()
	assert.Contains(t, result, "Exited chat mode")
}

func TestFormatMessage(t *testing.T) {
	msg := &pb.Message{
		Text:      "Hello, World!",
		Timestamp: 1234567890,
	}

	result := FormatMessage(msg, "Alice", false)
	assert.Contains(t, result, "Alice")
	assert.Contains(t, result, "Hello, World!")
}

func TestFormatContactList(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		result := FormatContactList([]*pb.Contact{})
		assert.Contains(t, result, "No contacts found")
		assert.Contains(t, result, "/add")
	})

	t.Run("with contacts", func(t *testing.T) {
		contacts := []*pb.Contact{
			{
				DisplayName: "Alice",
				PublicKey:   []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			},
			{
				DisplayName: "Bob",
				PublicKey:   []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20},
			},
		}
		result := FormatContactList(contacts)
		assert.Contains(t, result, "Contacts")
		assert.Contains(t, result, "Alice")
		assert.Contains(t, result, "Bob")
	})
}
