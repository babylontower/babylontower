package identity

import (
	"encoding/json"

	"github.com/mr-tron/base58"
)

// EncodeBase58 encodes bytes to a base58 string
// Used for human-readable public key representation
func EncodeBase58(input []byte) string {
	if len(input) == 0 {
		return ""
	}
	return base58.Encode(input)
}

// DecodeBase58 decodes a base58 string to bytes
// Returns error for invalid characters
func DecodeBase58(input string) ([]byte, error) {
	if len(input) == 0 {
		return []byte{}, nil
	}
	return base58.Decode(input)
}

// marshalJSON wraps json.Marshal with consistent options
func marshalJSON(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// unmarshalJSON wraps json.Unmarshal with consistent options
func unmarshalJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
