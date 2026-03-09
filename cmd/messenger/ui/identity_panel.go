package ui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// identityPanelState holds widget state for the "My Identity" overlay.
type identityPanelState struct {
	copyPubKeyBtn     widget.Clickable
	copyFingerprintBtn widget.Clickable
	copyContactLinkBtn widget.Clickable
	closeBtn          widget.Clickable
	// Feedback messages (shown briefly after copy)
	copyFeedback string
}

// layoutIdentityPanel renders the "My Identity" overlay.
func (a *App) layoutIdentityPanel(gtx layout.Context) layout.Dimensions {
	info := a.getIdentityInfo()

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
					// Header
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								l := material.H6(a.theme, "MY IDENTITY")
								l.Color = color.NRGBA(a.ui.theme.Accent)
								l.Font.Weight = font.Bold
								return l.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &a.ui.identityPanel.closeBtn, func(gtx layout.Context) layout.Dimensions {
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
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

					// Display Name
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutInfoRow(gtx, "Display Name", info.displayName)
					}),

					// Public Key (truncated with copy)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutCopyableRow(gtx, "Public Key", truncateKey(info.pubKeyBase58, 16), &a.ui.identityPanel.copyPubKeyBtn)
					}),

					// Peer ID
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutInfoRow(gtx, "Peer ID", truncateKey(info.peerID, 20))
					}),

					// Fingerprint with copy
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutCopyableRow(gtx, "Fingerprint", info.fingerprint, &a.ui.identityPanel.copyFingerprintBtn)
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

					// Contact Link section
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(a.theme, "CONTACT LINK")
						l.Color = color.NRGBA(a.ui.theme.Accent)
						l.Font.Weight = font.Medium
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutCopyableField(gtx, info.contactLink, &a.ui.identityPanel.copyContactLinkBtn)
					}),

					// Copy feedback
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.ui.identityPanel.copyFeedback == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, a.ui.identityPanel.copyFeedback)
							l.Color = color.NRGBA(a.ui.theme.Success)
							l.Alignment = text.Middle
							return l.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

// identityDisplayInfo is a simple DTO for template rendering.
type identityDisplayInfo struct {
	displayName  string
	pubKeyBase58 string
	peerID       string
	fingerprint  string
	contactLink  string
}

func (a *App) getIdentityInfo() identityDisplayInfo {
	info := identityDisplayInfo{
		displayName:  "Initializing...",
		pubKeyBase58: "",
		peerID:       "",
		fingerprint:  "",
		contactLink:  "",
	}
	if a.coreApp == nil {
		return info
	}
	ident := a.coreApp.GetIdentity()
	if ident == nil {
		return info
	}
	info.displayName = ident.DisplayName
	if info.displayName == "" {
		info.displayName = "Babylon User"
	}
	if ident.PublicKeyBase58 != "" {
		info.pubKeyBase58 = ident.PublicKeyBase58
	}
	if ident.PeerID != "" {
		info.peerID = ident.PeerID
	}
	if ident.Fingerprint != "" {
		info.fingerprint = ident.Fingerprint
	}
	info.contactLink = ident.ContactLink
	if info.contactLink == "" && ident.PublicKeyBase58 != "" {
		info.contactLink = "btower://" + ident.PublicKeyBase58
	}
	return info
}

// ── Reusable info row layouts ───────────────────────────────────────────────

func (a *App) layoutInfoRow(gtx layout.Context, label, value string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, label)
				l.Color = color.NRGBA(a.ui.theme.TextSecondary)
				l.TextSize = unit.Sp(10)
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(a.theme, value)
				l.Color = color.NRGBA(a.ui.theme.TextPrimary)
				return l.Layout(gtx)
			}),
		)
	})
}

func (a *App) layoutCopyableRow(gtx layout.Context, label, value string, btn *widget.Clickable) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(a.theme, label)
				l.Color = color.NRGBA(a.ui.theme.TextSecondary)
				l.TextSize = unit.Sp(10)
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						l := material.Body2(a.theme, value)
						l.Color = color.NRGBA(a.ui.theme.TextPrimary)
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								l := material.Caption(a.theme, "COPY")
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

func (a *App) layoutCopyableField(gtx layout.Context, value string, btn *widget.Clickable) layout.Dimensions {
	inputStyle := DarkClayTablet()
	inputStyle.CornerRadius = 4
	inputStyle.ShadowOffset = 1
	inputStyle.Padding = layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(8), Bottom: unit.Dp(8)}

	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ClayTablet(inputStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				display := value
				if len(display) > 50 {
					display = display[:47] + "..."
				}
				l := material.Caption(a.theme, display)
				l.Color = color.NRGBA(a.ui.theme.TextPrimary)
				l.TextSize = unit.Sp(11)
				return l.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(a.theme, "COPY")
					l.Color = color.NRGBA(a.ui.theme.Accent)
					l.Font.Weight = font.Medium
					l.TextSize = unit.Sp(10)
					return l.Layout(gtx)
				})
			})
		}),
	)
}

// truncateKey truncates a key string to show first and last chars.
func truncateKey(key string, maxLen int) string {
	if key == "" {
		return "(unknown)"
	}
	if len(key) <= maxLen {
		return key
	}
	half := (maxLen - 3) / 2
	return key[:half] + "..." + key[len(key)-half:]
}
