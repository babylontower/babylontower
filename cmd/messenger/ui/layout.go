package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// layout renders the main UI
func (a *App) layout(gtx layout.Context) layout.Dimensions {
	// Apply theme colors
	a.applyTheme(gtx)

	// Handle events
	a.handleEvents(gtx)

	// Main layout: 3 columns
	// Settings | Contacts | Chat
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// Settings column (narrow)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			maxWidth := gtx.Dp(200)
			oldConstraints := gtx.Constraints
			gtx.Constraints.Min.X = maxWidth
			gtx.Constraints.Max.X = maxWidth
			dims := a.layoutSettingsColumn(gtx)
			gtx.Constraints = oldConstraints
			return dims
		}),

		// Divider
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			maxWidth := gtx.Dp(1)
			oldConstraints := gtx.Constraints
			gtx.Constraints.Min.X = maxWidth
			gtx.Constraints.Max.X = maxWidth
			dims := func(gtx layout.Context) layout.Dimensions {
				background := widget.Border{
					Color:        a.ui.theme.Divider,
					Width:        unit.Dp(1),
					CornerRadius: 0,
				}
				return background.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					maxHeight := gtx.Dp(unit.Dp(gtx.Constraints.Max.Y))
					oldConstraints := gtx.Constraints
					gtx.Constraints.Min.Y = maxHeight
					gtx.Constraints.Max.Y = maxHeight
					dims := layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Min.X, Y: gtx.Constraints.Min.Y}}
					}, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{}
					})
					gtx.Constraints = oldConstraints
					return dims
				})
			}(gtx)
			gtx.Constraints = oldConstraints
			return dims
		}),

		// Contacts column (medium)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			maxWidth := gtx.Dp(300)
			oldConstraints := gtx.Constraints
			gtx.Constraints.Min.X = maxWidth
			gtx.Constraints.Max.X = maxWidth
			dims := a.layoutContactsColumn(gtx)
			gtx.Constraints = oldConstraints
			return dims
		}),

		// Divider
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			maxWidth := gtx.Dp(1)
			oldConstraints := gtx.Constraints
			gtx.Constraints.Min.X = maxWidth
			gtx.Constraints.Max.X = maxWidth
			dims := func(gtx layout.Context) layout.Dimensions {
				background := widget.Border{
					Color:        a.ui.theme.Divider,
					Width:        unit.Dp(1),
					CornerRadius: 0,
				}
				return background.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					maxHeight := gtx.Dp(unit.Dp(gtx.Constraints.Max.Y))
					oldConstraints := gtx.Constraints
					gtx.Constraints.Min.Y = maxHeight
					gtx.Constraints.Max.Y = maxHeight
					dims := layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Min.X, Y: gtx.Constraints.Min.Y}}
					}, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{}
					})
					gtx.Constraints = oldConstraints
					return dims
				})
			}(gtx)
			gtx.Constraints = oldConstraints
			return dims
		}),

		// Chat column (flexible, takes remaining space)
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutChatColumn(gtx)
		}),
	)
}

// handleEvents processes UI events
func (a *App) handleEvents(gtx layout.Context) {
	// Check for send button click
	if a.ui.sendButton.Clicked(gtx) {
		if err := a.sendMessage(); err != nil {
			logger.Errorw("send message failed", "error", err)
		}
	}

	// Check for message input submit (Enter key)
	for {
		event, ok := a.ui.messageInput.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			if err := a.sendMessage(); err != nil {
				logger.Errorw("send message failed", "error", err)
			}
		}
	}

	// Check for contact selection
	for i := range a.contacts {
		if len(a.ui.contactButtons) <= i {
			a.ui.contactButtons = append(a.ui.contactButtons, widget.Clickable{})
		}
		if a.ui.contactButtons[i].Clicked(gtx) {
			a.ui.selectedContact = i
			logger.Debugw("contact selected", "index", i, "name", a.contacts[i].Name)
		}
	}

	// Handle settings panel events
	a.handleSettingsEvents(gtx)
}

// handleSettingsEvents processes settings panel events
func (a *App) handleSettingsEvents(gtx layout.Context) {
	if a.ui.settings == nil {
		return
	}

	// Check for settings tab selection
	tabs := []string{"Network", "Appearance", "Privacy", "Notifications", "Storage", "About"}
	for i := range tabs {
		if i < len(a.ui.settingsButtons) && a.ui.settingsButtons[i].Clicked(gtx) {
			a.ui.settings.currentTab = i
			a.ui.settings.isModified = false
			a.ui.settings.statusMessage = ""
		}
	}

	// Check for save button
	if a.ui.settings.saveButton.Clicked(gtx) {
		a.saveSettings()
	}

	// Check for cancel button
	if a.ui.settings.cancelButton.Clicked(gtx) {
		a.cancelSettings()
	}

	// Check for editor changes (mark as modified)
	a.checkSettingsModified(gtx)
}

// saveSettings saves the current settings
func (a *App) saveSettings() {
	if a.ui.settings == nil {
		return
	}

	// Apply settings to state
	a.ui.settings.ApplySettings(nil) // TODO: Pass actual config

	a.ui.settings.statusMessage = "Settings saved successfully!"
	a.ui.settings.isModified = false

	logger.Info("settings saved")
}

// cancelSettings cancels settings changes
func (a *App) cancelSettings() {
	if a.ui.settings == nil {
		return
	}

	// TODO: Reload settings from config

	a.ui.settings.statusMessage = "Changes cancelled"
	a.ui.settings.isModified = false

	logger.Info("settings cancelled")
}

// checkSettingsModified checks if any settings have been modified
func (a *App) checkSettingsModified(gtx layout.Context) {
	// TODO: Implement proper change detection
	// For now, just check if any editors have been updated
	if a.ui.settings != nil && !a.ui.settings.isModified {
		// Simple heuristic: if we're in settings panel, mark as modified on any interaction
		a.ui.settings.isModified = true
	}
}

// applyTheme applies the current theme colors to the material theme
func (a *App) applyTheme(gtx layout.Context) {
	a.theme.Bg = color.NRGBA(a.ui.theme.Background)
	a.theme.Fg = color.NRGBA(a.ui.theme.TextPrimary)
	a.theme.ContrastBg = color.NRGBA(a.ui.theme.Primary)
	a.theme.ContrastFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
}

// layoutSettingsColumn renders the settings column (leftmost)
func (a *App) layoutSettingsColumn(gtx layout.Context) layout.Dimensions {
	// If settings panel is shown, show full settings editor
	if a.ui.showSettingsPanel && a.ui.settings != nil {
		return a.layoutSettingsPanel(gtx)
	}

	// Otherwise show simple settings list
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.H6(a.theme, "Settings").Layout(gtx)
			})
		}),

		// Settings items
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(a.theme, &a.ui.settingsList).Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
				return a.layoutSettingsItem(gtx, index)
			})
		}),
	)
}

// layoutSettingsItem renders a single settings item
func (a *App) layoutSettingsItem(gtx layout.Context, index int) layout.Dimensions {
	settings := []string{
		"⚙️ Open Settings",
		"Profile",
		"Privacy",
		"Notifications",
		"Appearance",
		"Network",
		"Storage",
		"About",
	}

	if index >= len(settings) {
		return layout.Dimensions{}
	}

	// Ensure we have enough buttons
	for len(a.ui.settingsButtons) <= index {
		a.ui.settingsButtons = append(a.ui.settingsButtons, widget.Clickable{})
	}

	return material.Clickable(gtx, &a.ui.settingsButtons[index], func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Body1(a.theme, settings[index])
			label.Color = color.NRGBA(a.ui.theme.TextPrimary)

			// Handle "Open Settings" click
			if index == 0 && a.ui.settingsButtons[index].Clicked(gtx) {
				a.ui.showSettingsPanel = true
			}

			return label.Layout(gtx)
		})
	})
}

// layoutContactsColumn renders the contacts column (middle)
func (a *App) layoutContactsColumn(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header with search
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.H6(a.theme, "Contacts").Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							editor := material.Editor(a.theme, &a.ui.searchInput, "Search contacts...")
							editor.TextSize = unit.Sp(12)
							return editor.Layout(gtx)
						})
					}),
				)
			})
		}),

		// Contact list
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(a.theme, &a.ui.contactList).Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
				return a.layoutContactItem(gtx, index)
			})
		}),
	)
}

// layoutContactItem renders a single contact item
func (a *App) layoutContactItem(gtx layout.Context, index int) layout.Dimensions {
	if index >= len(a.contacts) {
		return layout.Dimensions{}
	}

	contact := a.contacts[index]

	// Ensure we have enough buttons
	for len(a.ui.contactButtons) <= index {
		a.ui.contactButtons = append(a.ui.contactButtons, widget.Clickable{})
	}

	isSelected := index == a.ui.selectedContact
	_ = isSelected // TODO: Use for highlighting selected contact

	statusText := "Offline"
	statusColor := color.NRGBA(a.ui.theme.TextSecondary)
	if contact.IsOnline {
		statusText = "Online"
		statusColor = color.NRGBA(a.ui.theme.Success)
	}

	displayName := contact.Name
	if contact.DisplayName != "" {
		displayName = contact.DisplayName
	}

	return material.Clickable(gtx, &a.ui.contactButtons[index], func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Body1(a.theme, displayName)
					label.Color = color.NRGBA(a.ui.theme.TextPrimary)
					return label.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Caption(a.theme, statusText)
					label.Color = statusColor
					label.TextSize = unit.Sp(10)
					return label.Layout(gtx)
				}),
			)
		})
	})
}

// layoutChatColumn renders the chat column (rightmost, largest)
func (a *App) layoutChatColumn(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Chat header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				chatTitle := "Select a contact"
				if a.ui.selectedContact >= 0 && a.ui.selectedContact < len(a.contacts) {
					contact := a.contacts[a.ui.selectedContact]
					if contact.DisplayName != "" {
						chatTitle = contact.DisplayName
					} else {
						chatTitle = contact.Name
					}
				}
				return material.H6(a.theme, chatTitle).Layout(gtx)
			})
		}),

		// Message list
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Get messages for selected contact
				if a.ui.selectedContact < 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						label := material.Body1(a.theme, "Select a contact to start chatting")
						label.Color = color.NRGBA(a.ui.theme.TextSecondary)
						return label.Layout(gtx)
					})
				}

				messages := a.messages[a.ui.selectedContact]
				if len(messages) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						label := material.Body1(a.theme, "No messages yet")
						label.Color = color.NRGBA(a.ui.theme.TextSecondary)
						return label.Layout(gtx)
					})
				}

				return material.List(a.theme, &a.ui.chatList).Layout(gtx, len(messages), func(gtx layout.Context, index int) layout.Dimensions {
					return a.layoutMessageItem(gtx, index, messages)
				})
			})
		}),

		// Message input
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Spacing: layout.SpaceEnd}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						editor := material.Editor(a.theme, &a.ui.messageInput, "Type a message...")
						return editor.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							button := material.Button(a.theme, &a.ui.sendButton, "Send")
							return button.Layout(gtx)
						})
					}),
				)
			})
		}),
	)
}

// layoutMessageItem renders a single message item
func (a *App) layoutMessageItem(gtx layout.Context, index int, messages []MessageInfo) layout.Dimensions {
	if index >= len(messages) {
		return layout.Dimensions{}
	}

	msg := messages[index]
	alignment := layout.Start
	textColor := color.NRGBA(a.ui.theme.TextPrimary)

	if msg.IsOutgoing {
		alignment = layout.End
		textColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}

	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: alignment}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					// Simple message bubble
					label := material.Body1(a.theme, msg.Text)
					label.Color = textColor
					return label.Layout(gtx)
				})
			}),
		)
	})
}
