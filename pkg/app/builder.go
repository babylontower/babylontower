package app

import (
	"context"
	"fmt"
	"time"

	"babylontower/pkg/config"
	"babylontower/pkg/groups"
	"babylontower/pkg/identity"
	"babylontower/pkg/ipfsnode"
	"babylontower/pkg/mailbox"
	"babylontower/pkg/messaging"
	"babylontower/pkg/storage"

	"github.com/ipfs/go-log/v2"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

var logger = log.Logger("babylontower/app")

// application is the concrete implementation of the Application interface.
type application struct {
	config    *AppConfig
	identity  *identity.Identity
	storage   storage.Storage
	ipfsNode  *ipfsnode.Node
	messaging *messaging.Service
	groups    *groups.Service
	mailbox   *mailbox.Manager

	// High-level managers (lazy-initialized)
	contactMgr *contactManager
	chatMgr    *chatManager

	ctx    context.Context
	cancel context.CancelFunc
}

// ApplicationBuilder provides fluent construction of Application.
type ApplicationBuilder struct {
	config       *AppConfig
	identity     *identity.Identity
	storage      storage.Storage
	ipfsNode     *ipfsnode.Node
	messaging    *messaging.Service
	groups       *groups.Service
	mailbox      *mailbox.Manager
	skipServices bool // For testing
}

// NewApplicationBuilder creates a new application builder.
func NewApplicationBuilder() *ApplicationBuilder {
	return &ApplicationBuilder{
		config: DefaultAppConfig(),
	}
}

// WithConfig sets the application configuration.
func (b *ApplicationBuilder) WithConfig(cfg *AppConfig) *ApplicationBuilder {
	b.config = cfg
	return b
}

// WithIdentity sets the identity.
func (b *ApplicationBuilder) WithIdentity(ident *identity.Identity) *ApplicationBuilder {
	b.identity = ident
	return b
}

// WithStorage sets the storage instance.
func (b *ApplicationBuilder) WithStorage(store storage.Storage) *ApplicationBuilder {
	b.storage = store
	return b
}

// WithIPFSNode sets the IPFS node.
func (b *ApplicationBuilder) WithIPFSNode(node *ipfsnode.Node) *ApplicationBuilder {
	b.ipfsNode = node
	return b
}

// WithMessaging sets the messaging service.
func (b *ApplicationBuilder) WithMessaging(svc *messaging.Service) *ApplicationBuilder {
	b.messaging = svc
	return b
}

// WithGroups sets the groups service.
func (b *ApplicationBuilder) WithGroups(svc *groups.Service) *ApplicationBuilder {
	b.groups = svc
	return b
}

// WithMailbox sets the mailbox manager.
func (b *ApplicationBuilder) WithMailbox(mb *mailbox.Manager) *ApplicationBuilder {
	b.mailbox = mb
	return b
}

// Build creates the Application instance and starts all services.
func (b *ApplicationBuilder) Build() (Application, error) {
	if b.skipServices {
		// Return minimal application for testing
		ctx, cancel := context.WithCancel(context.Background())
		return &application{
			config:   b.config,
			identity: b.identity,
			storage:  b.storage,
			ctx:      ctx,
			cancel:   cancel,
		}, nil
	}

	app := &application{
		config:    b.config,
		identity:  b.identity,
		storage:   b.storage,
		ipfsNode:  b.ipfsNode,
		messaging: b.messaging,
		groups:    b.groups,
		mailbox:   b.mailbox,
	}

	app.ctx, app.cancel = context.WithCancel(context.Background())

	return app, nil
}

// NewApplication creates a fully configured Application instance.
// This is the main entry point for creating Babylon Tower Core.
func NewApplication(appConfig *AppConfig, ident *identity.Identity) (Application, error) {
	builder := NewApplicationBuilder().
		WithConfig(appConfig).
		WithIdentity(ident)

	// Create storage
	store, err := createStorage(appConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}
	builder = builder.WithStorage(store)

	// Create IPFS node (pass shared storage to avoid lock conflicts)
	node, err := createIPFSNode(appConfig, store)
	if err != nil {
		if closeErr := store.Close(); closeErr != nil {
			logger.Warnw("failed to close storage", "error", closeErr)
		}
		return nil, fmt.Errorf("failed to create IPFS node: %w", err)
	}
	builder = builder.WithIPFSNode(node)

	// Start IPFS node
	if err := node.Start(); err != nil {
		if closeErr := store.Close(); closeErr != nil {
			logger.Warnw("failed to close storage", "error", closeErr)
		}
		if stopErr := node.Stop(); stopErr != nil {
			logger.Warnw("failed to stop node", "error", stopErr)
		}
		return nil, fmt.Errorf("failed to start IPFS node: %w", err)
	}

	// Create messaging service
	msgService := createMessagingService(ident, store, node)
	builder = builder.WithMessaging(msgService)

	// Create and start mailbox manager
	mailboxManager, err := createMailboxManager(node, ident, store)
	if err != nil {
		logger.Warnw("failed to create mailbox manager, offline delivery disabled", "error", err)
	} else {
		if err := mailboxManager.Start(); err != nil {
			logger.Warnw("failed to start mailbox manager, offline delivery disabled", "error", err)
			mailboxManager = nil
		} else {
			msgService.SetMailboxManager(mailboxManager)
			builder = builder.WithMailbox(mailboxManager)
		}
	}

	// Start messaging service
	if err := msgService.Start(); err != nil {
		if mailboxManager != nil {
			if stopErr := mailboxManager.Stop(); stopErr != nil {
				logger.Warnw("failed to stop mailbox manager", "error", stopErr)
			}
		}
		if stopErr := node.Stop(); stopErr != nil {
			logger.Warnw("failed to stop node", "error", stopErr)
		}
		if closeErr := store.Close(); closeErr != nil {
			logger.Warnw("failed to close storage", "error", closeErr)
		}
		return nil, fmt.Errorf("failed to start messaging service: %w", err)
	}

	// Create groups service
	groupsService := createGroupsService(ident, store)
	builder = builder.WithGroups(groupsService)

	return builder.Build()
}

// createStorage creates the storage instance.
func createStorage(appConfig *AppConfig) (storage.Storage, error) {
	return storage.NewBadgerStorage(storage.Config{
		Path:     appConfig.StorageDir,
		InMemory: false,
	})
}

// createIPFSNode creates the IPFS node.
func createIPFSNode(appConfig *AppConfig, sharedStore storage.Storage) (*ipfsnode.Node, error) {
	ipfsConfig := appConfig.NetworkConfig

	// Load stored peers using shared storage (avoids BadgerDB lock conflicts)
	storedPeers, err := loadStoredPeers(sharedStore, ipfsConfig)
	if err != nil {
		logger.Warnw("failed to load stored peers", "error", err)
	}

	return ipfsnode.NewNode(&ipfsnode.Config{
		RepoDir:            appConfig.IPFSDir,
		ProtocolID:         ipfsConfig.ProtocolID,
		BootstrapPeers:     ipfsConfig.BootstrapPeers,
		StoredPeers:        storedPeers,
		EnableRelay:        ipfsConfig.EnableRelay,
		EnableHolePunching: ipfsConfig.EnableHolePunching,
		DHTMode:            ipfsConfig.DHTMode,
		Storage:            sharedStore, // Pass shared storage to node
	})
}

// createMessagingService creates the messaging service.
func createMessagingService(ident *identity.Identity, store storage.Storage, node *ipfsnode.Node) *messaging.Service {
	// Try to create IdentityV1 from the PoC identity
	// This allows backward compatibility while enabling Protocol v1 features
	identityV1 := createIdentityV1FromLegacy(ident)

	msgConfig := &messaging.Config{
		OwnEd25519PrivKey: ident.Ed25519PrivKey,
		OwnEd25519PubKey:  ident.Ed25519PubKey,
		OwnX25519PrivKey:  ident.X25519PrivKey,
		OwnX25519PubKey:   ident.X25519PubKey,
		IdentityV1:        identityV1,
	}
	return messaging.NewService(msgConfig, store, node)
}

// createIdentityV1FromLegacy creates an IdentityV1 from the legacy PoC identity
// This is a temporary bridge until full migration to Protocol v1
func createIdentityV1FromLegacy(legacyIdent *identity.Identity) *identity.IdentityV1 {
	// For backward compatibility, we create an IdentityV1 using the PoC derivation method
	// This produces the same keys as the legacy identity but wrapped in IdentityV1 structure
	identityV1, err := identity.NewIdentityV1(legacyIdent.Mnemonic, "Default Device")
	if err != nil {
		logger.Warnw("failed to create IdentityV1, continuing without protocol v1 features", "error", err)
		return nil
	}
	return identityV1
}

// createMailboxManager creates the mailbox manager.
func createMailboxManager(node *ipfsnode.Node, ident *identity.Identity, store storage.Storage) (*mailbox.Manager, error) {
	storeImpl, ok := store.(*storage.BadgerStorage)
	if !ok {
		return nil, fmt.Errorf("storage type %T does not support mailbox manager", store)
	}

	mbConfig := mailbox.DefaultConfig()
	return mailbox.NewManager(node.Host(), node.DHT(), node, ident, storeImpl.DB(), mbConfig)
}

// createGroupsService creates the groups service.
func createGroupsService(ident *identity.Identity, store storage.Storage) *groups.Service {
	// Storage interface satisfies groups.GroupStorage since both BadgerStorage
	// and MemoryStorage implement SaveGroup and SaveSenderKey
	return groups.NewService(store, ident.Ed25519PubKey, ident.Ed25519PrivKey,
		groups.WithX25519PublicKey(ident.X25519PubKey))
}

// loadStoredPeers loads stored peers from storage.
// Uses the shared storage instance to avoid BadgerDB lock conflicts.
func loadStoredPeers(sharedStore storage.Storage, ipfsConfig *config.NetworkConfig) ([]libp2ppeer.AddrInfo, error) {
	// Use the shared storage instance instead of creating a new one
	peers, err := sharedStore.ListPeers(ipfsConfig.MaxStoredPeers)
	if err != nil {
		return nil, err
	}

	if len(peers) == 0 {
		return nil, nil
	}

	goodPeers := filterGoodPeers(peers)
	return goodPeers, nil
}

// filterGoodPeers filters peers based on success rate and staleness.
func filterGoodPeers(peers []*storage.PeerRecord) []libp2ppeer.AddrInfo {
	const (
		peerMaxAge             = 7 * 24 * time.Hour
		minPeerSuccessRate     = 0.7
		maxConsecutiveFailures = 3
	)

	var goodPeers []libp2ppeer.AddrInfo

	for _, peer := range peers {
		if peer.SuccessRate() < minPeerSuccessRate {
			continue
		}
		if peer.IsStale(peerMaxAge) {
			continue
		}
		if peer.FailCount >= maxConsecutiveFailures {
			continue
		}

		addrInfo, err := parsePeerRecord(peer)
		if err != nil {
			continue
		}
		goodPeers = append(goodPeers, addrInfo)
	}

	return goodPeers
}

// parsePeerRecord converts a PeerRecord to AddrInfo.
func parsePeerRecord(peer *storage.PeerRecord) (libp2ppeer.AddrInfo, error) {
	peerID, err := libp2ppeer.Decode(peer.PeerID)
	if err != nil {
		return libp2ppeer.AddrInfo{}, fmt.Errorf("invalid peer ID %q: %w", peer.PeerID, err)
	}
	addrInfo := libp2ppeer.AddrInfo{
		ID: peerID,
	}

	for _, addrStr := range peer.Multiaddrs {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}
		addrInfo.Addrs = append(addrInfo.Addrs, ma)
	}

	if len(addrInfo.Addrs) == 0 {
		return addrInfo, fmt.Errorf("no valid addresses for peer %s", peer.PeerID)
	}

	return addrInfo, nil
}

// Lifecycle methods

func (a *application) Start() error {
	logger.Infow("application started",
		"peer_id", a.ipfsNode.PeerID(),
		"addresses", a.ipfsNode.Multiaddrs())
	return nil
}

func (a *application) Stop() error {
	logger.Info("stopping application...")

	if a.cancel != nil {
		a.cancel()
	}

	var lastErr error

	if a.mailbox != nil {
		if err := a.mailbox.Stop(); err != nil {
			logger.Warnw("mailbox stop error", "error", err)
			lastErr = err
		}
	}

	if a.messaging != nil {
		if err := a.messaging.Stop(); err != nil {
			logger.Warnw("messaging stop error", "error", err)
			lastErr = err
		}
	}

	if a.ipfsNode != nil {
		if err := a.ipfsNode.Stop(); err != nil {
			logger.Warnw("ipfs node stop error", "error", err)
			lastErr = err
		}
	}

	if a.storage != nil {
		if err := a.storage.Close(); err != nil {
			logger.Warnw("storage close error", "error", err)
			lastErr = err
		}
	}

	logger.Info("application stopped")
	return lastErr
}

// Identity methods

func (a *application) GetIdentity() *IdentityInfo {
	fingerprint, _ := a.identity.ComputeFingerprint()
	displayName := ""
	if a.config != nil {
		displayName = a.config.DisplayName
	}
	info := &IdentityInfo{
		PublicKey:       a.identity.PublicKeyHex(),
		PublicKeyBase58: a.identity.PublicKeyBase58(),
		X25519KeyBase58: a.identity.X25519PublicKeyBase58(),
		Mnemonic:        a.identity.Mnemonic,
		Fingerprint:     fingerprint,
		ContactLink:     GenerateContactLink(a.identity.Ed25519PubKey, a.identity.X25519PubKey, displayName),
		DisplayName:     displayName,
	}
	if a.ipfsNode != nil {
		info.PeerID = a.ipfsNode.PeerID()
		info.Multiaddrs = a.ipfsNode.Multiaddrs()
	}
	return info
}

// Service accessors

func (a *application) Contacts() ContactManager {
	if a.contactMgr == nil {
		a.contactMgr = &contactManager{app: a}
	}
	return a.contactMgr
}

func (a *application) Chat() ChatManager {
	if a.chatMgr == nil {
		a.chatMgr = newChatManager(a)
	}
	return a.chatMgr
}

func (a *application) Messenger() Messenger {
	return &messengerAdapter{svc: a.messaging}
}

func (a *application) Groups() GroupManager {
	return a.groups
}

func (a *application) UIGroups() UIGroupManager {
	return newUIGroupManager(a)
}

func (a *application) Network() NetworkNode {
	return &networkNodeAdapter{node: a.ipfsNode}
}

func (a *application) Storage() storage.Storage {
	return a.storage
}

func (a *application) MessageEvents() <-chan *MessageEvent {
	return a.messaging.Messages()
}

// messengerAdapter adapts *messaging.Service to the Messenger interface
type messengerAdapter struct {
	svc *messaging.Service
}

func (a *messengerAdapter) Start() error {
	return a.svc.Start()
}

func (a *messengerAdapter) Stop() error {
	return a.svc.Stop()
}

func (a *messengerAdapter) SendMessageToContact(text string, recipientEd25519PubKey, recipientX25519PubKey []byte) (*messaging.SendResult, error) {
	return a.svc.SendMessageToContact(text, recipientEd25519PubKey, recipientX25519PubKey)
}

func (a *messengerAdapter) GetDecryptedMessagesWithMeta(contactPubKey []byte, limit, offset int) ([]*messaging.MessageWithMeta, error) {
	return a.svc.GetDecryptedMessagesWithMeta(contactPubKey, limit, offset)
}

func (a *messengerAdapter) Messages() <-chan *MessageEvent {
	if a.svc == nil {
		// Return a closed channel instead of nil to prevent panics
		ch := make(chan *MessageEvent)
		close(ch)
		return ch
	}
	return a.svc.Messages()
}

func (a *messengerAdapter) GetContactStatus(contactPubKey []byte) (*messaging.ContactStatus, error) {
	return a.svc.GetContactStatus(contactPubKey)
}

func (a *messengerAdapter) GetAllContactStatuses() ([]*messaging.ContactStatus, error) {
	return a.svc.GetAllContactStatuses()
}

func (a *messengerAdapter) IsStarted() bool {
	return a.svc.IsStarted()
}

func (a *messengerAdapter) GetMailboxManager() MailboxManager {
	mb := a.svc.GetMailboxManager()
	if mb == nil {
		return nil
	}
	return &mailboxManagerAdapter{manager: mb}
}

func (a *messengerAdapter) RetrieveOfflineMessages() error {
	return a.svc.RetrieveOfflineMessages()
}

func (a *messengerAdapter) ReputationTracker() ReputationTracker {
	rt := a.svc.ReputationTracker()
	if rt == nil {
		return nil
	}
	return &reputationTrackerAdapter{tracker: rt}
}

// networkNodeAdapter adapts ipfsnode.Node to the NetworkNode interface.
type networkNodeAdapter struct {
	node *ipfsnode.Node
}

func (a *networkNodeAdapter) Start() error {
	return a.node.Start()
}

func (a *networkNodeAdapter) Stop() error {
	return a.node.Stop()
}

func (a *networkNodeAdapter) ConnectToPeer(maddr string) error {
	return a.node.ConnectToPeer(maddr)
}

func (a *networkNodeAdapter) FindPeer(peerID string) (*PeerAddrInfo, error) {
	info, err := a.node.FindPeer(peerID)
	if err != nil {
		return nil, err
	}

	addrs := make([]string, len(info.Addrs))
	for i, addr := range info.Addrs {
		addrs[i] = addr.String()
	}

	protocols, err := a.node.Host().Peerstore().GetProtocols(info.ID)
	if err != nil {
		protocols = nil
	}

	protoStrs := make([]string, len(protocols))
	for i, p := range protocols {
		protoStrs[i] = string(p)
	}

	return &PeerAddrInfo{
		ID:        info.ID.String(),
		Addrs:     addrs,
		Protocols: protoStrs,
	}, nil
}

func (a *networkNodeAdapter) Subscribe(topic string) (*NetworkSubscription, error) {
	sub, err := a.node.Subscribe(topic)
	if err != nil {
		return nil, err
	}

	return &NetworkSubscription{
		MessagesFn: func() <-chan []byte {
			out := make(chan []byte, 100)
			go func() {
				for msg := range sub.Messages() {
					out <- msg.Data
				}
				close(out)
			}()
			return out
		},
		ErrorsFn: sub.Errors(),
		CloseFn:  sub.Close,
	}, nil
}

func (a *networkNodeAdapter) AdvertiseSelf(ctx context.Context) error {
	return a.node.AdvertiseSelf(ctx)
}

func (a *networkNodeAdapter) WaitForDHT(timeout time.Duration) error {
	return a.node.WaitForDHT(timeout)
}

func (a *networkNodeAdapter) WaitForBabylonBootstrap(timeout time.Duration) error {
	return a.node.WaitForBabylonBootstrap(timeout)
}

func (a *networkNodeAdapter) PeerID() string {
	return a.node.PeerID()
}

func (a *networkNodeAdapter) Multiaddrs() []string {
	return a.node.Multiaddrs()
}

func (a *networkNodeAdapter) IsStarted() bool {
	return a.node.IsStarted()
}

func (a *networkNodeAdapter) Context() context.Context {
	return a.node.Context()
}

func (a *networkNodeAdapter) Host() host.Host {
	return a.node.Host()
}

func (a *networkNodeAdapter) DHT() *dht.IpfsDHT {
	return a.node.DHT()
}

func (a *networkNodeAdapter) PubSub() *pubsub.PubSub {
	return a.node.PubSub()
}

func (a *networkNodeAdapter) GetNetworkInfo() *NetworkInfo {
	info := a.node.GetNetworkInfo()
	return &NetworkInfo{
		PeerID:             info.PeerID,
		Multiaddrs:         info.Multiaddrs,
		ListenAddrs:        info.ListenAddrs,
		ConnectedPeerCount: info.ConnectedPeerCount,
		IsStarted:          info.IsStarted,
		ConnectedPeers: func() []PeerInfo {
			peers := make([]PeerInfo, len(info.ConnectedPeers))
			for i, p := range info.ConnectedPeers {
				peers[i] = PeerInfo{
					ID:        p.ID,
					Addresses: p.Addresses,
					Protocols: p.Protocols,
					Connected: p.Connected,
				}
			}
			return peers
		}(),
	}
}

func (a *networkNodeAdapter) GetDHTInfo() *DHTInfo {
	info := a.node.GetDHTInfo()
	return &DHTInfo{
		IsStarted:              info.IsStarted,
		Mode:                   info.Mode,
		RoutingTableSize:       info.RoutingTableSize,
		RoutingTablePeers:      info.RoutingTablePeers,
		ConnectedPeerCount:     info.ConnectedPeerCount,
		HasBootstrapConnection: info.HasBootstrapConnection,
	}
}

func (a *networkNodeAdapter) ClearAllBackoffs() {
	a.node.ClearAllBackoffs()
}

func (a *networkNodeAdapter) GetMDnsStats() MDnsStats {
	stats := a.node.GetMDnsStats()
	return MDnsStats{
		TotalDiscoveries: stats.TotalDiscoveries,
		LastPeerFound:    stats.LastPeerFound,
	}
}

func (a *networkNodeAdapter) GetMetricsFull() *MetricsFull {
	metrics := a.node.GetMetricsFull()
	return &MetricsFull{
		PeerID:                  metrics.PeerID,
		StartTime:               metrics.StartTime,
		UptimeSeconds:           metrics.UptimeSeconds,
		CurrentConnections:      int(metrics.CurrentConnections),
		TotalConnections:        int(metrics.TotalConnections),
		TotalDisconnections:     int(metrics.TotalDisconnections),
		ConnectionSuccessRate:   metrics.ConnectionSuccessRate,
		AverageLatencyMs:        metrics.AverageLatencyMs,
		DHTDiscoveries:          metrics.DHTDiscoveries,
		MDNSDiscoveries:         metrics.MDNSDiscoveries,
		PeerExchangeDiscoveries: metrics.PeerExchangeDiscoveries,
		DiscoveryBySource:       metrics.DiscoveryBySource,
		SuccessfulMessages:      metrics.SuccessfulMessages,
		FailedMessages:          metrics.FailedMessages,
		MessageSuccessRate:      metrics.MessageSuccessRate,
		BootstrapAttempts:       int(metrics.BootstrapAttempts),
		BootstrapSuccesses:      int(metrics.BootstrapSuccesses),
		LastBootstrapTime:       metrics.LastBootstrapTime,
	}
}

func (a *networkNodeAdapter) GetBabylonDHTInfo() *BabylonDHTInfo {
	info := a.node.GetBabylonDHTInfo()
	return &BabylonDHTInfo{
		StoredBabylonPeers:    info.StoredBabylonPeers,
		ConnectedBabylonPeers: info.ConnectedBabylonPeers,
		BabylonPeerIDs:        info.BabylonPeerIDs,
		RendezvousActive: info.RendezvousActive,
	}
}

func (a *networkNodeAdapter) GetPeerCountsBySource() *PeerCountsBySource {
	counts := a.node.GetPeerCountsBySource()
	return &PeerCountsBySource{
		Babylon:        counts.Babylon,
		IPFSBootstrap:  counts.IPFSBootstrap,
		IPFSDiscovery:  counts.IPFSDiscovery,
		MDNS:           counts.MDNS,
		ConnectedTotal: counts.ConnectedTotal,
	}
}

func (a *networkNodeAdapter) GetBootstrapStatus() *BootstrapStatus {
	status := a.node.GetBootstrapStatus()
	return &BootstrapStatus{
		IPFSBootstrapComplete:    status.IPFSBootstrapComplete,
		BabylonBootstrapComplete: status.BabylonBootstrapComplete,
		BabylonPeersStored:       status.BabylonPeersStored,
		BabylonPeersConnected:    status.BabylonPeersConnected,
		RendezvousActive:         status.RendezvousActive,
	}
}

func (a *networkNodeAdapter) TriggerRendezvousDiscovery() int {
	return a.node.TriggerRendezvousDiscovery()
}
