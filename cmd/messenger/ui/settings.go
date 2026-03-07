package ui

import (
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"babylontower/pkg/config"
)

// SettingsState holds the settings editor state
type SettingsState struct {
	// Current tab
	currentTab int

	// Network settings
	networkTab *NetworkSettingsState

	// Appearance settings
	appearanceTab *AppearanceSettingsState

	// Privacy settings
	privacyTab *PrivacySettingsState

	// Notifications settings
	notificationsTab *NotificationsSettingsState

	// Storage settings
	storageTab *StorageSettingsState

	// About section
	aboutTab *AboutSettingsState

	// Save button
	saveButton widget.Clickable

	// Cancel button
	cancelButton widget.Clickable

	// Status message
	statusMessage string

	// Show modified indicator
	isModified bool
}

// NetworkSettingsState holds network settings editor state
type NetworkSettingsState struct {
	// Listen addresses editor
	listenAddrsEditor widget.Editor

	// Bootstrap peers editor
	bootstrapPeersEditor widget.Editor

	// Connection limits
	maxConnectionsEditor     widget.Editor
	lowWaterEditor           widget.Editor
	highWaterEditor          widget.Editor
	maxStoredPeersEditor     widget.Editor
	minPeerConnectionsEditor widget.Editor

	// Timeouts
	bootstrapTimeoutEditor    widget.Editor
	connectionTimeoutEditor   widget.Editor
	dialTimeoutEditor         widget.Editor
	dhtBootstrapTimeoutEditor widget.Editor

	// Toggles
	enableRelay        widget.Bool
	enableHolePunching widget.Bool
	enableAutoNAT      widget.Bool

	// DHT mode - use text editor
	dhtModeEditor widget.Editor

	// Protocol ID
	protocolIDEditor widget.Editor

	// Scroll list
	list widget.List
}

// AppearanceSettingsState holds appearance settings editor state
type AppearanceSettingsState struct {
	// Theme selection
	darkMode widget.Bool

	// Font size
	fontSizeEditor widget.Editor

	// Window size
	windowWidthEditor  widget.Editor
	windowHeightEditor widget.Editor

	// Scroll list
	list widget.List
}

// PrivacySettingsState holds privacy settings editor state
type PrivacySettingsState struct {
	// Identity visibility
	dhtPublish widget.Bool

	// Multi-device
	maxDevicesEditor widget.Editor

	// Scroll list
	list widget.List
}

// NotificationsSettingsState holds notifications settings editor state
type NotificationsSettingsState struct {
	// Sound toggles
	enableSoundNotifications widget.Bool
	enableDesktopAlerts      widget.Bool

	// Message notifications
	notifyOnMention widget.Bool
	notifyOnMessage widget.Bool

	// Call notifications
	notifyOnCall widget.Bool

	// Scroll list
	list widget.List
}

// StorageSettingsState holds storage settings editor state
type StorageSettingsState struct {
	// Storage path
	storagePathEditor widget.Editor

	// In-memory storage
	inMemoryStorage widget.Bool

	// Message limits
	maxMessagesPerTargetEditor   widget.Editor
	maxMessageSizeEditor         widget.Editor
	maxTotalBytesPerTargetEditor widget.Editor
	defaultTTLEditor             widget.Editor
	rateLimitPerMinuteEditor     widget.Editor

	// Scroll list
	list widget.List
}

// AboutSettingsState holds about section state
type AboutSettingsState struct {
	// Version info (read-only)
	version   string
	buildTime string
	gitCommit string

	// Scroll list
	list widget.List
}

// NewSettingsState creates a new settings state
func NewSettingsState(cfg *config.AppConfig, version, buildTime, gitCommit string) *SettingsState {
	state := &SettingsState{
		currentTab:       0,
		networkTab:       &NetworkSettingsState{},
		appearanceTab:    &AppearanceSettingsState{},
		privacyTab:       &PrivacySettingsState{},
		notificationsTab: &NotificationsSettingsState{},
		storageTab:       &StorageSettingsState{},
		aboutTab: &AboutSettingsState{
			version:   version,
			buildTime: buildTime,
			gitCommit: gitCommit,
		},
	}

	// Initialize network settings
	initNetworkSettings(state.networkTab, cfg)

	// Initialize appearance settings
	initAppearanceSettings(state.appearanceTab)

	// Initialize privacy settings
	initPrivacySettings(state.privacyTab, cfg)

	// Initialize notifications settings
	initNotificationsSettings(state.notificationsTab)

	// Initialize storage settings
	initStorageSettings(state.storageTab, cfg)

	return state
}

// initNetworkSettings initializes network settings from config
func initNetworkSettings(state *NetworkSettingsState, cfg *config.AppConfig) {
	// Initialize editors
	state.listenAddrsEditor.SingleLine = false
	state.listenAddrsEditor.SetText(joinStrings(cfg.Network.ListenAddrs, "\n"))

	state.bootstrapPeersEditor.SingleLine = false
	state.bootstrapPeersEditor.SetText(joinStrings(cfg.Network.BootstrapPeers, "\n"))

	// Connection limits
	state.maxConnectionsEditor.SingleLine = true
	state.maxConnectionsEditor.SetText(itoa(cfg.Network.MaxConnections))

	state.lowWaterEditor.SingleLine = true
	state.lowWaterEditor.SetText(itoa(cfg.Network.LowWater))

	state.highWaterEditor.SingleLine = true
	state.highWaterEditor.SetText(itoa(cfg.Network.HighWater))

	state.maxStoredPeersEditor.SingleLine = true
	state.maxStoredPeersEditor.SetText(itoa(cfg.Network.MaxStoredPeers))

	state.minPeerConnectionsEditor.SingleLine = true
	state.minPeerConnectionsEditor.SetText(itoa(cfg.Network.MinPeerConnections))

	// Timeouts
	state.bootstrapTimeoutEditor.SingleLine = true
	state.bootstrapTimeoutEditor.SetText(cfg.Network.BootstrapTimeout.String())

	state.connectionTimeoutEditor.SingleLine = true
	state.connectionTimeoutEditor.SetText(cfg.Network.ConnectionTimeout.String())

	state.dialTimeoutEditor.SingleLine = true
	state.dialTimeoutEditor.SetText(cfg.Network.DialTimeout.String())

	state.dhtBootstrapTimeoutEditor.SingleLine = true
	state.dhtBootstrapTimeoutEditor.SetText(cfg.Network.DHTBootstrapTimeout.String())

	// Toggles
	state.enableRelay.Value = cfg.Network.EnableRelay
	state.enableHolePunching.Value = cfg.Network.EnableHolePunching
	state.enableAutoNAT.Value = cfg.Network.EnableAutoNAT

	// DHT mode - use text editor
	state.dhtModeEditor.SingleLine = true
	state.dhtModeEditor.SetText(cfg.Network.DHTMode)

	// Protocol ID
	state.protocolIDEditor.SingleLine = true
	state.protocolIDEditor.SetText(cfg.Network.ProtocolID)

	// Setup list
	state.list.Axis = layout.Vertical
}

// initAppearanceSettings initializes appearance settings
func initAppearanceSettings(state *AppearanceSettingsState) {
	state.fontSizeEditor.SingleLine = true
	state.fontSizeEditor.SetText("14")

	state.windowWidthEditor.SingleLine = true
	state.windowWidthEditor.SetText("1200")

	state.windowHeightEditor.SingleLine = true
	state.windowHeightEditor.SetText("800")

	state.list.Axis = layout.Vertical
}

// initPrivacySettings initializes privacy settings from config
func initPrivacySettings(state *PrivacySettingsState, cfg *config.AppConfig) {
	state.dhtPublish.Value = cfg.Identity.DHTPublish

	state.maxDevicesEditor.SingleLine = true
	state.maxDevicesEditor.SetText(itoa(cfg.Multidevice.MaxDevices))

	state.list.Axis = layout.Vertical
}

// initNotificationsSettings initializes notifications settings
func initNotificationsSettings(state *NotificationsSettingsState) {
	state.enableSoundNotifications.Value = true
	state.enableDesktopAlerts.Value = true
	state.notifyOnMention.Value = true
	state.notifyOnMessage.Value = true
	state.notifyOnCall.Value = true

	state.list.Axis = layout.Vertical
}

// initStorageSettings initializes storage settings from config
func initStorageSettings(state *StorageSettingsState, cfg *config.AppConfig) {
	state.storagePathEditor.SingleLine = true
	state.storagePathEditor.SetText(cfg.Storage.Path)

	state.inMemoryStorage.Value = cfg.Storage.InMemory

	state.maxMessagesPerTargetEditor.SingleLine = true
	state.maxMessagesPerTargetEditor.SetText(utoa(cfg.Mailbox.MaxMessagesPerTarget))

	state.maxMessageSizeEditor.SingleLine = true
	state.maxMessageSizeEditor.SetText(utoa(cfg.Mailbox.MaxMessageSize))

	state.maxTotalBytesPerTargetEditor.SingleLine = true
	state.maxTotalBytesPerTargetEditor.SetText(utoa64(cfg.Mailbox.MaxTotalBytesPerTarget))

	state.defaultTTLEditor.SingleLine = true
	state.defaultTTLEditor.SetText(cfg.Mailbox.DefaultTTL.String())

	state.rateLimitPerMinuteEditor.SingleLine = true
	state.rateLimitPerMinuteEditor.SetText(itoa(cfg.Mailbox.RateLimitPerMinute))

	state.list.Axis = layout.Vertical
}

// ApplySettings applies the current settings to the config
func (s *SettingsState) ApplySettings(cfg *config.AppConfig) {
	// Apply network settings
	applyNetworkSettings(s.networkTab, cfg)

	// Apply privacy settings
	applyPrivacySettings(s.privacyTab, cfg)

	// Apply storage settings
	applyStorageSettings(s.storageTab, cfg)
}

// applyNetworkSettings applies network settings to config
func applyNetworkSettings(state *NetworkSettingsState, cfg *config.AppConfig) {
	// Listen addresses
	cfg.Network.ListenAddrs = splitLines(state.listenAddrsEditor.Text())

	// Bootstrap peers
	cfg.Network.BootstrapPeers = splitLines(state.bootstrapPeersEditor.Text())

	// Connection limits - parse with defaults on error
	cfg.Network.MaxConnections = parseInt(state.maxConnectionsEditor.Text(), cfg.Network.MaxConnections)
	cfg.Network.LowWater = parseInt(state.lowWaterEditor.Text(), cfg.Network.LowWater)
	cfg.Network.HighWater = parseInt(state.highWaterEditor.Text(), cfg.Network.HighWater)
	cfg.Network.MaxStoredPeers = parseInt(state.maxStoredPeersEditor.Text(), cfg.Network.MaxStoredPeers)
	cfg.Network.MinPeerConnections = parseInt(state.minPeerConnectionsEditor.Text(), cfg.Network.MinPeerConnections)

	// Timeouts - parse with defaults on error
	cfg.Network.BootstrapTimeout = parseDuration(state.bootstrapTimeoutEditor.Text(), cfg.Network.BootstrapTimeout)
	cfg.Network.ConnectionTimeout = parseDuration(state.connectionTimeoutEditor.Text(), cfg.Network.ConnectionTimeout)
	cfg.Network.DialTimeout = parseDuration(state.dialTimeoutEditor.Text(), cfg.Network.DialTimeout)
	cfg.Network.DHTBootstrapTimeout = parseDuration(state.dhtBootstrapTimeoutEditor.Text(), cfg.Network.DHTBootstrapTimeout)

	// Toggles
	cfg.Network.EnableRelay = state.enableRelay.Value
	cfg.Network.EnableHolePunching = state.enableHolePunching.Value
	cfg.Network.EnableAutoNAT = state.enableAutoNAT.Value

	// DHT mode - get from editor text
	cfg.Network.DHTMode = state.dhtModeEditor.Text()

	// Protocol ID
	cfg.Network.ProtocolID = state.protocolIDEditor.Text()
}

// applyPrivacySettings applies privacy settings to config
func applyPrivacySettings(state *PrivacySettingsState, cfg *config.AppConfig) {
	cfg.Identity.DHTPublish = state.dhtPublish.Value
	cfg.Multidevice.MaxDevices = parseInt(state.maxDevicesEditor.Text(), cfg.Multidevice.MaxDevices)
}

// applyStorageSettings applies storage settings to config
func applyStorageSettings(state *StorageSettingsState, cfg *config.AppConfig) {
	cfg.Storage.Path = state.storagePathEditor.Text()
	cfg.Storage.InMemory = state.inMemoryStorage.Value

	cfg.Mailbox.MaxMessagesPerTarget = parseUint32(state.maxMessagesPerTargetEditor.Text(), cfg.Mailbox.MaxMessagesPerTarget)
	cfg.Mailbox.MaxMessageSize = parseUint32(state.maxMessageSizeEditor.Text(), cfg.Mailbox.MaxMessageSize)
	cfg.Mailbox.MaxTotalBytesPerTarget = parseUint64(state.maxTotalBytesPerTargetEditor.Text(), cfg.Mailbox.MaxTotalBytesPerTarget)
	cfg.Mailbox.DefaultTTL = parseDuration(state.defaultTTLEditor.Text(), cfg.Mailbox.DefaultTTL)
	cfg.Mailbox.RateLimitPerMinute = parseInt(state.rateLimitPerMinuteEditor.Text(), cfg.Mailbox.RateLimitPerMinute)
}

// Helper functions
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}

func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	// Simple split by newline
	result := []string{}
	current := ""
	for _, r := range s {
		if r == '\n' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func itoa(i int) string {
	return string(rune('0' + i%10))
}

func utoa(i uint32) string {
	return string(rune('0' + i%10))
}

func utoa64(i uint64) string {
	return string(rune('0' + i%10))
}

func parseInt(s string, defaultVal int) int {
	// Simple parser - in production would use strconv
	if s == "" {
		return defaultVal
	}
	result := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return defaultVal
		}
		result = result*10 + int(r-'0')
	}
	return result
}

func parseUint32(s string, defaultVal uint32) uint32 {
	if s == "" {
		return defaultVal
	}
	result := uint32(0)
	for _, r := range s {
		if r < '0' || r > '9' {
			return defaultVal
		}
		result = result*10 + uint32(r-'0')
	}
	return result
}

func parseUint64(s string, defaultVal uint64) uint64 {
	if s == "" {
		return defaultVal
	}
	result := uint64(0)
	for _, r := range s {
		if r < '0' || r > '9' {
			return defaultVal
		}
		result = result*10 + uint64(r-'0')
	}
	return result
}

func parseDuration(s string, defaultVal time.Duration) time.Duration {
	// Simple parser - in production would use time.ParseDuration
	if s == "" {
		return defaultVal
	}
	// Try to parse as plain seconds
	val := parseInt(s, -1)
	if val > 0 {
		return time.Duration(val) * time.Second
	}
	return defaultVal
}
