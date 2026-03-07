// Package ui provides the Gio-based graphical user interface entry point for Babylon Tower.
package ui

import (
	"context"
	"fmt"

	babylonapp "babylontower/pkg/app"
	"babylontower/pkg/storage"

	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("babylontower/ui")

// Config holds UI configuration
type Config struct {
	// DarkMode enables dark theme
	DarkMode bool
	// Title is the window title
	Title string
}

// UI represents the graphical user interface
type UI struct {
	config    *Config
	storage   storage.Storage
	messenger babylonapp.Messenger
	groups    babylonapp.GroupManager
	network   babylonapp.NetworkNode
	coreApp   babylonapp.Application

	ctx    context.Context
	cancel context.CancelFunc

	// The Gio app instance
	app *App
}

// New creates a new UI instance
func New(config *Config, coreApp babylonapp.Application) (*UI, error) {
	ctx, cancel := context.WithCancel(context.Background())

	ui := &UI{
		config:    config,
		storage:   coreApp.Storage(),
		messenger: coreApp.Messenger(),
		groups:    coreApp.Groups(),
		network:   coreApp.Network(),
		coreApp:   coreApp,
		ctx:       ctx,
		cancel:    cancel,
	}

	return ui, nil
}

// Start starts the UI event loop
func (u *UI) Start() error {
	logger.Info("starting Babylon Tower UI")

	// Create the Gio application
	appConfig := &AppConfig{
		DarkMode: u.config.DarkMode,
		Title:    u.config.Title,
		Width:    1200,
		Height:   800,
	}

	if appConfig.Title == "" {
		appConfig.Title = "Babylon Tower"
	}

	u.app = NewApp(u.coreApp, appConfig)

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
