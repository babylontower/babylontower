package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"babylontower/pkg/config"
)

// SettingsTab identifies a settings tab.
type SettingsTab int

const (
	TabProfile SettingsTab = iota
	TabNetwork
	TabAppearance
	TabPrivacy
	TabStorage
	TabAbout
	tabCount
)

// SettingsEvent is returned by the settings screen when a user action occurs.
type SettingsEvent int

const (
	SettingsNone   SettingsEvent = iota
	SettingsSaved                // User clicked Save
	SettingsClosed               // User clicked Close/Cancel
)

// SettingsColors defines the color scheme for the settings screen.
// This allows the screen to work in both onboarding and main app contexts.
type SettingsColors struct {
	Background    color.NRGBA
	Surface       color.NRGBA
	SurfaceBorder color.NRGBA
	Primary       color.NRGBA
	Accent        color.NRGBA
	TextTitle     color.NRGBA
	TextBody      color.NRGBA
	TextMuted     color.NRGBA
	TextOnPrimary color.NRGBA
	Error         color.NRGBA
	Divider       color.NRGBA
}

// DarkSettingsColors returns settings colors for the dark Babylonian theme.
func DarkSettingsColors() SettingsColors {
	return SettingsColors{
		Background:    color.NRGBA{R: 20, G: 16, B: 12, A: 255},
		Surface:       color.NRGBA{R: 30, G: 26, B: 22, A: 230},
		SurfaceBorder: color.NRGBA{R: 197, G: 150, B: 26, A: 60},
		Primary:       color.NRGBA{R: 30, G: 58, B: 95, A: 255},
		Accent:        color.NRGBA{R: 197, G: 150, B: 26, A: 255},
		TextTitle:     color.NRGBA{R: 232, G: 213, B: 183, A: 255},
		TextBody:      color.NRGBA{R: 200, G: 185, B: 165, A: 255},
		TextMuted:     color.NRGBA{R: 139, G: 125, B: 107, A: 255},
		TextOnPrimary: color.NRGBA{R: 232, G: 213, B: 183, A: 255},
		Error:         color.NRGBA{R: 207, G: 107, B: 90, A: 255},
		Divider:       color.NRGBA{R: 60, G: 50, B: 38, A: 255},
	}
}

// LightSettingsColors returns settings colors for the light Babylonian theme.
func LightSettingsColors() SettingsColors {
	return SettingsColors{
		Background:    color.NRGBA{R: 242, G: 232, B: 213, A: 255},
		Surface:       color.NRGBA{R: 255, G: 248, B: 235, A: 255},
		SurfaceBorder: color.NRGBA{R: 189, G: 168, B: 142, A: 100},
		Primary:       color.NRGBA{R: 30, G: 58, B: 95, A: 255},
		Accent:        color.NRGBA{R: 160, G: 120, B: 20, A: 255},
		TextTitle:     color.NRGBA{R: 44, G: 24, B: 16, A: 255},
		TextBody:      color.NRGBA{R: 74, G: 54, B: 36, A: 255},
		TextMuted:     color.NRGBA{R: 107, G: 91, B: 79, A: 255},
		TextOnPrimary: color.NRGBA{R: 232, G: 213, B: 183, A: 255},
		Error:         color.NRGBA{R: 181, G: 69, B: 58, A: 255},
		Divider:       color.NRGBA{R: 189, G: 168, B: 142, A: 255},
	}
}

// SettingsScreen is a reusable, self-contained settings UI widget.
type SettingsScreen struct {
	Colors SettingsColors

	// Active tab
	activeTab SettingsTab

	// Tab buttons
	tabButtons [tabCount]widget.Clickable

	// Action buttons
	saveBtn  widget.Clickable
	closeBtn widget.Clickable

	// Scroll for content area
	contentList widget.List

	// Status
	statusMsg    string
	originalCfg  *config.AppConfig // snapshot for detecting network changes

	// ── Profile tab ──
	DisplayNameEditor widget.Editor
	DeviceNameEditor  widget.Editor

	// ── Network tab ──
	listenAddrsEditor         widget.Editor
	bootstrapPeersEditor      widget.Editor
	maxConnectionsEditor      widget.Editor
	lowWaterEditor            widget.Editor
	highWaterEditor           widget.Editor
	maxStoredPeersEditor      widget.Editor
	minPeerConnectionsEditor  widget.Editor
	bootstrapTimeoutEditor    widget.Editor
	connectionTimeoutEditor   widget.Editor
	dialTimeoutEditor         widget.Editor
	dhtBootstrapTimeoutEditor widget.Editor
	enableRelay               widget.Bool
	enableHolePunching        widget.Bool
	enableAutoNAT             widget.Bool
	dhtModeEditor             widget.Editor
	protocolIDEditor          widget.Editor

	// ── Appearance tab ──
	darkMode            widget.Bool
	fontSizeEditor      widget.Editor
	windowWidthEditor   widget.Editor
	windowHeightEditor  widget.Editor

	// ── Privacy tab ──
	dhtPublish          widget.Bool
	sendReadReceipts    widget.Bool
	sendTypingIndicators widget.Bool
	maxDevicesEditor    widget.Editor

	// ── Storage tab ──
	storagePathEditor              widget.Editor
	inMemoryStorage                widget.Bool
	maxMessagesPerTargetEditor     widget.Editor
	maxMessageSizeEditor           widget.Editor
	maxTotalBytesPerTargetEditor   widget.Editor
	defaultTTLEditor               widget.Editor
	rateLimitPerMinuteEditor       widget.Editor

	// ── About tab ──
	version   string
	buildTime string
	gitCommit string
}

// NewSettingsScreen creates a new settings screen, optionally pre-populated from config.
func NewSettingsScreen(cfg *config.AppConfig, colors SettingsColors) *SettingsScreen {
	s := &SettingsScreen{
		Colors:  colors,
		version: "0.1.0-ui-alpha",
	}
	s.contentList.Axis = layout.Vertical

	// Profile
	s.DisplayNameEditor.SingleLine = true
	s.DisplayNameEditor.Submit = true
	s.DeviceNameEditor.SingleLine = true
	s.DeviceNameEditor.Submit = true

	// Network editors
	s.listenAddrsEditor.SingleLine = false
	s.bootstrapPeersEditor.SingleLine = false
	s.maxConnectionsEditor.SingleLine = true
	s.lowWaterEditor.SingleLine = true
	s.highWaterEditor.SingleLine = true
	s.maxStoredPeersEditor.SingleLine = true
	s.minPeerConnectionsEditor.SingleLine = true
	s.bootstrapTimeoutEditor.SingleLine = true
	s.connectionTimeoutEditor.SingleLine = true
	s.dialTimeoutEditor.SingleLine = true
	s.dhtBootstrapTimeoutEditor.SingleLine = true
	s.dhtModeEditor.SingleLine = true
	s.protocolIDEditor.SingleLine = true

	// Appearance editors
	s.fontSizeEditor.SingleLine = true
	s.windowWidthEditor.SingleLine = true
	s.windowHeightEditor.SingleLine = true

	// Privacy editors
	s.maxDevicesEditor.SingleLine = true

	// Storage editors
	s.storagePathEditor.SingleLine = true
	s.maxMessagesPerTargetEditor.SingleLine = true
	s.maxMessageSizeEditor.SingleLine = true
	s.maxTotalBytesPerTargetEditor.SingleLine = true
	s.defaultTTLEditor.SingleLine = true
	s.rateLimitPerMinuteEditor.SingleLine = true

	if cfg != nil {
		s.originalCfg = cfg
		s.loadFromConfig(cfg)
	} else {
		s.originalCfg = config.DefaultAppConfig()
		s.loadFromConfig(s.originalCfg)
	}

	return s
}

func (s *SettingsScreen) loadFromConfig(cfg *config.AppConfig) {
	// Profile
	s.DisplayNameEditor.SetText(cfg.Profile.DisplayName)
	s.DeviceNameEditor.SetText(cfg.Profile.DeviceName)

	// Network
	s.listenAddrsEditor.SetText(strings.Join(cfg.Network.ListenAddrs, "\n"))
	s.bootstrapPeersEditor.SetText(strings.Join(cfg.Network.BootstrapPeers, "\n"))
	s.maxConnectionsEditor.SetText(strconv.Itoa(cfg.Network.MaxConnections))
	s.lowWaterEditor.SetText(strconv.Itoa(cfg.Network.LowWater))
	s.highWaterEditor.SetText(strconv.Itoa(cfg.Network.HighWater))
	s.maxStoredPeersEditor.SetText(strconv.Itoa(cfg.Network.MaxStoredPeers))
	s.minPeerConnectionsEditor.SetText(strconv.Itoa(cfg.Network.MinPeerConnections))
	s.bootstrapTimeoutEditor.SetText(cfg.Network.BootstrapTimeout.String())
	s.connectionTimeoutEditor.SetText(cfg.Network.ConnectionTimeout.String())
	s.dialTimeoutEditor.SetText(cfg.Network.DialTimeout.String())
	s.dhtBootstrapTimeoutEditor.SetText(cfg.Network.DHTBootstrapTimeout.String())
	s.enableRelay.Value = cfg.Network.EnableRelay
	s.enableHolePunching.Value = cfg.Network.EnableHolePunching
	s.enableAutoNAT.Value = cfg.Network.EnableAutoNAT
	s.dhtModeEditor.SetText(cfg.Network.DHTMode)
	s.protocolIDEditor.SetText(cfg.Network.ProtocolID)

	// Appearance
	s.darkMode.Value = cfg.Appearance.DarkMode
	s.fontSizeEditor.SetText(strconv.Itoa(cfg.Appearance.FontSize))
	s.windowWidthEditor.SetText(strconv.Itoa(cfg.Appearance.WindowWidth))
	s.windowHeightEditor.SetText(strconv.Itoa(cfg.Appearance.WindowHeight))

	// Privacy
	s.dhtPublish.Value = cfg.Identity.DHTPublish
	s.sendReadReceipts.Value = cfg.Privacy.SendReadReceipts
	s.sendTypingIndicators.Value = cfg.Privacy.SendTypingIndicators
	s.maxDevicesEditor.SetText(strconv.Itoa(cfg.Multidevice.MaxDevices))

	// Storage
	s.storagePathEditor.SetText(cfg.Storage.Path)
	s.inMemoryStorage.Value = cfg.Storage.InMemory
	s.maxMessagesPerTargetEditor.SetText(fmt.Sprintf("%d", cfg.Mailbox.MaxMessagesPerTarget))
	s.maxMessageSizeEditor.SetText(fmt.Sprintf("%d", cfg.Mailbox.MaxMessageSize))
	s.maxTotalBytesPerTargetEditor.SetText(fmt.Sprintf("%d", cfg.Mailbox.MaxTotalBytesPerTarget))
	s.defaultTTLEditor.SetText(cfg.Mailbox.DefaultTTL.String())
	s.rateLimitPerMinuteEditor.SetText(strconv.Itoa(cfg.Mailbox.RateLimitPerMinute))
}

// ApplyToConfig writes the current editor values back to a config struct.
func (s *SettingsScreen) ApplyToConfig(cfg *config.AppConfig) {
	// Profile
	cfg.Profile.DisplayName = strings.TrimSpace(s.DisplayNameEditor.Text())
	cfg.Profile.DeviceName = strings.TrimSpace(s.DeviceNameEditor.Text())

	// Network
	cfg.Network.ListenAddrs = splitNonEmpty(s.listenAddrsEditor.Text())
	cfg.Network.BootstrapPeers = splitNonEmpty(s.bootstrapPeersEditor.Text())
	cfg.Network.MaxConnections = parseIntOr(s.maxConnectionsEditor.Text(), cfg.Network.MaxConnections)
	cfg.Network.LowWater = parseIntOr(s.lowWaterEditor.Text(), cfg.Network.LowWater)
	cfg.Network.HighWater = parseIntOr(s.highWaterEditor.Text(), cfg.Network.HighWater)
	cfg.Network.MaxStoredPeers = parseIntOr(s.maxStoredPeersEditor.Text(), cfg.Network.MaxStoredPeers)
	cfg.Network.MinPeerConnections = parseIntOr(s.minPeerConnectionsEditor.Text(), cfg.Network.MinPeerConnections)
	cfg.Network.BootstrapTimeout = parseDurationOr(s.bootstrapTimeoutEditor.Text(), cfg.Network.BootstrapTimeout)
	cfg.Network.ConnectionTimeout = parseDurationOr(s.connectionTimeoutEditor.Text(), cfg.Network.ConnectionTimeout)
	cfg.Network.DialTimeout = parseDurationOr(s.dialTimeoutEditor.Text(), cfg.Network.DialTimeout)
	cfg.Network.DHTBootstrapTimeout = parseDurationOr(s.dhtBootstrapTimeoutEditor.Text(), cfg.Network.DHTBootstrapTimeout)
	cfg.Network.EnableRelay = s.enableRelay.Value
	cfg.Network.EnableHolePunching = s.enableHolePunching.Value
	cfg.Network.EnableAutoNAT = s.enableAutoNAT.Value
	cfg.Network.DHTMode = strings.TrimSpace(s.dhtModeEditor.Text())
	cfg.Network.ProtocolID = strings.TrimSpace(s.protocolIDEditor.Text())

	// Privacy
	cfg.Identity.DHTPublish = s.dhtPublish.Value
	cfg.Privacy.SendReadReceipts = s.sendReadReceipts.Value
	cfg.Privacy.SendTypingIndicators = s.sendTypingIndicators.Value
	cfg.Multidevice.MaxDevices = parseIntOr(s.maxDevicesEditor.Text(), cfg.Multidevice.MaxDevices)

	// Appearance
	cfg.Appearance.DarkMode = s.darkMode.Value
	cfg.Appearance.FontSize = parseIntOr(s.fontSizeEditor.Text(), cfg.Appearance.FontSize)
	cfg.Appearance.WindowWidth = parseIntOr(s.windowWidthEditor.Text(), cfg.Appearance.WindowWidth)
	cfg.Appearance.WindowHeight = parseIntOr(s.windowHeightEditor.Text(), cfg.Appearance.WindowHeight)

	// Storage
	cfg.Storage.Path = strings.TrimSpace(s.storagePathEditor.Text())
	cfg.Storage.InMemory = s.inMemoryStorage.Value
	cfg.Mailbox.MaxMessagesPerTarget = parseUint32Or(s.maxMessagesPerTargetEditor.Text(), cfg.Mailbox.MaxMessagesPerTarget)
	cfg.Mailbox.MaxMessageSize = parseUint32Or(s.maxMessageSizeEditor.Text(), cfg.Mailbox.MaxMessageSize)
	cfg.Mailbox.MaxTotalBytesPerTarget = parseUint64Or(s.maxTotalBytesPerTargetEditor.Text(), cfg.Mailbox.MaxTotalBytesPerTarget)
	cfg.Mailbox.DefaultTTL = parseDurationOr(s.defaultTTLEditor.Text(), cfg.Mailbox.DefaultTTL)
	cfg.Mailbox.RateLimitPerMinute = parseIntOr(s.rateLimitPerMinuteEditor.Text(), cfg.Mailbox.RateLimitPerMinute)
}

// HasNetworkChanges returns true if network settings differ from the provided config.
// Used to show the "restart required" indicator.
func (s *SettingsScreen) HasNetworkChanges(original *config.AppConfig) bool {
	if original == nil {
		return false
	}
	// Check listen addresses
	newAddrs := strings.Join(splitNonEmpty(s.listenAddrsEditor.Text()), "\n")
	oldAddrs := strings.Join(original.Network.ListenAddrs, "\n")
	if newAddrs != oldAddrs {
		return true
	}
	// Check bootstrap peers
	newPeers := strings.Join(splitNonEmpty(s.bootstrapPeersEditor.Text()), "\n")
	oldPeers := strings.Join(original.Network.BootstrapPeers, "\n")
	if newPeers != oldPeers {
		return true
	}
	// Check connection limits
	if parseIntOr(s.maxConnectionsEditor.Text(), 0) != original.Network.MaxConnections {
		return true
	}
	// Check NAT settings
	if s.enableRelay.Value != original.Network.EnableRelay {
		return true
	}
	if s.enableHolePunching.Value != original.Network.EnableHolePunching {
		return true
	}
	if s.enableAutoNAT.Value != original.Network.EnableAutoNAT {
		return true
	}
	// Check DHT mode
	if strings.TrimSpace(s.dhtModeEditor.Text()) != original.Network.DHTMode {
		return true
	}
	return false
}

// Validate runs config validation and returns an error message, or "" if valid.
func (s *SettingsScreen) Validate() string {
	// Build a temporary config to validate
	cfg := config.DefaultAppConfig()
	s.ApplyToConfig(cfg)
	if err := config.ValidateAppConfig(cfg); err != nil {
		return err.Error()
	}
	return ""
}

// GetProfile returns just the profile values (useful for onboarding).
func (s *SettingsScreen) GetProfile() config.ProfileConfig {
	return config.ProfileConfig{
		DisplayName: strings.TrimSpace(s.DisplayNameEditor.Text()),
		DeviceName:  strings.TrimSpace(s.DeviceNameEditor.Text()),
	}
}

// SetVersion sets the version info for the About tab.
func (s *SettingsScreen) SetVersion(version, buildTime, gitCommit string) {
	s.version = version
	s.buildTime = buildTime
	s.gitCommit = gitCommit
}

// SetActiveTab selects the active tab.
func (s *SettingsScreen) SetActiveTab(tab SettingsTab) {
	s.activeTab = tab
}

// Update handles click events and returns any settings event.
func (s *SettingsScreen) Update(gtx layout.Context) SettingsEvent {
	// Tab clicks
	for i := range s.tabButtons {
		if s.tabButtons[i].Clicked(gtx) {
			s.activeTab = SettingsTab(i)
			s.statusMsg = ""
		}
	}

	if s.saveBtn.Clicked(gtx) {
		if errMsg := s.Validate(); errMsg != "" {
			s.statusMsg = "Error: " + errMsg
			return SettingsNone
		}
		s.statusMsg = "Settings saved"
		return SettingsSaved
	}
	if s.closeBtn.Clicked(gtx) {
		return SettingsClosed
	}
	return SettingsNone
}

// Layout renders the full settings screen.
func (s *SettingsScreen) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	c := s.Colors

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// ── Header ──
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutHeader(gtx, th)
		}),

		// ── Tab bar ──
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutTabBar(gtx, th)
		}),

		// ── Divider ──
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(1)
			paint.FillShape(gtx.Ops, c.Divider, clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X, h)).Op())
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
		}),

		// ── Content ──
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutContent(gtx, th)
		}),

		// ── Status bar ──
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			hasStatus := s.statusMsg != ""
			hasNetworkChanges := s.HasNetworkChanges(s.originalCfg)
			if !hasStatus && !hasNetworkChanges {
				return layout.Dimensions{}
			}
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !hasStatus {
							return layout.Dimensions{}
						}
						l := material.Caption(th, s.statusMsg)
						if strings.HasPrefix(s.statusMsg, "Error:") {
							l.Color = c.Error
						} else {
							l.Color = c.TextMuted
						}
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !hasNetworkChanges {
							return layout.Dimensions{}
						}
						l := material.Caption(th, "Network changes require restart to take effect")
						l.Color = c.Accent
						return l.Layout(gtx)
					}),
				)
			})
		}),
	)
}

func (s *SettingsScreen) layoutHeader(gtx layout.Context, th *material.Theme) layout.Dimensions {
	c := s.Colors
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.H5(th, "Settings")
				l.Color = c.Accent
				l.Font.Weight = font.Bold
				return l.Layout(gtx)
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(th, &s.saveBtn, "Save")
				btn.Background = c.Primary
				btn.Color = c.TextOnPrimary
				btn.CornerRadius = unit.Dp(6)
				btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(20), Right: unit.Dp(20)}
				return btn.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(th, &s.closeBtn, "Close")
				btn.Background = c.Surface
				btn.Color = c.TextBody
				btn.CornerRadius = unit.Dp(6)
				btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(20), Right: unit.Dp(20)}
				return btn.Layout(gtx)
			}),
		)
	})
}

func (s *SettingsScreen) layoutTabBar(gtx layout.Context, th *material.Theme) layout.Dimensions {
	c := s.Colors
	tabNames := [tabCount]string{"Profile", "Network", "Appearance", "Privacy", "Storage", "About"}

	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		for i := 0; i < int(tabCount); i++ {
			idx := i
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &s.tabButtons[idx], tabNames[idx])
					btn.TextSize = unit.Sp(13)
					btn.CornerRadius = unit.Dp(4)
					btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}
					if SettingsTab(idx) == s.activeTab {
						btn.Background = c.Primary
						btn.Color = c.TextOnPrimary
					} else {
						btn.Background = c.Surface
						btn.Color = c.TextMuted
					}
					return btn.Layout(gtx)
				})
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	})
}

func (s *SettingsScreen) layoutContent(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return material.List(th, &s.contentList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			switch s.activeTab {
			case TabProfile:
				return s.layoutProfileTab(gtx, th)
			case TabNetwork:
				return s.layoutNetworkTab(gtx, th)
			case TabAppearance:
				return s.layoutAppearanceTab(gtx, th)
			case TabPrivacy:
				return s.layoutPrivacyTab(gtx, th)
			case TabStorage:
				return s.layoutStorageTab(gtx, th)
			case TabAbout:
				return s.layoutAboutTab(gtx, th)
			}
			return layout.Dimensions{}
		})
	})
}

// ── Tab Layouts ──────────────────────────────────────────────────────────────

func (s *SettingsScreen) layoutProfileTab(gtx layout.Context, th *material.Theme) layout.Dimensions {
	c := s.Colors
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Your Profile")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(th, "Set your display name so others can recognize you in chats.")
				l.Color = c.TextMuted
				return l.Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Display Name", &s.DisplayNameEditor, "Enter your name...")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Device Name", &s.DeviceNameEditor, "e.g. My Laptop")
		}),
	)
}

func (s *SettingsScreen) layoutNetworkTab(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Listen Addresses")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutMultilineField(gtx, th, &s.listenAddrsEditor, "One address per line")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Bootstrap Peers")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutMultilineField(gtx, th, &s.bootstrapPeersEditor, "One peer per line")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Connection Limits")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Max Connections", &s.maxConnectionsEditor, "")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Low Water", &s.lowWaterEditor, "")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "High Water", &s.highWaterEditor, "")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Max Stored Peers", &s.maxStoredPeersEditor, "")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Min Peer Connections", &s.minPeerConnectionsEditor, "")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Timeouts")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Bootstrap Timeout", &s.bootstrapTimeoutEditor, "e.g. 60s")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Connection Timeout", &s.connectionTimeoutEditor, "e.g. 30s")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Dial Timeout", &s.dialTimeoutEditor, "e.g. 15s")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "DHT Bootstrap Timeout", &s.dhtBootstrapTimeoutEditor, "e.g. 60s")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "NAT Traversal")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSwitch(gtx, th, "Enable Relay", &s.enableRelay)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSwitch(gtx, th, "Enable Hole Punching", &s.enableHolePunching)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSwitch(gtx, th, "Enable AutoNAT", &s.enableAutoNAT)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Protocol")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "DHT Mode (auto/client/server)", &s.dhtModeEditor, "auto")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Protocol ID", &s.protocolIDEditor, "")
		}),
	)
}

func (s *SettingsScreen) layoutAppearanceTab(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Theme")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSwitch(gtx, th, "Dark Mode", &s.darkMode)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Display")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Font Size", &s.fontSizeEditor, "14")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Window Width", &s.windowWidthEditor, "1200")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Window Height", &s.windowHeightEditor, "800")
		}),
	)
}

func (s *SettingsScreen) layoutPrivacyTab(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Messaging Privacy")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSwitch(gtx, th, "Send read receipts", &s.sendReadReceipts)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSwitch(gtx, th, "Send typing indicators", &s.sendTypingIndicators)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Identity")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSwitch(gtx, th, "Publish to DHT", &s.dhtPublish)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Multi-Device")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Max Devices", &s.maxDevicesEditor, "5")
		}),
	)
}

func (s *SettingsScreen) layoutStorageTab(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Storage")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Storage Path", &s.storagePathEditor, "")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSwitch(gtx, th, "In-Memory Storage (testing only)", &s.inMemoryStorage)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Mailbox Limits")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Max Messages Per Target", &s.maxMessagesPerTargetEditor, "500")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Max Message Size (bytes)", &s.maxMessageSizeEditor, "262144")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Max Total Bytes Per Target", &s.maxTotalBytesPerTargetEditor, "67108864")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Default TTL", &s.defaultTTLEditor, "168h")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutField(gtx, th, "Rate Limit Per Minute", &s.rateLimitPerMinuteEditor, "60")
		}),
	)
}

func (s *SettingsScreen) layoutAboutTab(gtx layout.Context, th *material.Theme) layout.Dimensions {
	c := s.Colors
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSectionTitle(gtx, th, "Babylon Tower")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Body1(th, "Decentralized P2P Messenger")
			l.Color = c.TextBody
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutInfoLine(gtx, th, "Version", s.version)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutInfoLine(gtx, th, "Build Time", s.buildTime)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutInfoLine(gtx, th, "Git Commit", s.gitCommit)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Caption(th, "End-to-end encrypted  //  Peer-to-peer  //  No servers")
			l.Color = c.TextMuted
			l.Alignment = text.Middle
			return l.Layout(gtx)
		}),
	)
}

// ── Field Primitives ─────────────────────────────────────────────────────────

func (s *SettingsScreen) layoutSectionTitle(gtx layout.Context, th *material.Theme, title string) layout.Dimensions {
	c := s.Colors
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.H6(th, title)
		l.Color = c.Accent
		l.Font.Weight = font.Bold
		return l.Layout(gtx)
	})
}

func (s *SettingsScreen) layoutField(gtx layout.Context, th *material.Theme, label string, editor *widget.Editor, hint string) layout.Dimensions {
	c := s.Colors
	return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(th, label)
				l.Color = c.TextMuted
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutFieldCard(gtx, th, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(th, editor, hint)
					ed.Color = c.TextTitle
					ed.HintColor = c.TextMuted
					ed.TextSize = unit.Sp(14)
					return ed.Layout(gtx)
				})
			}),
		)
	})
}

func (s *SettingsScreen) layoutMultilineField(gtx layout.Context, th *material.Theme, editor *widget.Editor, hint string) layout.Dimensions {
	c := s.Colors
	return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.layoutFieldCard(gtx, th, func(gtx layout.Context) layout.Dimensions {
			ed := material.Editor(th, editor, hint)
			ed.Color = c.TextTitle
			ed.HintColor = c.TextMuted
			ed.TextSize = unit.Sp(13)
			return ed.Layout(gtx)
		})
	})
}

func (s *SettingsScreen) layoutSwitch(gtx layout.Context, th *material.Theme, label string, cb *widget.Bool) layout.Dimensions {
	c := s.Colors
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				sw := material.Switch(th, cb, label)
				return sw.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(th, label)
				l.Color = c.TextBody
				return l.Layout(gtx)
			}),
		)
	})
}

func (s *SettingsScreen) layoutInfoLine(gtx layout.Context, th *material.Theme, label, value string) layout.Dimensions {
	c := s.Colors
	return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(th, label+": ")
				l.Color = c.TextMuted
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(th, value)
				l.Color = c.TextBody
				return l.Layout(gtx)
			}),
		)
	})
}

func (s *SettingsScreen) layoutFieldCard(gtx layout.Context, th *material.Theme, content layout.Widget) layout.Dimensions {
	c := s.Colors
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			rr := gtx.Dp(unit.Dp(6))
			rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
			paint.FillShape(gtx.Ops, c.Surface, rect.Op(gtx.Ops))
			borderRect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
			paint.FillShape(gtx.Ops, c.SurfaceBorder, clip.Stroke{Path: borderRect.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: sz}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, content)
		}),
	)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func splitNonEmpty(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func parseIntOr(s string, fallback int) int {
	s = strings.TrimSpace(s)
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func parseUint32Or(s string, fallback uint32) uint32 {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return fallback
	}
	return uint32(v)
}

func parseUint64Or(s string, fallback uint64) uint64 {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func parseDurationOr(s string, fallback time.Duration) time.Duration {
	s = strings.TrimSpace(s)
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
