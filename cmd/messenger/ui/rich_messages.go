package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	babylonapp "babylontower/pkg/app"
)

// ── Message Context Menu ────────────────────────────────────────────────────

type contextMenuState struct {
	visible    bool
	messageIdx int
	copyBtn    widget.Clickable
	replyBtn   widget.Clickable
	editBtn    widget.Clickable
	deleteBtn  widget.Clickable
	reactBtn   widget.Clickable
}

func (a *App) layoutContextMenu(gtx layout.Context) layout.Dimensions {
	if !a.ui.contextMenu.visible {
		return layout.Dimensions{}
	}

	// Semi-transparent overlay to catch dismiss clicks
	paint.FillShape(gtx.Ops, color.NRGBA{A: 80},
		clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(200)
		gtx.Constraints.Min.X = maxW
		gtx.Constraints.Max.X = maxW

		return a.layoutDialogCard(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				msg := a.contextMenuMessage()
				isOwn := msg != nil && msg.IsOutgoing

				children := []layout.FlexChild{
					a.contextMenuItem(gtx, &a.ui.contextMenu.copyBtn, "Copy Text"),
					a.contextMenuItem(gtx, &a.ui.contextMenu.replyBtn, "Reply"),
				}

				if isOwn {
					children = append(children,
						a.contextMenuItem(gtx, &a.ui.contextMenu.editBtn, "Edit"),
						a.contextMenuItem(gtx, &a.ui.contextMenu.deleteBtn, "Delete"),
					)
				}

				children = append(children,
					a.contextMenuItem(gtx, &a.ui.contextMenu.reactBtn, "React"),
				)

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func (a *App) contextMenuItem(_ layout.Context, btn *widget.Clickable, label string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(a.theme, label)
				l.Color = color.NRGBA(a.ui.theme.TextPrimary)
				l.TextSize = unit.Sp(12)
				return l.Layout(gtx)
			})
		})
	})
}

func (a *App) contextMenuMessage() *babylonapp.ChatMessage {
	msgs := a.messageCache[a.selectedKey]
	idx := a.ui.contextMenu.messageIdx
	if idx < 0 || idx >= len(msgs) {
		return nil
	}
	return msgs[idx]
}

// ── Message Status Indicator ────────────────────────────────────────────────

func (a *App) layoutMessageStatus(gtx layout.Context, msg *babylonapp.ChatMessage) layout.Dimensions {
	if !msg.IsOutgoing {
		return layout.Dimensions{}
	}

	var statusText string
	var statusColor color.NRGBA

	switch msg.Status {
	case babylonapp.StatusSending:
		statusText = "\u25CB" // ○ clock-like
		statusColor = color.NRGBA(a.ui.theme.TextSecondary)
	case babylonapp.StatusSent:
		statusText = "\u2713" // ✓ single check
		statusColor = color.NRGBA(a.ui.theme.TextSecondary)
	case babylonapp.StatusDelivered:
		statusText = "\u2713\u2713" // ✓✓ double check
		statusColor = color.NRGBA(a.ui.theme.TextSecondary)
	case babylonapp.StatusRead:
		statusText = "\u2713\u2713" // ✓✓ double check (gold)
		statusColor = color.NRGBA(a.ui.theme.Accent)
	case babylonapp.StatusFailed:
		statusText = "\u26A0" // ⚠ warning
		statusColor = color.NRGBA(a.ui.theme.Error)
	default:
		statusText = "\u2713"
		statusColor = color.NRGBA(a.ui.theme.TextSecondary)
	}

	return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Caption(a.theme, statusText)
		l.Color = statusColor
		l.TextSize = unit.Sp(8)
		return l.Layout(gtx)
	})
}

// ── Reactions Display ───────────────────────────────────────────────────────

func (a *App) layoutReactions(gtx layout.Context, msg *babylonapp.ChatMessage) layout.Dimensions {
	if len(msg.Reactions) == 0 {
		return layout.Dimensions{}
	}

	// Build reaction chips
	children := make([]layout.FlexChild, 0, len(msg.Reactions))
	for emoji, count := range msg.Reactions {
		e, c := emoji, count
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(4), Top: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				chipText := e
				if c > 1 {
					chipText = fmt.Sprintf("%s %d", e, c)
				}
				// Small pill background
				return layout.Stack{Alignment: layout.Center}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						sz := gtx.Constraints.Min
						rr := sz.Y / 2
						pillColor := a.ui.theme.Surface
						pillColor.A = 200
						rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
						paint.FillShape(gtx.Ops, pillColor, rect.Op(gtx.Ops))
						return layout.Dimensions{Size: sz}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, chipText)
							l.Color = color.NRGBA(a.ui.theme.Accent)
							l.TextSize = unit.Sp(9)
							return l.Layout(gtx)
						})
					}),
				)
			})
		}))
	}

	return layout.Flex{}.Layout(gtx, children...)
}

// ── Edit Indicator ──────────────────────────────────────────────────────────

func (a *App) layoutEditIndicator(gtx layout.Context, msg *babylonapp.ChatMessage) layout.Dimensions {
	if !msg.IsEdited {
		return layout.Dimensions{}
	}

	editText := "edited"
	if msg.EditedAt != nil {
		editText = fmt.Sprintf("edited %s", msg.EditedAt.Format("15:04"))
	}

	return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Caption(a.theme, editText)
		editColor := color.NRGBA(a.ui.theme.TextSecondary)
		editColor.A = 160
		l.Color = editColor
		l.TextSize = unit.Sp(8)
		return l.Layout(gtx)
	})
}

// ── Delete Tombstone ────────────────────────────────────────────────────────

func (a *App) layoutDeleteTombstone(gtx layout.Context) layout.Dimensions {
	tabletStyle := DarkClayTablet()
	tabletStyle.CornerRadius = 8
	tabletStyle.Padding = layout.UniformInset(unit.Dp(12))
	tabletStyle.Clay.A = 120 // More transparent

	return ClayTablet(tabletStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Body2(a.theme, "This inscription was erased")
		erasedColor := color.NRGBA(a.ui.theme.TextSecondary)
		erasedColor.A = 140
		l.Color = erasedColor
		l.Font.Style = font.Italic
		return l.Layout(gtx)
	})
}

// ── Reply/Quote Preview ─────────────────────────────────────────────────────

func (a *App) layoutReplyPreview(gtx layout.Context, msg *babylonapp.ChatMessage) layout.Dimensions {
	if msg.ReplyToText == "" {
		return layout.Dimensions{}
	}

	replyText := msg.ReplyToText
	if len(replyText) > 60 {
		replyText = replyText[:60] + "..."
	}

	return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Gold left border + quote content
		return layout.Flex{}.Layout(gtx,
			// Gold bar
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(28)
				w := gtx.Dp(2)
				rect := clip.Rect{Max: image.Pt(w, h)}
				paint.FillShape(gtx.Ops, color.NRGBA(a.ui.theme.Accent), rect.Op())
				return layout.Dimensions{Size: image.Pt(w, h)}
			}),
			// Quote text
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							sender := msg.ReplyToSender
							if sender == "" {
								sender = "Unknown"
							}
							l := material.Caption(a.theme, sender)
							l.Color = color.NRGBA(a.ui.theme.Accent)
							l.Font.Weight = font.Bold
							l.TextSize = unit.Sp(9)
							return l.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, replyText)
							l.Color = color.NRGBA(a.ui.theme.TextSecondary)
							l.TextSize = unit.Sp(9)
							return l.Layout(gtx)
						}),
					)
				})
			}),
		)
	})
}

// ── Typing Indicator ────────────────────────────────────────────────────────

func (a *App) layoutTypingIndicator(gtx layout.Context) layout.Dimensions {
	if a.selectedKey == "" {
		return layout.Dimensions{}
	}

	a.typingMu.Lock()
	typing, exists := a.typingContacts[a.selectedKey]
	a.typingMu.Unlock()

	if !exists || !typing {
		return layout.Dimensions{}
	}

	return layout.Inset{Left: unit.Dp(16), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		contactName := a.selectedContactName()
		if contactName == "" {
			contactName = "Contact"
		}
		l := material.Caption(a.theme, contactName+" is typing...")
		l.Color = color.NRGBA(a.ui.theme.TextSecondary)
		l.Font.Style = font.Italic
		l.TextSize = unit.Sp(10)
		return l.Layout(gtx)
	})
}

// ── Reply Compose Bar ───────────────────────────────────────────────────────

func (a *App) layoutReplyBar(gtx layout.Context) layout.Dimensions {
	if a.replyToMessage == nil {
		return layout.Dimensions{}
	}

	replyText := a.replyToMessage.Text
	if len(replyText) > 50 {
		replyText = replyText[:50] + "..."
	}

	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		replyBg := a.ui.theme.Accent
		replyBg.A = 25
		rr := gtx.Dp(4)
		sz := image.Pt(gtx.Constraints.Max.X, gtx.Dp(32))
		rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
		paint.FillShape(gtx.Ops, replyBg, rect.Op(gtx.Ops))

		return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(6), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				// Gold bar
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					h := gtx.Dp(20)
					w := gtx.Dp(2)
					rect := clip.Rect{Max: image.Pt(w, h)}
					paint.FillShape(gtx.Ops, color.NRGBA(a.ui.theme.Accent), rect.Op())
					return layout.Dimensions{Size: image.Pt(w, h)}
				}),
				// Reply content
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						sender := a.replyToMessage.SenderName
						l := material.Caption(a.theme, fmt.Sprintf("Replying to %s: %s", sender, replyText))
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						l.TextSize = unit.Sp(10)
						return l.Layout(gtx)
					})
				}),
				// Close button
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Clickable(gtx, &a.ui.replyCancelBtn, func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, "\u2715")
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						l.TextSize = unit.Sp(12)
						return l.Layout(gtx)
					})
				}),
			)
		})
	})
}

// ── Enhanced Message Item ───────────────────────────────────────────────────

func (a *App) layoutRichMessageItem(gtx layout.Context, msg *babylonapp.ChatMessage, index int) layout.Dimensions {
	if msg == nil {
		return layout.Dimensions{}
	}

	// Deleted message tombstone
	if msg.IsDeleted {
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			maxBubbleW := gtx.Constraints.Max.X * 3 / 4
			dir := layout.W
			if msg.IsOutgoing {
				dir = layout.E
			}
			return dir.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = maxBubbleW
				return a.layoutDeleteTombstone(gtx)
			})
		})
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

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Message bubble
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Make message clickable for context menu
					return material.Clickable(gtx, &a.ui.messageBubbleBtns[index%len(a.ui.messageBubbleBtns)], func(gtx layout.Context) layout.Dimensions {
						return ClayTablet(tabletStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								// Reply preview
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return a.layoutReplyPreview(gtx, msg)
								}),
								// Message text
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									label := material.Body2(a.theme, msg.Text)
									label.Color = textColor
									return label.Layout(gtx)
								}),
								// Timestamp + status + edited row
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
										// Timestamp
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											ts := msg.Timestamp.Format("15:04")
											l := material.Caption(a.theme, ts)
											tsColor := textColor
											tsColor.A = 140
											l.Color = tsColor
											l.TextSize = unit.Sp(9)
											return l.Layout(gtx)
										}),
										// Edited indicator
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return a.layoutEditIndicator(gtx, msg)
										}),
										// Message status (outgoing only)
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return a.layoutMessageStatus(gtx, msg)
										}),
									)
								}),
							)
						})
					})
				}),
				// Reactions below bubble
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutReactions(gtx, msg)
				}),
			)
		})
	})
}

// ── Typing State Management ─────────────────────────────────────────────────

func (a *App) setContactTyping(contactKey string, isTyping bool) {
	a.typingMu.Lock()
	defer a.typingMu.Unlock()

	if isTyping {
		a.typingContacts[contactKey] = true
		// Auto-clear after 5 seconds
		go func() {
			time.Sleep(5 * time.Second)
			a.typingMu.Lock()
			delete(a.typingContacts, contactKey)
			a.typingMu.Unlock()
			a.window.Invalidate()
		}()
	} else {
		delete(a.typingContacts, contactKey)
	}
	a.window.Invalidate()
}

// handleTypingDebounce sends typing events debounced.
func (a *App) handleTypingDebounce() {
	if a.appCfg != nil && !a.appCfg.Privacy.SendTypingIndicators {
		return
	}

	inputText := a.ui.messageInput.Text()
	wasTyping := a.isLocalTyping
	isTyping := len(strings.TrimSpace(inputText)) > 0

	if isTyping && !wasTyping {
		a.isLocalTyping = true
		a.lastTypingEvent = time.Now()
		// Would send typing event here when backend is wired
	} else if !isTyping && wasTyping {
		a.isLocalTyping = false
		// Would send stop-typing event here
	} else if isTyping && time.Since(a.lastTypingEvent) > 3*time.Second {
		// Refresh typing indicator every 3s while typing
		a.lastTypingEvent = time.Now()
	}
}

// ── Context Menu Event Handling ─────────────────────────────────────────────

func (a *App) handleContextMenuEvents(gtx layout.Context) {
	if !a.ui.contextMenu.visible {
		return
	}

	if a.ui.contextMenu.copyBtn.Clicked(gtx) {
		if msg := a.contextMenuMessage(); msg != nil {
			a.copyToClipboard(gtx, msg.Text)
		}
		a.ui.contextMenu.visible = false
	}

	if a.ui.contextMenu.replyBtn.Clicked(gtx) {
		if msg := a.contextMenuMessage(); msg != nil {
			a.replyToMessage = msg
		}
		a.ui.contextMenu.visible = false
	}

	if a.ui.contextMenu.editBtn.Clicked(gtx) {
		if msg := a.contextMenuMessage(); msg != nil && msg.IsOutgoing {
			a.editingMessage = msg
			a.ui.messageInput.SetText(msg.Text)
		}
		a.ui.contextMenu.visible = false
	}

	if a.ui.contextMenu.deleteBtn.Clicked(gtx) {
		if msg := a.contextMenuMessage(); msg != nil && msg.IsOutgoing {
			// Mark as deleted locally (backend wiring pending)
			msg.IsDeleted = true
			msg.Text = ""
		}
		a.ui.contextMenu.visible = false
	}

	if a.ui.contextMenu.reactBtn.Clicked(gtx) {
		// For now, add a default thumbs-up reaction
		if msg := a.contextMenuMessage(); msg != nil {
			if msg.Reactions == nil {
				msg.Reactions = make(map[string]int)
			}
			msg.Reactions["\U0001F44D"]++ // 👍
		}
		a.ui.contextMenu.visible = false
	}
}

// handleMessageBubbleClicks checks for long-press / click on message bubbles.
func (a *App) handleMessageBubbleClicks(gtx layout.Context) {
	msgs := a.messageCache[a.selectedKey]
	for i := range a.ui.messageBubbleBtns {
		if i >= len(msgs) {
			break
		}
		if a.ui.messageBubbleBtns[i].Clicked(gtx) {
			a.ui.contextMenu.visible = true
			a.ui.contextMenu.messageIdx = i
		}
	}
}
