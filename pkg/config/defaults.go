package config

import "time"

// DefaultAppConfig returns an AppConfig with sensible defaults that match the
// existing hardcoded values throughout the codebase.
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		Network: NetworkConfig{
			BootstrapPeers: []string{
				"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
				"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
				"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiRNN6vEC9qmL9egu92p",
				"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
				"/dnsaddr/bootstrap.libp2p.io/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
				"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
				"/ip4/104.236.179.241/tcp/4001/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
				"/ip4/128.199.219.111/tcp/4001/p2p/QmSoLV4Bbm51jM9C4gDYZQ9Cy3U6aXMJDAbzgu2fzaDs64",
				"/ip4/104.236.76.40/tcp/4001/p2p/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
				"/ip4/178.62.158.147/tcp/4001/p2p/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd",
				"/ip4/178.62.61.185/tcp/4001/p2p/QmSoLMeWqB7YGVLJN3pNLQpmmEk35v6wYtsMGLzSr5QBU3",
			},
			DialTimeout:         15 * time.Second,
			ConnectionTimeout:   30 * time.Second,
			BootstrapTimeout:    60 * time.Second,
			MaxConnections:      400,
			LowWater:            50,
			HighWater:           400,
			EnableRelay:         false,
			EnableHolePunching:  true,
			EnableAutoNAT:       true,
			DHTMode:             "auto",
			DHTBootstrapTimeout: 60 * time.Second,
			ListenAddrs: []string{
				"/ip4/0.0.0.0/tcp/0",
				"/ip4/0.0.0.0/tcp/0/ws",
				"/ip6/::/tcp/0",
			},
			ProtocolID:         "/babylontower/1.0.0",
			MaxStoredPeers:     100,
			MinPeerConnections: 10,
		},
		Storage: StorageConfig{
			Path:     "", // Set at runtime from data-dir
			InMemory: false,
		},
		Logging: LoggingConfig{
			Level: "warn",
			File:  "",
		},
		Messaging: MessagingConfig{
			ChannelBufferSize: 100,
		},
		Mailbox: MailboxConfig{
			MaxMessagesPerTarget:   500,
			MaxMessageSize:         256 * 1024,       // 256 KB
			MaxTotalBytesPerTarget: 64 * 1024 * 1024, // 64 MB
			DefaultTTL:             7 * 24 * time.Hour,
			RateLimitPerMinute:     60,
		},
		RTC: RTCConfig{
			CallTimeout:     30 * time.Second,
			EnableVideo:     true,
			MaxParticipants: 25,
		},
		Multidevice: MultideviceConfig{
			DeviceName:   "",
			MaxDevices:   5,
			SyncInterval: 30 * time.Second,
		},
		Identity: IdentityConfig{
			DHTPublish:      true,
			RefreshInterval: 4 * time.Hour,
		},
		Bootstrap: BootstrapConfig{
			PubSubTopic:             "/babylon/bootstrap",
			ResponseProbability:     0.5,
			MaxResponsesPerMinute:   30,
			RequestDedupWindowSecs:  30,
			MinUptimeSecs:           300, // 5 minutes
			MinPeerCount:            3,
			MinRoutingTableSize:     10,
			StoredPeerTimeoutSecs:   10,
			PubSubListenSecs:        5,
			MinBabylonPeersRequired: 3,
		},
	}
}
