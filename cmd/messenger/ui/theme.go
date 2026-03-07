// Package ui provides the Gio-based graphical user interface for Babylon Tower.
package ui

import (
	"image/color"

	"gioui.org/unit"
)

// ColorPalette contains the Babylonian-themed color palette
type ColorPalette struct {
	// Primary colors
	Primary        color.NRGBA // Lapis lazuli blue
	PrimaryVariant color.NRGBA // Turquoise glaze
	Accent         color.NRGBA // Hammered gold

	// Background colors
	BackgroundCity color.NRGBA // Warm sandstone (light) / Deep brown-black (dark)
	BackgroundSky  color.NRGBA // Desert sky blue (light) / Night sky (dark)
	Surface        color.NRGBA // Pale clay (light) / Dark clay (dark)

	// Text colors
	TextPrimary   color.NRGBA // Dark umber (light) / Pale sand (dark)
	TextSecondary color.NRGBA // Warm grey (light) / Muted sand (dark)

	// Status colors
	Success color.NRGBA // Palm green (light) / Soft green (dark)
	Error   color.NRGBA // Terracotta red (light) / Warm red (dark)

	// Additional UI colors
	Divider    color.NRGBA
	Highlight  color.NRGBA
	Background color.NRGBA
}

// LightTheme returns the daylight Babylon theme
func LightTheme() *ColorPalette {
	return &ColorPalette{
		// Primary colors - Ishtar Gate inspired
		Primary:        color.NRGBA{R: 30, G: 58, B: 95, A: 255},   // #1E3A5F - Lapis lazuli blue
		PrimaryVariant: color.NRGBA{R: 46, G: 139, B: 139, A: 255}, // #2E8B8B - Turquoise glaze
		Accent:         color.NRGBA{R: 197, G: 150, B: 26, A: 255}, // #C5961A - Hammered gold

		// Background colors - Desert Babylon
		BackgroundCity: color.NRGBA{R: 232, G: 213, B: 183, A: 255}, // #E8D5B7 - Warm sandstone
		BackgroundSky:  color.NRGBA{R: 245, G: 230, B: 202, A: 255}, // #F5E6CA - Light desert sky
		Surface:        color.NRGBA{R: 242, G: 232, B: 213, A: 255}, // #F2E8D5 - Pale clay

		// Text colors
		TextPrimary:   color.NRGBA{R: 44, G: 24, B: 16, A: 255},  // #2C1810 - Dark umber
		TextSecondary: color.NRGBA{R: 107, G: 91, B: 79, A: 255}, // #6B5B4F - Warm grey

		// Status colors
		Success: color.NRGBA{R: 74, G: 124, B: 63, A: 255}, // #4A7C3F - Palm green
		Error:   color.NRGBA{R: 181, G: 69, B: 58, A: 255}, // #B5453A - Terracotta red

		// Additional UI colors
		Divider:    color.NRGBA{R: 189, G: 168, B: 142, A: 255}, // Muted clay
		Highlight:  color.NRGBA{R: 197, G: 150, B: 26, A: 51},   // Gold with 20% opacity
		Background: color.NRGBA{R: 232, G: 213, B: 183, A: 255}, // Same as BackgroundCity
	}
}

// DarkTheme returns the night Babylon theme
func DarkTheme() *ColorPalette {
	return &ColorPalette{
		// Primary colors - Moonlit Ishtar Gate
		Primary:        color.NRGBA{R: 74, G: 127, B: 191, A: 255}, // #4A7FBF - Bright lapis
		PrimaryVariant: color.NRGBA{R: 58, G: 175, B: 175, A: 255}, // #3AAFAF - Teal glaze
		Accent:         color.NRGBA{R: 212, G: 168, B: 67, A: 255}, // #D4A843 - Warm gold

		// Background colors - Night Babylon
		BackgroundCity: color.NRGBA{R: 26, G: 20, B: 16, A: 255}, // #1A1410 - Deep brown-black
		BackgroundSky:  color.NRGBA{R: 10, G: 14, B: 26, A: 255}, // #0A0E1A - Night sky
		Surface:        color.NRGBA{R: 42, G: 34, B: 24, A: 255}, // #2A2218 - Dark clay

		// Text colors
		TextPrimary:   color.NRGBA{R: 232, G: 213, B: 183, A: 255}, // #E8D5B7 - Pale sand
		TextSecondary: color.NRGBA{R: 139, G: 125, B: 107, A: 255}, // #8B7D6B - Muted sand

		// Status colors
		Success: color.NRGBA{R: 90, G: 156, B: 79, A: 255},  // #5A9C4F - Soft green
		Error:   color.NRGBA{R: 207, G: 107, B: 90, A: 255}, // #CF6B5A - Warm red

		// Additional UI colors
		Divider:    color.NRGBA{R: 74, G: 66, B: 54, A: 255},  // Dark clay
		Highlight:  color.NRGBA{R: 212, G: 168, B: 67, A: 51}, // Gold with 20% opacity
		Background: color.NRGBA{R: 26, G: 20, B: 16, A: 255},  // Same as BackgroundCity
	}
}

// Theme holds the current theme configuration
type Theme struct {
	*ColorPalette
	Font unit.Sp
}

// NewTheme creates a new theme with the given color palette
func NewTheme(palette *ColorPalette) *Theme {
	return &Theme{
		ColorPalette: palette,
		Font:         14,
	}
}
