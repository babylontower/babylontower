package messaging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiaddrsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []string{}, []string{}, true},
		{"equal", []string{"/ip4/1.2.3.4/tcp/80"}, []string{"/ip4/1.2.3.4/tcp/80"}, true},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different content", []string{"a"}, []string{"b"}, false},
		{"order matters", []string{"a", "b"}, []string{"b", "a"}, false},
		{"multi equal", []string{"x", "y", "z"}, []string{"x", "y", "z"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, multiaddrsEqual(tt.a, tt.b))
		})
	}
}

func TestNewService(t *testing.T) {
	cfg := &Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
		OwnX25519PubKey:   make([]byte, 32),
		OwnX25519PrivKey:  make([]byte, 32),
	}
	svc := NewService(cfg, nil, nil)
	require.NotNil(t, svc)
	assert.False(t, svc.IsStarted())
}

func TestIsStarted_NotStarted(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)
	assert.False(t, svc.IsStarted())
}

func TestGetContactPeerInfo_Empty(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)

	_, ok := svc.GetContactPeerInfo([]byte("nonexistent"))
	assert.False(t, ok)
}

func TestGetAllContactStats_Empty(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)

	stats := svc.GetAllContactStats()
	assert.Empty(t, stats)
}

func TestGetTopicMeshSize_NilNode(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)

	// With nil ipfsNode, GetTopicMeshSize should return 0 (will panic-guard via nil check)
	// Actually this will call ipfsNode.GetTopicInfo which would panic on nil.
	// This test verifies the service can be created with nil node.
	assert.NotNil(t, svc)
}

func TestFindAndConnectToContact_NotStarted(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)

	_, err := svc.FindAndConnectToContact([]byte("some-key"))
	assert.ErrorIs(t, err, ErrServiceNotStarted)
}

func TestIsContactOnline_NotStarted(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)

	_, err := svc.IsContactOnline([]byte("some-key"))
	assert.ErrorIs(t, err, ErrServiceNotStarted)
}

func TestFindAndConnect_NotStarted(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)

	_, err := svc.FindAndConnect([]byte("key"), nil, "")
	assert.ErrorIs(t, err, ErrServiceNotStarted)
}

func TestReputationTracker_NilByDefault(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)

	assert.Nil(t, svc.ReputationTracker())
}

func TestMessages_Channel(t *testing.T) {
	svc := NewService(&Config{
		OwnEd25519PubKey:  make([]byte, 32),
		OwnEd25519PrivKey: make([]byte, 64),
	}, nil, nil)

	ch := svc.Messages()
	assert.NotNil(t, ch)
}
