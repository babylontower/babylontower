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
	"gioui.org/widget/material"
)

// layoutAddContactDialog renders a modal overlay dialog for adding a contact.
func (a *App) layoutAddContactDialog(gtx layout.Context) layout.Dimensions {
	// Semi-transparent overlay
	overlayColor := color.NRGBA{R: 0, G: 0, B: 0, A: 160}
	paint.FillShape(gtx.Ops, overlayColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	// Center the dialog card
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(420)
		if gtx.Constraints.Max.X < maxW {
			maxW = gtx.Constraints.Max.X - gtx.Dp(32)
		}
		gtx.Constraints.Min.X = maxW
		gtx.Constraints.Max.X = maxW

		return a.layoutDialogCard(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					// Title
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.H6(a.theme, "ADD CONTACT")
						l.Color = color.NRGBA(a.ui.theme.Accent)
						l.Font.Weight = font.Bold
						l.Alignment = text.Middle
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),

					// Decorative line
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.drawDecorativeLine(gtx, 140)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

					// Instructions
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Body2(a.theme, "Enter a btower:// contact link or a public key (hex or base58)")
						l.Color = color.NRGBA(a.ui.theme.TextSecondary)
						l.Alignment = text.Middle
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

					// Input field in clay tablet
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						inputStyle := DarkClayTablet()
						inputStyle.CornerRadius = 5
						inputStyle.ShadowOffset = 1
						inputStyle.Padding = layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(10), Bottom: unit.Dp(10)}
						return ClayTablet(inputStyle).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							ed := material.Editor(a.theme, &a.ui.addContactInput, "btower://... or public key")
							ed.Color = color.NRGBA(a.ui.theme.TextPrimary)
							ed.HintColor = color.NRGBA(a.ui.theme.TextSecondary)
							ed.TextSize = unit.Sp(13)
							return ed.Layout(gtx)
						})
					}),

					// Error message
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.ui.addContactError == "" {
							return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx)
						}
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(a.theme, a.ui.addContactError)
							l.Color = color.NRGBA(a.ui.theme.Error)
							l.Alignment = text.Middle
							return l.Layout(gtx)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

					// Buttons row
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Spacing: layout.SpaceSides}.Layout(gtx,
							// Cancel
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := material.Button(a.theme, &a.ui.addContactCancel, "Cancel")
								btn.Background = color.NRGBA(a.ui.theme.Surface)
								btn.Color = color.NRGBA(a.ui.theme.TextSecondary)
								btn.CornerRadius = unit.Dp(6)
								btn.Inset = layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(24), Right: unit.Dp(24)}
								return btn.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							// Add
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := material.Button(a.theme, &a.ui.addContactSubmit, "Add Contact")
								btn.Background = color.NRGBA(a.ui.theme.Primary)
								btn.Color = color.NRGBA(a.ui.theme.TextPrimary)
								btn.CornerRadius = unit.Dp(6)
								btn.Inset = layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(24), Right: unit.Dp(24)}
								return btn.Layout(gtx)
							}),
						)
					}),
				)
			})
		})
	})
}

// layoutDialogCard renders a dark clay card with gold border (like onboarding cards).
func (a *App) layoutDialogCard(gtx layout.Context, content layout.Widget) layout.Dimensions {
	surfaceColor := a.ui.theme.BackgroundCity
	borderColor := a.ui.theme.Accent
	borderColor.A = 80

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			rr := gtx.Dp(8)
			rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
			paint.FillShape(gtx.Ops, surfaceColor, rect.Op(gtx.Ops))
			// Gold border
			borderRect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
			paint.FillShape(gtx.Ops, borderColor, clip.Stroke{Path: borderRect.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: sz}
		}),
		layout.Stacked(content),
	)
}
