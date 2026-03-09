package ui

import (
	"image"
	"image/color"

	babylonapp "babylontower/pkg/app"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// contactDetailState holds widget state for the contact detail overlay.
type contactDetailState struct {
	closeBtn        widget.Clickable
	copyPubKeyBtn   widget.Clickable
	copyLinkBtn     widget.Clickable
	renameBtn       widget.Clickable
	removeBtn       widget.Clickable
	findNetworkBtn  widget.Clickable
	renameInput     widget.Editor
	renameSaveBtn   widget.Clickable
	renameCancelBtn widget.Clickable
	// Verification
	copySafetyNumBtn widget.Clickable
	verifyBtn        widget.Clickable
	copyFingerprintBtn widget.Clickable
	// State
	showRename    bool
	renameError   string
	copyFeedback  string
	findStatus    string // "" | "Searching..." | "Found!" | "Not found"
	confirmRemove bool
}

// layoutContactDetail renders the contact detail overlay.
func (a *App) layoutContactDetail(gtx layout.Context) layout.Dimensions {
	contact := a.selectedContactInfo()
	if contact == nil {
		return layout.Dimensions{}
	}

	// Semi-transparent backdrop
	paint.FillShape(gtx.Ops, color.NRGBA{A: 160},
		clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(480)
		if gtx.Constraints.Max.X < maxW {
			maxW = gtx.Constraints.Max.X - gtx.Dp(32)
		}
		gtx.Constraints.Min.X = maxW
		gtx.Constraints.Max.X = maxW

		return a.layoutDialogCard(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// Header with close button
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								l := material.H6(a.theme, "CONTACT INFO")
								l.Color = color.NRGBA(a.ui.theme.Accent)
								l.Font.Weight = font.Bold
								return l.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &a.ui.contactDetail.closeBtn, func(gtx layout.Context) layout.Dimensions {
									l := material.Body1(a.theme, "\u2715") // ✕
									l.Color = color.NRGBA(a.ui.theme.TextSecondary)
									l.TextSize = unit.Sp(18)
									return l.Layout(gtx)
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.drawDecorativeLine(gtx, 200)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

					// Verified badge + Online status
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							// Verified badge
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !a.isContactVerified(contact.PublicKeyBase58) {
									return layout.Dimensions{}
								}
								return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return a.layoutVerifiedBadge(gtx)
								})
							}),
							// Online dot
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								statusColor := color.NRGBA(a.ui.theme.TextSecondary)
								if contact.IsOnline {
									statusColor = color.NRGBA(a.ui.theme.Success)
								}
								radius := gtx.Dp(5)
								size := image.Point{X: radius * 2, Y: radius * 2}
								defer clip.Ellipse{Max: size}.Push(gtx.Ops).Pop()
								paint.FillShape(gtx.Ops, statusColor, clip.Ellipse{Max: size}.Op(gtx.Ops))
								return layout.Dimensions{Size: size}
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								statusText := "Offline"
								statusColor := color.NRGBA(a.ui.theme.TextSecondary)
								if contact.IsOnline {
									statusText = "Online"
									statusColor = color.NRGBA(a.ui.theme.Success)
								}
								return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									l := material.Body2(a.theme, statusText)
									l.Color = statusColor
									return l.Layout(gtx)
								})
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

					// Display Name (with rename)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.ui.contactDetail.showRename {
							return a.layoutRenameInput(gtx)
						}
						return a.layoutNameWithRename(gtx, contact.DisplayName)
					}),

					// Public Key
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutCopyableRow(gtx, "Public Key", truncateKey(contact.PublicKeyBase58, 20), &a.ui.contactDetail.copyPubKeyBtn)
					}),

					// Fingerprint
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						fp, err := babylonapp.ContactFingerprint(contact.PublicKeyBase58, contact.X25519KeyBase58)
						if err != nil {
							return layout.Dimensions{}
						}
						return a.layoutCopyableRow(gtx, "Fingerprint", fp, &a.ui.contactDetail.copyFingerprintBtn)
					}),

					// Peer ID
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						peerID := contact.PeerID
						if peerID == "" {
							peerID = "(not discovered)"
						}
						return a.layoutInfoRow(gtx, "Peer ID", truncateKey(peerID, 24))
					}),

					// Contact Link
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						link := contact.ContactLink
						if link == "" {
							link = "btower://" + contact.PublicKeyBase58
						}
						return a.layoutCopyableRow(gtx, "Contact Link", truncateKey(link, 40), &a.ui.contactDetail.copyLinkBtn)
					}),

					// Safety Number section
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSafetyNumber(gtx, contact)
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

					// Action buttons
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutContactActions(gtx)
					}),

					// Copy/find feedback
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						feedback := a.ui.contactDetail.copyFeedback
						if a.ui.contactDetail.findStatus != "" {
							feedback = a.ui.contactDetail.findStatus
						}
						if feedback == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							feedbackColor := color.NRGBA(a.ui.theme.Success)
							if a.ui.contactDetail.findStatus == "Not found" {
								feedbackColor = color.NRGBA(a.ui.theme.Error)
							}
							l := material.Caption(a.theme, feedback)
							l.Color = feedbackColor
							l.Alignment = text.Middle
							return l.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func (a *App) layoutNameWithRename(gtx layout.Context, name string) layout.Dimensions {
	if name == "" {
		name = "(no name)"
	}
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, "Display Name")
				l.Color = color.NRGBA(a.ui.theme.TextSecondary)
				l.TextSize = unit.Sp(10)
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						l := material.Body1(a.theme, name)
						l.Color = color.NRGBA(a.ui.theme.TextPrimary)
						l.Font.Weight = font.Medium
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &a.ui.contactDetail.renameBtn, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								l := material.Caption(a.theme, "RENAME")
								l.Color = color.NRGBA(a.ui.theme.Accent)
								l.Font.Weight = font.Medium
								l.TextSize = unit.Sp(10)
								return l.Layout(gtx)
							})
						})
					}),
				)
			}),
		)
	})
}

func (a *App) layoutRenameInput(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, "New Display Name")
				l.Color = color.NRGBA(a.ui.theme.TextSecondary)
				l.TextSize = unit.Sp(10)
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						inputStyle := DarkClayTablet()
						inputStyle.CornerRadius = 4
						inputStyle.ShadowOffset = 1
						inputStyle.Padding = layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(6), Bottom: unit.Dp(6)}
						return ClayTablet(inputStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							ed := material.Editor(a.theme, &a.ui.contactDetail.renameInput, "Enter new name...")
							ed.Color = color.NRGBA(a.ui.theme.TextPrimary)
							ed.HintColor = color.NRGBA(a.ui.theme.TextSecondary)
							ed.TextSize = unit.Sp(13)
							return ed.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return material.Clickable(gtx, &a.ui.contactDetail.renameSaveBtn, func(gtx layout.Context) layout.Dimensions {
								l := material.Caption(a.theme, "SAVE")
								l.Color = color.NRGBA(a.ui.theme.Success)
								l.Font.Weight = font.Medium
								l.TextSize = unit.Sp(10)
								return l.Layout(gtx)
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return material.Clickable(gtx, &a.ui.contactDetail.renameCancelBtn, func(gtx layout.Context) layout.Dimensions {
								l := material.Caption(a.theme, "CANCEL")
								l.Color = color.NRGBA(a.ui.theme.TextSecondary)
								l.Font.Weight = font.Medium
								l.TextSize = unit.Sp(10)
								return l.Layout(gtx)
							})
						})
					}),
				)
			}),
			// Rename error
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if a.ui.contactDetail.renameError == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(a.theme, a.ui.contactDetail.renameError)
					l.Color = color.NRGBA(a.ui.theme.Error)
					l.TextSize = unit.Sp(10)
					return l.Layout(gtx)
				})
			}),
		)
	})
}

func (a *App) layoutContactActions(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Spacing: layout.SpaceSides, Alignment: layout.Middle}.Layout(gtx,
		// Find on Network
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(a.theme, &a.ui.contactDetail.findNetworkBtn, "Find on Network")
			btn.Background = color.NRGBA(a.ui.theme.Primary)
			btn.Color = color.NRGBA(a.ui.theme.TextPrimary)
			btn.CornerRadius = unit.Dp(6)
			btn.TextSize = unit.Sp(12)
			btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
			return btn.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		// Verify / Unverify contact
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			contact := a.selectedContactInfo()
			if contact == nil {
				return layout.Dimensions{}
			}
			isVerified := a.isContactVerified(contact.PublicKeyBase58)
			label := "Mark Verified"
			btnBg := color.NRGBA(a.ui.theme.Success)
			btnBg.A = 40
			btnFg := color.NRGBA(a.ui.theme.Success)
			if isVerified {
				label = "Unverify"
				btnBg = color.NRGBA(a.ui.theme.TextSecondary)
				btnBg.A = 30
				btnFg = color.NRGBA(a.ui.theme.TextSecondary)
			}
			btn := material.Button(a.theme, &a.ui.contactDetail.verifyBtn, label)
			btn.Background = btnBg
			btn.Color = btnFg
			btn.CornerRadius = unit.Dp(6)
			btn.TextSize = unit.Sp(12)
			btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
			return btn.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		// Remove Contact
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "Remove Contact"
			btnColor := color.NRGBA(a.ui.theme.Error)
			if a.ui.contactDetail.confirmRemove {
				label = "Confirm Remove?"
				btnColor.A = 255
			}
			btn := material.Button(a.theme, &a.ui.contactDetail.removeBtn, label)
			btn.Background = color.NRGBA{R: btnColor.R, G: btnColor.G, B: btnColor.B, A: 40}
			btn.Color = btnColor
			btn.CornerRadius = unit.Dp(6)
			btn.TextSize = unit.Sp(12)
			btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
			return btn.Layout(gtx)
		}),
	)
}

// layoutSafetyNumber renders the safety number section in contact detail.
func (a *App) layoutSafetyNumber(gtx layout.Context, contact *babylonapp.ContactInfo) layout.Dimensions {
	info := a.getIdentityInfo()
	if info.pubKeyBase58 == "" || contact.PublicKeyBase58 == "" {
		return layout.Dimensions{}
	}

	safetyNum, err := babylonapp.SafetyNumber(info.pubKeyBase58, contact.PublicKeyBase58)
	if err != nil {
		return layout.Dimensions{}
	}

	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, "SAFETY NUMBER")
						l.Color = color.NRGBA(a.ui.theme.Accent)
						l.Font.Weight = font.Medium
						l.TextSize = unit.Sp(10)
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &a.ui.contactDetail.copySafetyNumBtn, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, "COPY")
							l.Color = color.NRGBA(a.ui.theme.Accent)
							l.Font.Weight = font.Medium
							l.TextSize = unit.Sp(10)
							return l.Layout(gtx)
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				inputStyle := DarkClayTablet()
				inputStyle.CornerRadius = 4
				inputStyle.ShadowOffset = 1
				inputStyle.Padding = layout.UniformInset(unit.Dp(10))
				return ClayTablet(inputStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(a.theme, safetyNum)
					l.Color = color.NRGBA(a.ui.theme.TextPrimary)
					l.TextSize = unit.Sp(12)
					l.Alignment = text.Middle
					return l.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(a.theme, "Compare this number with your contact in person or via a trusted channel")
					l.Color = color.NRGBA(a.ui.theme.TextSecondary)
					l.TextSize = unit.Sp(9)
					return l.Layout(gtx)
				})
			}),
		)
	})
}

// layoutVerifiedBadge renders a gold "Verified" badge.
func (a *App) layoutVerifiedBadge(gtx layout.Context) layout.Dimensions {
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			rr := gtx.Dp(3)
			badgeColor := a.ui.theme.Accent
			badgeColor.A = 50
			rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
			paint.FillShape(gtx.Ops, badgeColor, rect.Op(gtx.Ops))
			return layout.Dimensions{Size: sz}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, "\u2713 Verified")
				l.Color = color.NRGBA(a.ui.theme.Accent)
				l.Font.Weight = font.Bold
				l.TextSize = unit.Sp(10)
				return l.Layout(gtx)
			})
		}),
	)
}

// isContactVerified checks if a contact is marked as verified.
func (a *App) isContactVerified(pubKeyBase58 string) bool {
	if a.coreApp == nil || a.coreApp.Storage() == nil {
		return false
	}
	val, err := a.coreApp.Storage().GetConfig("verified:" + pubKeyBase58)
	return err == nil && val == "true"
}

// setContactVerified marks or unmarks a contact as verified.
func (a *App) setContactVerified(pubKeyBase58 string, verified bool) {
	if a.coreApp == nil || a.coreApp.Storage() == nil {
		return
	}
	val := "false"
	if verified {
		val = "true"
	}
	if err := a.coreApp.Storage().SetConfig("verified:"+pubKeyBase58, val); err != nil {
		logger.Warnw("failed to save verification status", "error", err)
	}
}
