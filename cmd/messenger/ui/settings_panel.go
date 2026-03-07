package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// layoutSettingsPanel renders the full settings editor panel
func (a *App) layoutSettingsPanel(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Settings header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.H5(a.theme, "Settings").Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Spacer{Width: unit.Dp(16)}.Layout(gtx)
					}),
					// Save button
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(a.theme, &a.ui.settings.saveButton, "Save")
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
					}),
					// Cancel button
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(a.theme, &a.ui.settings.cancelButton, "Cancel")
						return btn.Layout(gtx)
					}),
				)
			})
		}),

		// Settings tabs
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSettingsTabs(gtx)
		}),

		// Settings content
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutSettingsContent(gtx)
		}),

		// Status message
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.ui.settings.statusMessage != "" {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Caption(a.theme, a.ui.settings.statusMessage)
					if a.ui.settings.isModified {
						label.Color = color.NRGBA(a.ui.theme.Primary)
					}
					return label.Layout(gtx)
				})
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutSettingsTabs renders the settings tabs
func (a *App) layoutSettingsTabs(gtx layout.Context) layout.Dimensions {
	tabs := []string{"Network", "Appearance", "Privacy", "Notifications", "Storage", "About"}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			for i, tab := range tabs {
				i := i
				tab := tab
				selected := a.ui.settings.currentTab == i

				// Ensure we have enough buttons
				for len(a.ui.settingsButtons) <= i {
					a.ui.settingsButtons = append(a.ui.settingsButtons, widget.Clickable{})
				}

				btn := material.Button(a.theme, &a.ui.settingsButtons[i], tab)
				if selected {
					btn.Background = color.NRGBA(a.ui.theme.Primary)
					btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				} else {
					btn.Background = color.NRGBA(a.ui.theme.Surface)
					btn.Color = color.NRGBA(a.ui.theme.TextPrimary)
				}

				result := layout.Inset{
					Left:   unit.Dp(4),
					Right:  unit.Dp(4),
					Top:    unit.Dp(4),
					Bottom: unit.Dp(4),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return btn.Layout(gtx)
				})

				if i < len(tabs)-1 {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return result
						}),
					)
				}
				return result
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutSettingsContent renders the content for the current settings tab
func (a *App) layoutSettingsContent(gtx layout.Context) layout.Dimensions {
	switch a.ui.settings.currentTab {
	case 0:
		return a.layoutNetworkSettings(gtx)
	case 1:
		return a.layoutAppearanceSettings(gtx)
	case 2:
		return a.layoutPrivacySettings(gtx)
	case 3:
		return a.layoutNotificationsSettings(gtx)
	case 4:
		return a.layoutStorageSettings(gtx)
	case 5:
		return a.layoutAboutSettings(gtx)
	default:
		return layout.Dimensions{}
	}
}

// layoutNetworkSettings renders network settings
func (a *App) layoutNetworkSettings(gtx layout.Context) layout.Dimensions {
	state := a.ui.settings.networkTab

	return material.List(a.theme, &state.list).Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				// Listen Addresses
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Listen Addresses", func(gtx layout.Context) layout.Dimensions {
						editor := material.Editor(a.theme, &state.listenAddrsEditor, "Enter listen addresses (one per line)")
						return editor.Layout(gtx)
					})
				}),

				// Bootstrap Peers
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Bootstrap Peers", func(gtx layout.Context) layout.Dimensions {
						editor := material.Editor(a.theme, &state.bootstrapPeersEditor, "Enter bootstrap peers (one per line)")
						return editor.Layout(gtx)
					})
				}),

				// Connection Limits
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Connection Limits", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Max Connections", &state.maxConnectionsEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Low Water", &state.lowWaterEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "High Water", &state.highWaterEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Max Stored Peers", &state.maxStoredPeersEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Min Peer Connections", &state.minPeerConnectionsEditor)
							}),
						)
					})
				}),

				// Timeouts
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Timeouts", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Bootstrap Timeout", &state.bootstrapTimeoutEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Connection Timeout", &state.connectionTimeoutEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Dial Timeout", &state.dialTimeoutEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "DHT Bootstrap Timeout", &state.dhtBootstrapTimeoutEditor)
							}),
						)
					})
				}),

				// Toggles
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "NAT Traversal", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsSwitch(gtx, a.theme, "Enable Relay", &state.enableRelay)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsSwitch(gtx, a.theme, "Enable Hole Punching", &state.enableHolePunching)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsSwitch(gtx, a.theme, "Enable AutoNAT", &state.enableAutoNAT)
							}),
						)
					})
				}),

				// DHT Mode
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "DHT Mode", func(gtx layout.Context) layout.Dimensions {
						return layoutSettingsField(gtx, a.theme, "Mode (auto/server/client)", &state.dhtModeEditor)
					})
				}),

				// Protocol ID
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Protocol ID", func(gtx layout.Context) layout.Dimensions {
						editor := material.Editor(a.theme, &state.protocolIDEditor, "Protocol ID")
						return editor.Layout(gtx)
					})
				}),
			)
		})
	})
}

// layoutAppearanceSettings renders appearance settings
func (a *App) layoutAppearanceSettings(gtx layout.Context) layout.Dimensions {
	state := a.ui.settings.appearanceTab

	return material.List(a.theme, &state.list).Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				// Theme
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Theme", func(gtx layout.Context) layout.Dimensions {
						return layoutSettingsSwitch(gtx, a.theme, "Dark Mode", &state.darkMode)
					})
				}),

				// Font Size
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Font Size", func(gtx layout.Context) layout.Dimensions {
						return layoutSettingsField(gtx, a.theme, "Font Size", &state.fontSizeEditor)
					})
				}),

				// Window Size
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Window Size", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Width", &state.windowWidthEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Height", &state.windowHeightEditor)
							}),
						)
					})
				}),
			)
		})
	})
}

// layoutPrivacySettings renders privacy settings
func (a *App) layoutPrivacySettings(gtx layout.Context) layout.Dimensions {
	state := a.ui.settings.privacyTab

	return material.List(a.theme, &state.list).Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				// Identity Visibility
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Identity", func(gtx layout.Context) layout.Dimensions {
						return layoutSettingsSwitch(gtx, a.theme, "Publish to DHT", &state.dhtPublish)
					})
				}),

				// Multi-device
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Multi-Device", func(gtx layout.Context) layout.Dimensions {
						return layoutSettingsField(gtx, a.theme, "Max Devices", &state.maxDevicesEditor)
					})
				}),
			)
		})
	})
}

// layoutNotificationsSettings renders notifications settings
func (a *App) layoutNotificationsSettings(gtx layout.Context) layout.Dimensions {
	state := a.ui.settings.notificationsTab

	return material.List(a.theme, &state.list).Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				// Sound & Alerts
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Sound & Alerts", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsSwitch(gtx, a.theme, "Sound Notifications", &state.enableSoundNotifications)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsSwitch(gtx, a.theme, "Desktop Alerts", &state.enableDesktopAlerts)
							}),
						)
					})
				}),

				// Message Notifications
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Message Notifications", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsSwitch(gtx, a.theme, "Notify on Mention", &state.notifyOnMention)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsSwitch(gtx, a.theme, "Notify on Message", &state.notifyOnMessage)
							}),
						)
					})
				}),

				// Call Notifications
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Call Notifications", func(gtx layout.Context) layout.Dimensions {
						return layoutSettingsSwitch(gtx, a.theme, "Notify on Call", &state.notifyOnCall)
					})
				}),
			)
		})
	})
}

// layoutStorageSettings renders storage settings
func (a *App) layoutStorageSettings(gtx layout.Context) layout.Dimensions {
	state := a.ui.settings.storageTab

	return material.List(a.theme, &state.list).Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				// Storage Path
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Storage", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Storage Path", &state.storagePathEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsSwitch(gtx, a.theme, "In-Memory Storage", &state.inMemoryStorage)
							}),
						)
					})
				}),

				// Mailbox Limits
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Mailbox Limits", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Max Messages Per Target", &state.maxMessagesPerTargetEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Max Message Size", &state.maxMessageSizeEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Max Total Bytes Per Target", &state.maxTotalBytesPerTargetEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Default TTL", &state.defaultTTLEditor)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutSettingsField(gtx, a.theme, "Rate Limit Per Minute", &state.rateLimitPerMinuteEditor)
							}),
						)
					})
				}),
			)
		})
	})
}

// layoutAboutSettings renders about section
func (a *App) layoutAboutSettings(gtx layout.Context) layout.Dimensions {
	state := a.ui.settings.aboutTab

	return material.List(a.theme, &state.list).Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				// Version Info
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutSettingsSection(gtx, a.theme, "Babylon Tower", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									label := material.Body1(a.theme, "Decentralized P2P Messenger")
									return label.Layout(gtx)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									label := material.Caption(a.theme, "Version: "+state.version)
									label.Color = color.NRGBA(a.ui.theme.TextSecondary)
									return label.Layout(gtx)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									label := material.Caption(a.theme, "Build Time: "+state.buildTime)
									label.Color = color.NRGBA(a.ui.theme.TextSecondary)
									return label.Layout(gtx)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := material.Caption(a.theme, "Git Commit: "+state.gitCommit)
								label.Color = color.NRGBA(a.ui.theme.TextSecondary)
								return label.Layout(gtx)
							}),
						)
					})
				}),
			)
		})
	})
}

// Helper functions for settings layout

// layoutSettingsSection renders a settings section with title and content
func layoutSettingsSection(gtx layout.Context, theme *material.Theme, title string, content layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(theme, title)
				label.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
				label.Font.Weight = 700
				return label.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return content(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Spacer{Height: unit.Dp(16)}.Layout(gtx)
		}),
	)
}

// layoutSettingsField renders a settings field with label and editor
func layoutSettingsField(gtx layout.Context, theme *material.Theme, label string, editor *widget.Editor) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(theme, label)
					lbl.Color = color.NRGBA(theme.Fg)
					return lbl.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				editor := material.Editor(theme, editor, "")
				editor.TextSize = unit.Sp(12)
				return editor.Layout(gtx)
			}),
		)
	})
}

// layoutSettingsSwitch renders a settings switch/toggle
func layoutSettingsSwitch(gtx layout.Context, theme *material.Theme, label string, cb *widget.Bool) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				sw := material.Switch(theme, cb, label)
				return sw.Layout(gtx)
			}),
		)
	})
}
