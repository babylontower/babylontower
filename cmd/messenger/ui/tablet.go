package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// ClayTabletStyle defines the visual style of a clay tablet.
type ClayTabletStyle struct {
	// Base clay color (warm terracotta)
	Clay color.NRGBA
	// Darker shadow/edge color
	Shadow color.NRGBA
	// Lighter highlight on top surface
	Highlight color.NRGBA
	// Thin border/crack line color
	Border color.NRGBA
	// Inner surface (slightly lighter than clay for depth)
	Surface color.NRGBA
	// Corner radius
	CornerRadius unit.Dp
	// Shadow offset in dp (depth effect)
	ShadowOffset unit.Dp
	// Padding inside the tablet
	Padding layout.Inset
}

// LightClayTablet returns a style for light/warm environments.
func LightClayTablet() ClayTabletStyle {
	return ClayTabletStyle{
		Clay:         color.NRGBA{R: 194, G: 164, B: 120, A: 255}, // Warm terracotta clay
		Shadow:       color.NRGBA{R: 130, G: 105, B: 72, A: 180},  // Dark clay shadow
		Highlight:    color.NRGBA{R: 220, G: 200, B: 170, A: 100}, // Sun-baked highlight
		Border:       color.NRGBA{R: 150, G: 120, B: 80, A: 80},   // Dried crack edge
		Surface:      color.NRGBA{R: 208, G: 182, B: 143, A: 255}, // Slightly lighter face
		CornerRadius: 6,
		ShadowOffset: 3,
		Padding:      layout.UniformInset(unit.Dp(10)),
	}
}

// DarkClayTablet returns a style for dark/night environments.
func DarkClayTablet() ClayTabletStyle {
	return ClayTabletStyle{
		Clay:         color.NRGBA{R: 72, G: 58, B: 42, A: 255},    // Dark fired clay
		Shadow:       color.NRGBA{R: 36, G: 28, B: 18, A: 200},    // Deep shadow
		Highlight:    color.NRGBA{R: 100, G: 82, B: 60, A: 80},    // Moonlit edge
		Border:       color.NRGBA{R: 90, G: 72, B: 50, A: 70},     // Subtle crack
		Surface:      color.NRGBA{R: 82, G: 66, B: 48, A: 255},    // Lighter face
		CornerRadius: 6,
		ShadowOffset: 3,
		Padding:      layout.UniformInset(unit.Dp(10)),
	}
}

// GoldInscriptionTablet returns a dark clay tablet with gold-toned surface,
// suitable for mnemonic words and important inscriptions.
func GoldInscriptionTablet() ClayTabletStyle {
	return ClayTabletStyle{
		Clay:         color.NRGBA{R: 48, G: 38, B: 26, A: 255},    // Very dark clay base
		Shadow:       color.NRGBA{R: 24, G: 18, B: 10, A: 220},    // Near-black shadow
		Highlight:    color.NRGBA{R: 80, G: 65, B: 42, A: 90},     // Warm highlight
		Border:       color.NRGBA{R: 140, G: 110, B: 50, A: 60},   // Faint gold edge
		Surface:      color.NRGBA{R: 58, G: 46, B: 32, A: 255},    // Dark clay face
		CornerRadius: 5,
		ShadowOffset: 2,
		Padding:      layout.UniformInset(unit.Dp(8)),
	}
}

// ClayTablet renders content inside a clay tablet shape.
// The tablet has layered depth: shadow → clay body → surface highlight → content.
func ClayTablet(style ClayTabletStyle) ClayTabletWidget {
	return ClayTabletWidget{Style: style}
}

// ClayTabletWidget is the renderable clay tablet.
type ClayTabletWidget struct {
	Style ClayTabletStyle
}

// Layout renders the clay tablet with the given content inside.
func (t ClayTabletWidget) Layout(gtx layout.Context, content layout.Widget) layout.Dimensions {
	s := t.Style
	shadowOff := gtx.Dp(s.ShadowOffset)
	rr := gtx.Dp(s.CornerRadius)

	return layout.Stack{}.Layout(gtx,
		// Layer 0: Shadow (offset down-right for depth)
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			rect := clip.RRect{
				Rect: image.Rect(shadowOff, shadowOff, sz.X+shadowOff, sz.Y+shadowOff),
				NE:   rr, NW: rr, SE: rr, SW: rr,
			}
			paint.FillShape(gtx.Ops, s.Shadow, rect.Op(gtx.Ops))
			return layout.Dimensions{Size: sz}
		}),

		// Layer 1: Main clay body
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			rect := clip.RRect{
				Rect: image.Rect(0, 0, sz.X, sz.Y),
				NE:   rr, NW: rr, SE: rr, SW: rr,
			}
			paint.FillShape(gtx.Ops, s.Clay, rect.Op(gtx.Ops))
			return layout.Dimensions{Size: sz}
		}),

		// Layer 2: Border stroke (thin crack-like edge)
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			borderRect := clip.RRect{
				Rect: image.Rect(0, 0, sz.X, sz.Y),
				NE:   rr, NW: rr, SE: rr, SW: rr,
			}
			paint.FillShape(gtx.Ops, s.Border,
				clip.Stroke{Path: borderRect.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: sz}
		}),

		// Layer 5: Content with padding
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return s.Padding.Layout(gtx, content)
		}),
	)
}
