package ui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"strings"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	babylonapp "babylontower/pkg/app"
	"babylontower/pkg/config"
)

// ── Main Layout ─────────────────────────────────────────────────────────────

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	a.applyTheme(gtx)
	a.handleEvents(gtx)

	a.drawBackground(gtx)

	// Overlay dialogs on top of main UI
	if a.ui.showAddContact || a.ui.showSettingsPanel || a.ui.showIdentityPanel || a.ui.showContactDetail || a.ui.showCreateGroup || a.ui.showGroupDetail || a.ui.contextMenu.visible {
		return a.layoutWithOverlay(gtx)
	}

	return a.layoutMain(gtx)
}

func (a *App) layoutMain(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Custom title bar (replaces OS decorations)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutTitleBar(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutHorizontalDivider(gtx, a.goldDividerColor())
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutColumns(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutHorizontalDivider(gtx, a.goldDividerColor())
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutStatusBar(gtx)
		}),
	)
}

func (a *App) layoutWithOverlay(gtx layout.Context) layout.Dimensions {
	// Render main UI behind
	a.layoutMain(gtx)
	// Overlay the appropriate dialog (priority order)
	if a.ui.showSettingsPanel {
		return a.layoutSettingsOverlay(gtx)
	}
	if a.ui.showIdentityPanel {
		return a.layoutIdentityPanel(gtx)
	}
	if a.ui.showContactDetail {
		return a.layoutContactDetail(gtx)
	}
	if a.ui.showCreateGroup {
		return a.layoutCreateGroupDialog(gtx)
	}
	if a.ui.showGroupDetail {
		return a.layoutGroupDetailPanel(gtx)
	}
	if a.ui.contextMenu.visible {
		return a.layoutContextMenu(gtx)
	}
	return a.layoutAddContactDialog(gtx)
}

// layoutSettingsOverlay renders the settings panel as a centered overlay.
func (a *App) layoutSettingsOverlay(gtx layout.Context) layout.Dimensions {
	if a.ui.settingsScreen == nil {
		return layout.Dimensions{}
	}

	// Semi-transparent backdrop
	paint.FillShape(gtx.Ops, color.NRGBA{A: 160},
		clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	// Centered card with max width/height
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(600)
		maxH := gtx.Dp(500)
		if gtx.Constraints.Max.X < maxW {
			maxW = gtx.Constraints.Max.X - gtx.Dp(32)
		}
		if gtx.Constraints.Max.Y < maxH {
			maxH = gtx.Constraints.Max.Y - gtx.Dp(32)
		}
		gtx.Constraints.Min.X = maxW
		gtx.Constraints.Max.X = maxW
		gtx.Constraints.Min.Y = maxH
		gtx.Constraints.Max.Y = maxH

		// Card background
		rr := gtx.Dp(8)
		size := image.Pt(maxW, maxH)
		cardRect := clip.RRect{Rect: image.Rect(0, 0, size.X, size.Y), NE: rr, NW: rr, SE: rr, SW: rr}
		paint.FillShape(gtx.Ops, a.ui.theme.Surface, cardRect.Op(gtx.Ops))

		// Gold border
		borderColor := a.ui.theme.Accent
		borderColor.A = 80
		paint.FillShape(gtx.Ops, borderColor, clip.Stroke{
			Path:  cardRect.Path(gtx.Ops),
			Width: float32(gtx.Dp(1)),
		}.Op())

		return a.layoutSettingsPanel(gtx)
	})
}

func (a *App) drawBackground(gtx layout.Context) {
	w := gtx.Constraints.Max.X
	h := gtx.Constraints.Max.Y
	mid := h / 2
	paint.FillShape(gtx.Ops, a.ui.theme.BackgroundSky, clip.Rect{Max: image.Pt(w, mid)}.Op())
	paint.FillShape(gtx.Ops, a.ui.theme.BackgroundCity, clip.Rect{Min: image.Pt(0, mid), Max: image.Pt(w, h)}.Op())
}

func (a *App) layoutColumns(gtx layout.Context) layout.Dimensions {
	dividerColor := a.goldDividerColor()

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(300)
			gtx.Constraints.Max.X = gtx.Dp(300)
			return a.layoutContactsColumn(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutVerticalDivider(gtx, dividerColor)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutChatColumn(gtx)
		}),
	)
}

// ── Contacts Column ─────────────────────────────────────────────────────────

func (a *App) layoutContactsColumn(gtx layout.Context) layout.Dimensions {
	a.fillColumnBg(gtx)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header with settings gear and identity/add buttons
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(8), Top: unit.Dp(16), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					// Profile/identity button
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &a.ui.identityBtn, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								// Simple avatar circle with initial
								radius := gtx.Dp(14)
								size := image.Point{X: radius * 2, Y: radius * 2}
								circleColor := a.ui.theme.Primary
								circleColor.A = 180
								defer clip.Ellipse{Max: size}.Push(gtx.Ops).Pop()
								paint.FillShape(gtx.Ops, circleColor, clip.Ellipse{Max: size}.Op(gtx.Ops))
								// Center a "B" inside
								return layout.Stack{Alignment: layout.Center}.Layout(gtx,
									layout.Expanded(func(gtx layout.Context) layout.Dimensions {
										return layout.Dimensions{Size: size}
									}),
									layout.Stacked(func(gtx layout.Context) layout.Dimensions {
										l := material.Caption(a.theme, "B")
										l.Color = color.NRGBA(a.ui.theme.TextPrimary)
										l.Font.Weight = font.Bold
										l.TextSize = unit.Sp(12)
										return l.Layout(gtx)
									}),
								)
							})
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						title := "BABYLON TOWER"
						l := material.H6(a.theme, title)
						l.Color = color.NRGBA(a.ui.theme.Accent)
						l.Font.Weight = font.Bold
						l.TextSize = unit.Sp(14)
						return l.Layout(gtx)
					}),
					// Add contact button (only show on chats tab)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.sidebarTab != "chats" {
							return layout.Dimensions{}
						}
						btn := material.Clickable(gtx, &a.ui.addContactBtn, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								l := material.Body1(a.theme, "+")
								l.Color = color.NRGBA(a.ui.theme.Accent)
								l.Font.Weight = font.Bold
								l.TextSize = unit.Sp(18)
								return l.Layout(gtx)
							})
						})
						return btn
					}),
					// Settings gear button
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSettingsGear(gtx)
					}),
				)
			})
		}),

		// Tab navigation: CHATS | GROUPS
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSidebarTabs(gtx)
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.drawDecorativeLine(gtx, 160)
		}),

		// Tab content
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if a.sidebarTab == "groups" {
				return a.layoutGroupsTab(gtx)
			}
			return a.layoutChatsTab(gtx)
		}),
	)
}

// layoutChatsTab renders the contacts/conversations tab content.
func (a *App) layoutChatsTab(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Search
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutSearchInput(gtx)
			})
		}),

		// Mnemonic backup reminder
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutMnemonicReminder(gtx)
		}),

		// Contact / conversation list
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			count := len(a.conversations)
			if count == 0 {
				count = len(a.contactList)
			}
			if count == 0 {
				return a.layoutEmptyContacts(gtx)
			}

			if len(a.conversations) > 0 {
				return material.List(a.theme, &a.ui.contactList).Layout(gtx, len(a.conversations), func(gtx layout.Context, index int) layout.Dimensions {
					return a.layoutConversationItem(gtx, index)
				})
			}
			return material.List(a.theme, &a.ui.contactList).Layout(gtx, len(a.contactList), func(gtx layout.Context, index int) layout.Dimensions {
				return a.layoutContactItem(gtx, index)
			})
		}),
	)
}

func (a *App) layoutSearchInput(gtx layout.Context) layout.Dimensions {
	inputStyle := DarkClayTablet()
	inputStyle.CornerRadius = 4
	inputStyle.ShadowOffset = 1
	inputStyle.Padding = layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(6), Bottom: unit.Dp(6)}
	return ClayTablet(inputStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		ed := material.Editor(a.theme, &a.ui.searchInput, "Search contacts...")
		ed.Color = color.NRGBA(a.ui.theme.TextPrimary)
		ed.HintColor = color.NRGBA(a.ui.theme.TextSecondary)
		ed.TextSize = unit.Sp(12)
		return ed.Layout(gtx)
	})
}

func (a *App) layoutEmptyContacts(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(a.theme, "No contacts yet")
				l.Color = color.NRGBA(a.ui.theme.TextSecondary)
				l.Alignment = text.Middle
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, "Press + to add a contact")
				l.Color = color.NRGBA(a.ui.theme.Accent)
				l.Alignment = text.Middle
				return l.Layout(gtx)
			}),
		)
	})
}

func (a *App) layoutConversationItem(gtx layout.Context, index int) layout.Dimensions {
	if index >= len(a.conversations) {
		return layout.Dimensions{}
	}
	conv := a.conversations[index]
	contact := conv.Contact

	for len(a.ui.contactButtons) <= index {
		a.ui.contactButtons = append(a.ui.contactButtons, widget.Clickable{})
	}

	isSelected := contact.PublicKeyBase58 == a.selectedKey
	unread := a.unreadCounts[contact.PublicKeyBase58]

	displayName := contact.DisplayName
	if displayName == "" {
		pk := contact.PublicKeyBase58
		if len(pk) > 12 {
			displayName = pk[:6] + "..." + pk[len(pk)-6:]
		} else {
			displayName = pk
		}
	}

	lastMsg := ""
	if conv.LastMessage != nil {
		lastMsg = conv.LastMessage.Text
		if len(lastMsg) > 35 {
			lastMsg = lastMsg[:35] + "..."
		}
		if conv.LastMessage.IsOutgoing {
			lastMsg = "You: " + lastMsg
		}
	}

	return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, &a.ui.contactButtons[index], func(gtx layout.Context) layout.Dimensions {
			// Selected highlight
			if isSelected {
				sz := gtx.Constraints.Max
				rr := gtx.Dp(4)
				highlightColor := a.ui.theme.Accent
				highlightColor.A = 30
				rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, gtx.Dp(56)), NE: rr, NW: rr, SE: rr, SW: rr}
				paint.FillShape(gtx.Ops, highlightColor, rect.Op(gtx.Ops))
			}

			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					// Online dot
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dotColor := color.NRGBA(a.ui.theme.TextSecondary)
						dotColor.A = 80
						if contact.IsOnline {
							dotColor = color.NRGBA(a.ui.theme.Success)
						}
						radius := gtx.Dp(4)
						size := image.Point{X: radius * 2, Y: radius * 2}
						defer clip.Ellipse{Max: size}.Push(gtx.Ops).Pop()
						paint.FillShape(gtx.Ops, dotColor, clip.Ellipse{Max: size}.Op(gtx.Ops))
						return layout.Dimensions{Size: size}
					}),
					// Name, last message, unread
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								// Name row
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
										// Verified indicator
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if !a.isContactVerified(contact.PublicKeyBase58) {
												return layout.Dimensions{}
											}
											return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												l := material.Caption(a.theme, "\u2713")
												l.Color = color.NRGBA(a.ui.theme.Accent)
												l.Font.Weight = font.Bold
												l.TextSize = unit.Sp(10)
												return l.Layout(gtx)
											})
										}),
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											nameColor := color.NRGBA(a.ui.theme.TextPrimary)
											if isSelected {
												nameColor = color.NRGBA(a.ui.theme.Accent)
											}
											l := material.Body2(a.theme, displayName)
											l.Color = nameColor
											l.Font.Weight = font.Medium
											return l.Layout(gtx)
										}),
										// Unread badge
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if unread <= 0 {
												return layout.Dimensions{}
											}
											return a.layoutUnreadBadge(gtx, unread)
										}),
									)
								}),
								// Last message preview
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if lastMsg == "" {
										return layout.Dimensions{}
									}
									l := material.Caption(a.theme, lastMsg)
									l.Color = color.NRGBA(a.ui.theme.TextSecondary)
									l.TextSize = unit.Sp(10)
									return l.Layout(gtx)
								}),
							)
						})
					}),
				)
			})
		})
	})
}

func (a *App) layoutContactItem(gtx layout.Context, index int) layout.Dimensions {
	if index >= len(a.contactList) {
		return layout.Dimensions{}
	}
	contact := a.contactList[index]

	for len(a.ui.contactButtons) <= index {
		a.ui.contactButtons = append(a.ui.contactButtons, widget.Clickable{})
	}

	isSelected := contact.PublicKeyBase58 == a.selectedKey

	displayName := contact.DisplayName
	if displayName == "" {
		pk := contact.PublicKeyBase58
		if len(pk) > 12 {
			displayName = pk[:6] + "..." + pk[len(pk)-6:]
		} else {
			displayName = pk
		}
	}

	return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, &a.ui.contactButtons[index], func(gtx layout.Context) layout.Dimensions {
			if isSelected {
				sz := gtx.Constraints.Max
				rr := gtx.Dp(4)
				highlightColor := a.ui.theme.Accent
				highlightColor.A = 30
				rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, gtx.Dp(44)), NE: rr, NW: rr, SE: rr, SW: rr}
				paint.FillShape(gtx.Ops, highlightColor, rect.Op(gtx.Ops))
			}

			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dotColor := color.NRGBA(a.ui.theme.TextSecondary)
						dotColor.A = 80
						if contact.IsOnline {
							dotColor = color.NRGBA(a.ui.theme.Success)
						}
						radius := gtx.Dp(4)
						size := image.Point{X: radius * 2, Y: radius * 2}
						defer clip.Ellipse{Max: size}.Push(gtx.Ops).Pop()
						paint.FillShape(gtx.Ops, dotColor, clip.Ellipse{Max: size}.Op(gtx.Ops))
						return layout.Dimensions{Size: size}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							nameColor := color.NRGBA(a.ui.theme.TextPrimary)
							if isSelected {
								nameColor = color.NRGBA(a.ui.theme.Accent)
							}
							l := material.Body2(a.theme, displayName)
							l.Color = nameColor
							l.Font.Weight = font.Medium
							return l.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func (a *App) layoutUnreadBadge(gtx layout.Context, count int) layout.Dimensions {
	badgeText := fmt.Sprintf("%d", count)
	if count > 99 {
		badgeText = "99+"
	}

	return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Constraints.Min
				minW := gtx.Dp(18)
				if sz.X < minW {
					sz.X = minW
				}
				rr := sz.Y / 2
				rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
				paint.FillShape(gtx.Ops, a.ui.theme.Accent, rect.Op(gtx.Ops))
				return layout.Dimensions{Size: sz}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(a.theme, badgeText)
					l.Color = color.NRGBA{R: 20, G: 16, B: 12, A: 255}
					l.TextSize = unit.Sp(9)
					l.Font.Weight = font.Bold
					return l.Layout(gtx)
				})
			}),
		)
	})
}

// ── Chat Column ─────────────────────────────────────────────────────────────

func (a *App) layoutChatColumn(gtx layout.Context) layout.Dimensions {
	// Show group view when a group is selected
	if a.sidebarTab == "groups" && a.selectedGroupID != "" {
		return a.layoutGroupChatColumn(gtx)
	}
	if a.sidebarTab == "groups" && a.selectedGroupID == "" {
		surfaceColor := a.ui.theme.Surface
		surfaceColor.A = 180
		paint.FillShape(gtx.Ops, surfaceColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())
		return a.layoutChatPlaceholder(gtx, "Select a group to view details")
	}

	surfaceColor := a.ui.theme.Surface
	surfaceColor.A = 180
	paint.FillShape(gtx.Ops, surfaceColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutChatHeader(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutHorizontalDivider(gtx, a.goldDividerColor())
		}),
		// Key change alert banner
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !a.hasKeyChangeAlert() {
				return layout.Dimensions{}
			}
			return a.layoutKeyChangeAlert(gtx)
		}),
		// Chat error banner
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.chatError == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, a.chatError)
				l.Color = color.NRGBA(a.ui.theme.Error)
				return l.Layout(gtx)
			})
		}),
		// Messages
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if a.selectedKey == "" {
					return a.layoutChatPlaceholder(gtx, "Select a contact to start chatting")
				}

				messages := a.messageCache[a.selectedKey]
				if len(messages) == 0 {
					return a.layoutChatPlaceholder(gtx, "No messages yet.\nSend the first inscription...")
				}

				// Scroll to bottom if flagged
				if a.scrollToBottom {
					a.ui.chatList.Position.First = len(messages)
					a.ui.chatList.Position.BeforeEnd = false
					a.scrollToBottom = false
				}

				return material.List(a.theme, &a.ui.chatList).Layout(gtx, len(messages), func(gtx layout.Context, index int) layout.Dimensions {
					return a.layoutRichMessageItem(gtx, messages[index], index)
				})
			})
		}),
		// Typing indicator
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutTypingIndicator(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutHorizontalDivider(gtx, a.goldDividerColor())
		}),
		// Reply compose bar
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutReplyBar(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutMessageInput(gtx)
		}),
	)
}

func (a *App) layoutChatHeader(gtx layout.Context) layout.Dimensions {
	chatTitle := "Select a contact"
	hasContact := a.selectedKey != ""
	if hasContact {
		chatTitle = a.selectedContactName()
		if chatTitle == "" {
			chatTitle = "Chat"
		}
	}

	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20), Top: unit.Dp(14), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if hasContact {
			// Clickable contact name opens detail panel
			return material.Clickable(gtx, &a.ui.chatHeaderBtn, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					// Verified checkmark
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !a.isContactVerified(a.selectedKey) {
							return layout.Dimensions{}
						}
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, "\u2713")
							l.Color = color.NRGBA(a.ui.theme.Accent)
							l.Font.Weight = font.Bold
							l.TextSize = unit.Sp(12)
							return l.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.H6(a.theme, chatTitle)
						l.Color = color.NRGBA(a.ui.theme.Accent)
						l.Font.Weight = font.Bold
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, "\u25B8") // ▸ arrow
							l.Color = color.NRGBA(a.ui.theme.TextSecondary)
							return l.Layout(gtx)
						})
					}),
				)
			})
		}
		l := material.H6(a.theme, chatTitle)
		l.Color = color.NRGBA(a.ui.theme.Accent)
		l.Font.Weight = font.Bold
		return l.Layout(gtx)
	})
}

func (a *App) layoutChatPlaceholder(gtx layout.Context, msg string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Body1(a.theme, msg)
		l.Color = color.NRGBA(a.ui.theme.TextSecondary)
		l.Alignment = text.Middle
		return l.Layout(gtx)
	})
}

func (a *App) layoutMessageInput(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				inputStyle := DarkClayTablet()
				inputStyle.CornerRadius = 6
				inputStyle.ShadowOffset = 1
				inputStyle.Padding = layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}
				return ClayTablet(inputStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(a.theme, &a.ui.messageInput, "Inscribe your message...")
					ed.Color = color.NRGBA(a.ui.theme.TextPrimary)
					ed.HintColor = color.NRGBA(a.ui.theme.TextSecondary)
					return ed.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(a.theme, &a.ui.sendButton, "Send")
					btn.Background = color.NRGBA(a.ui.theme.Primary)
					btn.Color = color.NRGBA(a.ui.theme.TextPrimary)
					btn.CornerRadius = unit.Dp(6)
					btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
					return btn.Layout(gtx)
				})
			}),
		)
	})
}

// ── Message Bubbles ─────────────────────────────────────────────────────────

func (a *App) layoutMessageItem(gtx layout.Context, msg *babylonapp.ChatMessage) layout.Dimensions {
	if msg == nil {
		return layout.Dimensions{}
	}

	var tabletStyle ClayTabletStyle
	var textColor color.NRGBA
	if msg.IsOutgoing {
		tabletStyle = ClayTabletStyle{
			Clay:         color.NRGBA{R: 35, G: 50, B: 75, A: 255},
			Shadow:       color.NRGBA{R: 18, G: 26, B: 44, A: 180},
			Highlight:    color.NRGBA{R: 55, G: 75, B: 105, A: 80},
			Border:       color.NRGBA{R: 65, G: 85, B: 115, A: 50},
			Surface:      color.NRGBA{R: 45, G: 60, B: 85, A: 255},
			CornerRadius: 8,
			ShadowOffset: 2,
			Padding:      layout.UniformInset(unit.Dp(12)),
		}
		textColor = color.NRGBA{R: 220, G: 210, B: 195, A: 255}
	} else {
		tabletStyle = DarkClayTablet()
		tabletStyle.CornerRadius = 8
		tabletStyle.Padding = layout.UniformInset(unit.Dp(12))
		textColor = color.NRGBA(a.ui.theme.TextPrimary)
	}

	return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxBubbleW := gtx.Constraints.Max.X * 3 / 4
		if maxBubbleW < gtx.Dp(120) {
			maxBubbleW = gtx.Constraints.Max.X
		}

		dir := layout.W
		if msg.IsOutgoing {
			dir = layout.E
		}

		return dir.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = maxBubbleW
			return ClayTablet(tabletStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// Message text
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Body2(a.theme, msg.Text)
						label.Color = textColor
						return label.Layout(gtx)
					}),
					// Timestamp
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						ts := msg.Timestamp.Format("15:04")
						l := material.Caption(a.theme, ts)
						tsColor := textColor
						tsColor.A = 140
						l.Color = tsColor
						l.TextSize = unit.Sp(9)
						l.Alignment = text.End
						return l.Layout(gtx)
					}),
				)
			})
		})
	})
}

// ── Title Bar ───────────────────────────────────────────────────────────────

func (a *App) layoutTitleBar(gtx layout.Context) layout.Dimensions {
	barColor := a.ui.theme.BackgroundCity

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(32)}
			paint.FillShape(gtx.Ops, barColor, clip.Rect{Max: size}.Op())
			// Make the entire title bar a window-drag area
			defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
			system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(4), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					// Status dot
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutStatusDot(gtx)
					}),
					// Title
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, "Babylon Tower")
							l.Color = color.NRGBA(a.ui.theme.Accent)
							l.Font.Weight = font.Bold
							l.TextSize = unit.Sp(11)
							return l.Layout(gtx)
						})
					}),
					// Spacer
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X}}
					}),
					// Close button
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &a.ui.closeBtn, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								l := material.Body2(a.theme, "\u2715") // ✕
								l.Color = color.NRGBA(a.ui.theme.TextSecondary)
								return l.Layout(gtx)
							})
						})
					}),
				)
			})
		},
	)
}

func (a *App) layoutStatusDot(gtx layout.Context) layout.Dimensions {
	a.status.mu.RLock()
	coreReady := a.status.CoreReady
	coreError := a.status.CoreError
	ipfs := a.status.IPFSConnected
	babylon := a.status.BabylonConnected
	a.status.mu.RUnlock()

	var dotColor color.NRGBA
	switch {
	case coreError != "":
		dotColor = color.NRGBA(a.ui.theme.Error)
	case !coreReady:
		dotColor = color.NRGBA(a.ui.theme.Accent)
	case ipfs && babylon:
		dotColor = color.NRGBA(a.ui.theme.Success)
	case ipfs:
		dotColor = color.NRGBA(a.ui.theme.Accent)
	default:
		dotColor = color.NRGBA(a.ui.theme.Error)
	}

	radius := gtx.Dp(3)
	size := image.Point{X: radius * 2, Y: radius * 2}
	defer clip.Ellipse{Max: size}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, dotColor, clip.Ellipse{Max: size}.Op(gtx.Ops))
	return layout.Dimensions{Size: size}
}

func (a *App) layoutStatusBar(gtx layout.Context) layout.Dimensions {
	barColor := a.ui.theme.BackgroundCity
	statusText := a.StatusText()

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(22)}
			paint.FillShape(gtx.Ops, barColor, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(3), Bottom: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(a.theme, statusText)
				label.Color = color.NRGBA(a.ui.theme.TextSecondary)
				label.TextSize = unit.Sp(9)
				return label.Layout(gtx)
			})
		},
	)
}

// ── Events ──────────────────────────────────────────────────────────────────

func (a *App) handleEvents(gtx layout.Context) {
	// Drain cross-goroutine events first (keeps all state writes on main goroutine)
	a.drainUIEvents()

	// Send button
	if a.ui.sendButton.Clicked(gtx) {
		if err := a.sendMessage(); err != nil {
			logger.Errorw("send message failed", "error", err)
		}
	}

	// Enter key in message input
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

	// Add Contact button
	if a.ui.addContactBtn.Clicked(gtx) {
		a.ui.showAddContact = true
		a.ui.addContactInput.SetText("")
		a.ui.addContactError = ""
	}

	// Add Contact dialog: submit (button or Enter)
	if a.ui.addContactSubmit.Clicked(gtx) {
		a.addContact()
	}
	for {
		event, ok := a.ui.addContactInput.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			a.addContact()
		}
	}

	// Add Contact dialog: cancel
	if a.ui.addContactCancel.Clicked(gtx) {
		a.ui.showAddContact = false
	}

	// Contact selection from conversations
	if len(a.conversations) > 0 {
		for i, conv := range a.conversations {
			if len(a.ui.contactButtons) <= i {
				a.ui.contactButtons = append(a.ui.contactButtons, widget.Clickable{})
			}
			if a.ui.contactButtons[i].Clicked(gtx) {
				a.selectContact(conv.Contact.PublicKeyBase58)
			}
		}
	} else {
		// Fallback: contact list without conversations
		for i, contact := range a.contactList {
			if len(a.ui.contactButtons) <= i {
				a.ui.contactButtons = append(a.ui.contactButtons, widget.Clickable{})
			}
			if a.ui.contactButtons[i].Clicked(gtx) {
				a.selectContact(contact.PublicKeyBase58)
			}
		}
	}

	// Window close button
	if a.ui.closeBtn.Clicked(gtx) {
		a.window.Perform(system.ActionClose)
	}

	// Settings gear button
	if a.ui.settingsBtn.Clicked(gtx) {
		a.ui.showSettingsPanel = true
	}
	a.handleSettingsEvents(gtx)

	// Identity panel
	if a.ui.identityBtn.Clicked(gtx) {
		a.ui.showIdentityPanel = true
		a.ui.identityPanel.copyFeedback = ""
	}
	if a.ui.identityPanel.closeBtn.Clicked(gtx) {
		a.ui.showIdentityPanel = false
	}
	a.handleIdentityCopyEvents(gtx)

	// Key change alert dismiss
	if a.ui.keyChangeDismissBtn.Clicked(gtx) && a.selectedKey != "" {
		a.keyChangeAlerts[a.selectedKey] = true // true = dismissed
	}

	// Mnemonic backup reminder dismiss
	if a.ui.mnemonicDismissBtn.Clicked(gtx) {
		a.mnemonicReminderDismissed = true
		if a.coreApp != nil && a.coreApp.Storage() != nil {
			_ = a.coreApp.Storage().SetConfig("mnemonic_backup_dismissed", "true")
		}
	}

	// Chat header click → contact detail or group detail
	if a.ui.chatHeaderBtn.Clicked(gtx) {
		if a.sidebarTab == "groups" && a.selectedGroupID != "" {
			a.ui.showGroupDetail = true
			a.ui.groupConfirmLeave = false
			a.ui.groupConfirmDelete = false
		} else if a.selectedKey != "" {
			a.ui.showContactDetail = true
			a.ui.contactDetail.copyFeedback = ""
			a.ui.contactDetail.findStatus = ""
			a.ui.contactDetail.confirmRemove = false
			a.ui.contactDetail.showRename = false
		}
	}
	a.handleContactDetailEvents(gtx)

	// Group events
	a.handleGroupEvents(gtx)

	// Rich message events (context menu, message bubble clicks, reply cancel)
	a.handleContextMenuEvents(gtx)
	a.handleMessageBubbleClicks(gtx)
	if a.ui.replyCancelBtn.Clicked(gtx) {
		a.replyToMessage = nil
	}

	// Typing debounce
	a.handleTypingDebounce()
}

func (a *App) handleSettingsEvents(gtx layout.Context) {
	if a.ui.settingsScreen == nil || !a.ui.showSettingsPanel {
		return
	}
	event := a.ui.settingsScreen.Update(gtx)
	switch event {
	case SettingsSaved:
		a.saveSettings()
	case SettingsClosed:
		a.ui.showSettingsPanel = false
	}
}

func (a *App) saveSettings() {
	if a.ui.settingsScreen == nil {
		return
	}

	if a.appCfg == nil {
		a.appCfg = config.DefaultAppConfig()
	}

	a.ui.settingsScreen.ApplyToConfig(a.appCfg)

	if a.dataDir != "" {
		if err := config.SaveAppConfig(a.dataDir, a.appCfg); err != nil {
			logger.Errorw("failed to save config", "error", err)
			return
		}
	}

	// Apply immediate changes (no restart needed)
	a.applyAppearanceSettings()

	logger.Infow("settings saved", "display_name", a.appCfg.Profile.DisplayName)
}

// applyAppearanceSettings applies appearance changes that don't require restart.
func (a *App) applyAppearanceSettings() {
	if a.appCfg == nil {
		return
	}

	// Dark mode toggle
	wantDark := a.appCfg.Appearance.DarkMode
	if wantDark != a.ui.isDarkMode {
		a.ui.isDarkMode = wantDark
		if wantDark {
			a.ui.theme = NewTheme(DarkTheme())
		} else {
			a.ui.theme = NewTheme(LightTheme())
		}
	}

	// Font size
	fontSize := a.appCfg.Appearance.FontSize
	if fontSize >= 8 && fontSize <= 32 {
		a.theme.TextSize = unit.Sp(float32(fontSize))
	}

	// Window size
	w := a.appCfg.Appearance.WindowWidth
	h := a.appCfg.Appearance.WindowHeight
	if w >= 400 && h >= 300 {
		a.window.Option(app.Size(unit.Dp(w), unit.Dp(h)))
	}
}

func (a *App) handleIdentityCopyEvents(gtx layout.Context) {
	if !a.ui.showIdentityPanel {
		return
	}
	info := a.getIdentityInfo()

	if a.ui.identityPanel.copyPubKeyBtn.Clicked(gtx) {
		a.copyToClipboard(gtx, info.pubKeyBase58)
		a.ui.identityPanel.copyFeedback = "Public key copied!"
	}
	if a.ui.identityPanel.copyFingerprintBtn.Clicked(gtx) {
		a.copyToClipboard(gtx, info.fingerprint)
		a.ui.identityPanel.copyFeedback = "Fingerprint copied!"
	}
	if a.ui.identityPanel.copyContactLinkBtn.Clicked(gtx) {
		a.copyToClipboard(gtx, info.contactLink)
		a.ui.identityPanel.copyFeedback = "Contact link copied!"
	}
}

func (a *App) handleContactDetailEvents(gtx layout.Context) {
	if !a.ui.showContactDetail {
		return
	}

	// Close
	if a.ui.contactDetail.closeBtn.Clicked(gtx) {
		a.ui.showContactDetail = false
		return
	}

	contact := a.selectedContactInfo()
	if contact == nil {
		a.ui.showContactDetail = false
		return
	}

	// Copy public key
	if a.ui.contactDetail.copyPubKeyBtn.Clicked(gtx) {
		a.copyToClipboard(gtx, contact.PublicKeyBase58)
		a.ui.contactDetail.copyFeedback = "Public key copied!"
	}

	// Copy contact link
	if a.ui.contactDetail.copyLinkBtn.Clicked(gtx) {
		link := contact.ContactLink
		if link == "" {
			link = "btower://" + contact.PublicKeyBase58
		}
		a.copyToClipboard(gtx, link)
		a.ui.contactDetail.copyFeedback = "Contact link copied!"
	}

	// Rename button
	if a.ui.contactDetail.renameBtn.Clicked(gtx) {
		a.ui.contactDetail.showRename = true
		a.ui.contactDetail.renameInput.SetText(contact.DisplayName)
		a.ui.contactDetail.renameError = ""
	}

	// Rename save
	if a.ui.contactDetail.renameSaveBtn.Clicked(gtx) {
		a.renameSelectedContact()
	}
	// Rename save on Enter
	for {
		event, ok := a.ui.contactDetail.renameInput.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			a.renameSelectedContact()
		}
	}

	// Rename cancel
	if a.ui.contactDetail.renameCancelBtn.Clicked(gtx) {
		a.ui.contactDetail.showRename = false
		a.ui.contactDetail.renameError = ""
	}

	// Copy fingerprint
	if a.ui.contactDetail.copyFingerprintBtn.Clicked(gtx) {
		if fp, err := babylonapp.ContactFingerprint(contact.PublicKeyBase58, contact.X25519KeyBase58); err == nil {
			a.copyToClipboard(gtx, fp)
			a.ui.contactDetail.copyFeedback = "Fingerprint copied!"
		}
	}

	// Copy safety number
	if a.ui.contactDetail.copySafetyNumBtn.Clicked(gtx) {
		info := a.getIdentityInfo()
		if sn, err := babylonapp.SafetyNumber(info.pubKeyBase58, contact.PublicKeyBase58); err == nil {
			a.copyToClipboard(gtx, sn)
			a.ui.contactDetail.copyFeedback = "Safety number copied!"
		}
	}

	// Verify / unverify contact
	if a.ui.contactDetail.verifyBtn.Clicked(gtx) {
		isVerified := a.isContactVerified(contact.PublicKeyBase58)
		a.setContactVerified(contact.PublicKeyBase58, !isVerified)
		if !isVerified {
			a.ui.contactDetail.copyFeedback = "Contact verified!"
		} else {
			a.ui.contactDetail.copyFeedback = "Verification removed"
		}
	}

	// Find on Network
	if a.ui.contactDetail.findNetworkBtn.Clicked(gtx) {
		a.ui.contactDetail.findStatus = "Searching DHT..."
		go a.findContactOnNetwork(contact.PublicKeyBase58)
	}

	// Remove contact (two-click confirmation)
	if a.ui.contactDetail.removeBtn.Clicked(gtx) {
		if a.ui.contactDetail.confirmRemove {
			a.removeSelectedContact()
		} else {
			a.ui.contactDetail.confirmRemove = true
		}
	}
}

func (a *App) renameSelectedContact() {
	if a.contactMgr == nil || a.selectedKey == "" {
		return
	}
	newName := a.ui.contactDetail.renameInput.Text()
	if newName == "" {
		a.ui.contactDetail.renameError = "Name cannot be empty"
		return
	}
	if err := a.contactMgr.UpdateContactName(a.selectedKey, newName); err != nil {
		a.ui.contactDetail.renameError = err.Error()
		return
	}
	a.ui.contactDetail.showRename = false
	a.ui.contactDetail.renameError = ""
	a.refreshContacts()
}

func (a *App) removeSelectedContact() {
	if a.contactMgr == nil || a.selectedKey == "" {
		return
	}
	if err := a.contactMgr.RemoveContact(a.selectedKey); err != nil {
		logger.Warnw("failed to remove contact", "error", err)
		return
	}
	a.ui.showContactDetail = false
	delete(a.messageCache, a.selectedKey)
	delete(a.unreadCounts, a.selectedKey)
	a.selectedKey = ""
	a.refreshContacts()
}

func (a *App) findContactOnNetwork(pubKey string) {
	if a.contactMgr == nil {
		a.ui.contactDetail.findStatus = "Not connected"
		a.window.Invalidate()
		return
	}
	_, err := a.contactMgr.FindAndConnect(pubKey)
	if err != nil {
		a.ui.contactDetail.findStatus = "Not found"
		logger.Debugw("contact not found on network", "error", err)
	} else {
		a.ui.contactDetail.findStatus = "Found!"
		a.uiEvents <- uiEvent{Type: evtContactsChanged}
	}
	a.window.Invalidate()
}

func (a *App) copyToClipboard(gtx layout.Context, text string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
}

func (a *App) applyTheme(gtx layout.Context) {
	a.theme.Bg = color.NRGBA(a.ui.theme.Background)
	a.theme.Fg = color.NRGBA(a.ui.theme.TextPrimary)
	a.theme.ContrastBg = color.NRGBA(a.ui.theme.Primary)
	a.theme.ContrastFg = color.NRGBA(a.ui.theme.TextPrimary)
}

// ── Settings Gear Button ────────────────────────────────────────────────────

func (a *App) layoutSettingsGear(gtx layout.Context) layout.Dimensions {
	return material.Clickable(gtx, &a.ui.settingsBtn, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := material.Body1(a.theme, "\u2699") // ⚙ gear unicode
			l.Color = color.NRGBA(a.ui.theme.TextSecondary)
			l.TextSize = unit.Sp(18)
			return l.Layout(gtx)
		})
	})
}

// ── Key Change Alert ────────────────────────────────────────────────────────

func (a *App) layoutKeyChangeAlert(gtx layout.Context) layout.Dimensions {
	alertBg := a.ui.theme.Accent
	alertBg.A = 35

	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(6), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rr := gtx.Dp(4)
		size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(28)}
		rect := clip.RRect{Rect: image.Rect(0, 0, size.X, size.Y), NE: rr, NW: rr, SE: rr, SW: rr}
		paint.FillShape(gtx.Ops, alertBg, rect.Op(gtx.Ops))

		return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(6), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(a.theme, "Security alert: this contact's key has changed")
					l.Color = color.NRGBA(a.ui.theme.Accent)
					l.Font.Weight = font.Bold
					l.TextSize = unit.Sp(10)
					return l.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Clickable(gtx, &a.ui.keyChangeDismissBtn, func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, "DISMISS")
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						l.TextSize = unit.Sp(9)
						return l.Layout(gtx)
					})
				}),
			)
		})
	})
}

// ── Mnemonic Backup Reminder ────────────────────────────────────────────────

func (a *App) layoutMnemonicReminder(gtx layout.Context) layout.Dimensions {
	if a.mnemonicReminderDismissed || a.coreApp == nil {
		return layout.Dimensions{}
	}

	return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		reminderStyle := ClayTabletStyle{
			Clay:         color.NRGBA{R: 60, G: 50, B: 20, A: 255},
			Shadow:       color.NRGBA{R: 40, G: 30, B: 10, A: 180},
			Highlight:    color.NRGBA{R: 80, G: 70, B: 30, A: 80},
			Border:       color.NRGBA{R: 180, G: 150, B: 50, A: 120},
			Surface:      color.NRGBA{R: 55, G: 45, B: 18, A: 255},
			CornerRadius: 6,
			ShadowOffset: 1,
			Padding:      layout.Inset{Left: unit.Dp(10), Right: unit.Dp(6), Top: unit.Dp(6), Bottom: unit.Dp(6)},
		}
		return ClayTablet(reminderStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(a.theme, "Back up your mnemonic phrase to avoid losing access")
					l.Color = color.NRGBA(a.ui.theme.Accent)
					l.TextSize = unit.Sp(10)
					return l.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Clickable(gtx, &a.ui.mnemonicDismissBtn, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, "\u2715") // ✕
							l.Color = color.NRGBA(a.ui.theme.TextSecondary)
							l.TextSize = unit.Sp(12)
							return l.Layout(gtx)
						})
					})
				}),
			)
		})
	})
}

// ── Visual Helpers ──────────────────────────────────────────────────────────

func (a *App) layoutColumnHeader(gtx layout.Context, title string) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(16), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.H6(a.theme, title)
		l.Color = color.NRGBA(a.ui.theme.Accent)
		l.Font.Weight = font.Bold
		l.TextSize = unit.Sp(14)
		return l.Layout(gtx)
	})
}

func (a *App) drawDecorativeLine(gtx layout.Context, widthDp int) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Dp(unit.Dp(widthDp))
		h := gtx.Dp(1)
		accentColor := a.ui.theme.Accent
		accentColor.A = 120
		paint.FillShape(gtx.Ops, accentColor, clip.Rect{Max: image.Pt(w, h)}.Op())
		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}

func (a *App) fillColumnBg(gtx layout.Context) {
	bgColor := a.ui.theme.BackgroundCity
	bgColor.A = 200
	paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())
}

func (a *App) goldDividerColor() color.NRGBA {
	c := a.ui.theme.Accent
	c.A = 80
	return c
}

func layoutVerticalDivider(gtx layout.Context, dividerColor color.NRGBA) layout.Dimensions {
	w := gtx.Dp(1)
	h := gtx.Constraints.Max.Y
	size := image.Point{X: w, Y: h}
	paint.FillShape(gtx.Ops, dividerColor, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

func layoutHorizontalDivider(gtx layout.Context, dividerColor color.NRGBA) layout.Dimensions {
	size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(1)}
	paint.FillShape(gtx.Ops, dividerColor, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}
