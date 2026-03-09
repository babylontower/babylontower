// Package ui provides the Gio-based graphical user interface entry point for Babylon Tower.
package ui

import (
	"context"
	"fmt"
	"sync"

	babylonapp "babylontower/pkg/app"
	"babylontower/pkg/config"

	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("babylontower/ui")

// Config holds UI configuration
type Config struct {
	// DarkMode enables dark theme
	DarkMode bool
	// Title is the window title
	Title string
	// DataDir is the data directory for saving config
	DataDir string
	// AppConfig is the loaded application config (for settings screen)
	AppConfig *config.AppConfig
}

// UI represents the graphical user interface
type UI struct {
	config  *Config
	coreApp babylonapp.Application

	ctx    context.Context
	cancel context.CancelFunc

	mu sync.Mutex
	// The Gio app instance
	app *App
}

// New creates a new UI instance.
// coreApp may be nil if services are being initialized asynchronously.
func New(config *Config, coreApp babylonapp.Application) (*UI, error) {
	ctx, cancel := context.WithCancel(context.Background())

	ui := &UI{
		config:  config,
		coreApp: coreApp,
		ctx:     ctx,
		cancel:  cancel,
	}

	return ui, nil
}

// SetCoreApp forwards core app attachment to the underlying Gio App.
func (u *UI) SetCoreApp(coreApp babylonapp.Application) {
	u.mu.Lock()
	u.coreApp = coreApp
	gioApp := u.app
	u.mu.Unlock()

	if gioApp != nil {
		gioApp.SetCoreApp(coreApp)
	}
}

// SetCoreError forwards a core initialization error to the underlying Gio App.
func (u *UI) SetCoreError(err error) {
	u.mu.Lock()
	gioApp := u.app
	u.mu.Unlock()

	if gioApp != nil {
		gioApp.SetCoreError(err)
	}
}

// Start starts the UI event loop
func (u *UI) Start() error {
	logger.Info("starting Babylon Tower UI")

	// Create the Gio application
	appConfig := &GioAppConfig{
		DarkMode:  u.config.DarkMode,
		Title:     u.config.Title,
		Width:     1200,
		Height:    800,
		DataDir:   u.config.DataDir,
		AppConfig: u.config.AppConfig,
	}

	if appConfig.Title == "" {
		appConfig.Title = "Babylon Tower"
	}

	u.mu.Lock()
	u.app = NewApp(u.coreApp, appConfig)
	// If SetCoreApp was called before app was created, forward it now
	if u.coreApp != nil && u.app.coreApp == nil {
		u.app.SetCoreApp(u.coreApp)
	}
	u.mu.Unlock()

	// Run the event loop (blocking)
	if err := u.app.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the UI
func (u *UI) Stop() {
	logger.Info("stopping Babylon Tower UI")
	u.cancel()
	if u.app != nil {
		u.app.Stop()
	}
}
