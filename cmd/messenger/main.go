package main

import (
	"fmt"
	"os"
	"path/filepath"

	"babylontower/pkg/cli"
	"babylontower/pkg/identity"
	"babylontower/pkg/ipfsnode"
	"babylontower/pkg/messaging"
	"babylontower/pkg/storage"
	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("babylontower")

// Version information injected at build time
var (
	Version   = "0.1.0-poc"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

const (
	// DefaultDataDir is the default directory for application data
	DefaultDataDir = ".babylontower"
	// IdentityFileName is the name of the identity file
	IdentityFileName = "identity.json"
	// StorageDirName is the name of the storage directory
	StorageDirName = "storage"
	// IPFSDirName is the name of the IPFS repo directory
	IPFSDirName = "ipfs"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Setup logging - use debug level for development
	if err := log.SetLogLevel("babylontower", "debug"); err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Setup data directory
	dataDir := filepath.Join(homeDir, DefaultDataDir)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Setup paths
	identityPath := filepath.Join(dataDir, IdentityFileName)
	storageDir := filepath.Join(dataDir, StorageDirName)
	ipfsDir := filepath.Join(dataDir, IPFSDirName)

	logger.Infow("starting Babylon Tower", "version", Version, "data_dir", dataDir)

	// Load or create identity
	ident, err := loadOrCreateIdentity(identityPath)
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}

	logger.Infow("identity loaded", "public_key", ident.PublicKeyHex())

	// Initialize storage
	store, err := storage.NewBadgerStorage(storage.Config{
		Path:     storageDir,
		InMemory: false,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Warnw("failed to close storage", "error", err)
		}
	}()

	logger.Info("storage initialized")

	// Initialize IPFS node
	ipfsNode, err := ipfsnode.NewNode(&ipfsnode.Config{
		RepoDir: ipfsDir,
	})
	if err != nil {
		return fmt.Errorf("failed to create IPFS node: %w", err)
	}

	// Start IPFS node
	if err := ipfsNode.Start(); err != nil {
		return fmt.Errorf("failed to start IPFS node: %w", err)
	}
	defer func() {
		if err := ipfsNode.Stop(); err != nil {
			logger.Warnw("failed to stop IPFS node", "error", err)
		}
	}()

	logger.Infow("IPFS node started", "peer_id", ipfsNode.PeerID())

	// Initialize messaging service
	msgConfig := &messaging.Config{
		OwnEd25519PrivKey: ident.Ed25519PrivKey,
		OwnEd25519PubKey:  ident.Ed25519PubKey,
		OwnX25519PrivKey:  ident.X25519PrivKey,
		OwnX25519PubKey:   ident.X25519PubKey,
	}

	msgService := messaging.NewService(msgConfig, store, ipfsNode)

	// Start messaging service
	if err := msgService.Start(); err != nil {
		return fmt.Errorf("failed to start messaging service: %w", err)
	}
	defer func() {
		if err := msgService.Stop(); err != nil {
			logger.Warnw("failed to stop messaging service", "error", err)
		}
	}()

	logger.Info("messaging service started")

	// Create CLI
	cliConfig := &cli.Config{
		Version:      Version,
		DataDir:      dataDir,
		IdentityPath: identityPath,
	}

	// Convert identity to CLI identity
	cliIdentity := &cli.Identity{
		Ed25519PubKey:  ident.Ed25519PubKey,
		Ed25519PrivKey: ident.Ed25519PrivKey,
		X25519PubKey:   ident.X25519PubKey,
		X25519PrivKey:  ident.X25519PrivKey,
	}

	app, err := cli.New(cliConfig, cliIdentity, store, ipfsNode, msgService)
	if err != nil {
		return fmt.Errorf("failed to create CLI: %w", err)
	}

	// Start CLI main loop
	if err := app.Start(); err != nil {
		return fmt.Errorf("CLI error: %w", err)
	}

	// Graceful shutdown
	if err := app.Stop(); err != nil {
		logger.Warnw("CLI stop error", "error", err)
	}

	logger.Info("Babylon Tower shutdown complete")

	return nil
}

// loadOrCreateIdentity loads an existing identity or creates a new one
func loadOrCreateIdentity(identityPath string) (*identity.Identity, error) {
	if identity.IdentityExists(identityPath) {
		// Load existing identity
		ident, err := identity.LoadIdentity(identityPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load identity: %w", err)
		}
		return ident, nil
	}

	// Generate new identity
	logger.Info("Generating new identity...")
	ident, err := identity.GenerateIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity: %w", err)
	}

	// Save identity
	if err := identity.SaveIdentity(ident, identityPath); err != nil {
		return nil, fmt.Errorf("failed to save identity: %w", err)
	}

	logger.Info("New identity generated and saved")
	fmt.Printf("\n🎉 New identity generated!\n")
	fmt.Printf("Your mnemonic (write this down safely):\n")
	fmt.Printf("  %s\n\n", ident.Mnemonic)
	fmt.Printf("WARNING: If you lose this mnemonic, you lose your identity!\n")
	fmt.Printf("Store it in a secure location.\n\n")

	return ident, nil
}
