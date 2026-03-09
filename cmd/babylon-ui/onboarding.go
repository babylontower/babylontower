package main

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	btapp "babylontower/pkg/app"
	"babylontower/cmd/messenger/ui"
	"babylontower/pkg/identity"
)

// onboardingScreen identifies which screen is active.
type onboardingScreen int

const (
	screenWelcome onboardingScreen = iota
	screenCreate
	screenRestore
	screenConfirm
	screenConfig
)

// onboardingResult is returned when onboarding completes.
type onboardingResult struct {
	identity    *identity.Identity
	mnemonic    string
	displayName string
	deviceName  string
}

// onboardingState holds the full onboarding UI state.
type onboardingState struct {
	screen onboardingScreen

	// Theme
	theme    *material.Theme
	palette  *onboardingPalette
	darkMode bool

	// Welcome screen
	createBtn  widget.Clickable
	restoreBtn widget.Clickable

	// Create screen
	generatedMnemonic string
	generatedResult   *btapp.NewIdentityResult
	mnemonicCopyBtn   widget.Clickable
	confirmBackupBtn  widget.Clickable
	createBackBtn     widget.Clickable

	// Restore screen
	mnemonicEditor   widget.Editor
	restoreSubmitBtn widget.Clickable
	restoreBackBtn   widget.Clickable
	restoreError     string
	restoredResult   *btapp.NewIdentityResult

	// Confirm screen
	confirmContinueBtn widget.Clickable

	// Config screen (reusable settings)
	settingsScreen *ui.SettingsScreen
	enterBtn       widget.Clickable

	// Scroll
	scrollList widget.List

	// Result channel
	done   bool
	result *onboardingResult
}

// onboardingPalette holds colors for the onboarding screens (Babylonian theme).
type onboardingPalette struct {
	bgGradientTop    color.NRGBA // Night sky / desert sky
	bgGradientBottom color.NRGBA // Sandstone ground
	surface          color.NRGBA // Card background
	surfaceBorder    color.NRGBA // Card border
	primary          color.NRGBA // Lapis lazuli
	accent           color.NRGBA // Gold
	textTitle        color.NRGBA // Bright text
	textBody         color.NRGBA // Body text
	textMuted        color.NRGBA // Secondary text
	textOnPrimary    color.NRGBA // Text on blue buttons
	success          color.NRGBA // Green
	errorColor       color.NRGBA // Red
	mnemonicBg       color.NRGBA // Mnemonic box background
}

func darkOnboardingPalette() *onboardingPalette {
	return &onboardingPalette{
		bgGradientTop:    color.NRGBA{R: 8, G: 12, B: 24, A: 255},    // Deep night sky
		bgGradientBottom: color.NRGBA{R: 36, G: 28, B: 20, A: 255},   // Dark sandstone
		surface:          color.NRGBA{R: 30, G: 26, B: 22, A: 230},   // Dark clay card
		surfaceBorder:    color.NRGBA{R: 197, G: 150, B: 26, A: 60},  // Gold border
		primary:          color.NRGBA{R: 30, G: 58, B: 95, A: 255},   // Lapis lazuli
		accent:           color.NRGBA{R: 197, G: 150, B: 26, A: 255}, // Hammered gold
		textTitle:        color.NRGBA{R: 232, G: 213, B: 183, A: 255}, // Pale sand
		textBody:         color.NRGBA{R: 200, G: 185, B: 165, A: 255}, // Warm sand
		textMuted:        color.NRGBA{R: 139, G: 125, B: 107, A: 255}, // Muted sand
		textOnPrimary:    color.NRGBA{R: 232, G: 213, B: 183, A: 255},
		success:          color.NRGBA{R: 90, G: 156, B: 79, A: 255},
		errorColor:       color.NRGBA{R: 207, G: 107, B: 90, A: 255},
		mnemonicBg:       color.NRGBA{R: 20, G: 16, B: 12, A: 255},
	}
}

func newOnboardingState() *onboardingState {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	pal := darkOnboardingPalette()
	th.Bg = pal.bgGradientTop
	th.Fg = pal.textBody
	th.ContrastBg = pal.primary
	th.ContrastFg = pal.textOnPrimary

	s := &onboardingState{
		screen:  screenWelcome,
		theme:   th,
		palette: pal,
	}
	s.mnemonicEditor.SingleLine = false
	s.mnemonicEditor.Submit = false
	s.settingsScreen = ui.NewSettingsScreen(nil, ui.DarkSettingsColors())
	s.settingsScreen.SetActiveTab(ui.TabProfile)
	s.scrollList.Axis = layout.Vertical
	return s
}

// runOnboarding opens the onboarding window and blocks until identity is created.
// Returns nil if the user closes the window without completing.
func runOnboarding() *onboardingResult {
	state := newOnboardingState()

	window := &app.Window{}
	window.Option(app.Size(unit.Dp(600), unit.Dp(700)))
	window.Option(app.MinSize(unit.Dp(480), unit.Dp(600)))
	window.Option(app.Title("Babylon Tower"))

	var ops op.Ops

	for {
		e := window.Event()
		if e == nil {
			continue
		}
		switch e := e.(type) {
		case app.DestroyEvent:
			// Window was closed — return whatever result we have
			return state.result

		case app.FrameEvent:
			gtx := layout.Context{
				Constraints: layout.Constraints{Max: e.Size},
				Metric:      e.Metric,
				Now:         e.Now,
				Source:      e.Source,
				Ops:         &ops,
			}

			state.handleEvents(gtx)
			state.layout(gtx)
			e.Frame(&ops)

			// If onboarding is done, close the window
			if state.done {
				return state.result
			}
		}
	}
}

// handleEvents processes clicks and transitions between screens.
func (s *onboardingState) handleEvents(gtx layout.Context) {
	switch s.screen {
	case screenWelcome:
		if s.createBtn.Clicked(gtx) {
			s.generateIdentity()
			s.screen = screenCreate
		}
		if s.restoreBtn.Clicked(gtx) {
			s.screen = screenRestore
			s.restoreError = ""
		}

	case screenCreate:
		if s.createBackBtn.Clicked(gtx) {
			s.screen = screenWelcome
		}
		if s.confirmBackupBtn.Clicked(gtx) {
			s.screen = screenConfirm
		}

	case screenRestore:
		if s.restoreBackBtn.Clicked(gtx) {
			s.screen = screenWelcome
			s.restoreError = ""
		}
		if s.restoreSubmitBtn.Clicked(gtx) {
			s.tryRestore()
		}

	case screenConfirm:
		if s.confirmContinueBtn.Clicked(gtx) {
			s.screen = screenConfig
		}

	case screenConfig:
		// Let the settings screen handle its own tab clicks, etc.
		s.settingsScreen.Update(gtx)

		if s.enterBtn.Clicked(gtx) {
			r := s.activeResult()
			if r != nil {
				profile := s.settingsScreen.GetProfile()
				s.result = &onboardingResult{
					identity:    r.Identity,
					mnemonic:    r.Mnemonic,
					displayName: profile.DisplayName,
					deviceName:  profile.DeviceName,
				}
				s.done = true
			}
		}
	}
}

func (s *onboardingState) generateIdentity() {
	result, err := btapp.GenerateNewIdentity()
	if err != nil {
		return
	}
	s.generatedResult = result
	s.generatedMnemonic = result.Mnemonic
}

func (s *onboardingState) tryRestore() {
	mnemonic := strings.TrimSpace(s.mnemonicEditor.Text())
	if mnemonic == "" {
		s.restoreError = "Please enter your mnemonic phrase"
		return
	}
	if !btapp.ValidateMnemonic(mnemonic) {
		s.restoreError = "Invalid mnemonic phrase. Please check your words and try again."
		return
	}
	result, err := btapp.RestoreIdentityFromMnemonic(mnemonic)
	if err != nil {
		s.restoreError = "Failed to restore: " + err.Error()
		return
	}
	s.restoredResult = result
	s.restoreError = ""
	s.screen = screenConfirm
}

func (s *onboardingState) activeResult() *btapp.NewIdentityResult {
	if s.restoredResult != nil {
		return s.restoredResult
	}
	return s.generatedResult
}

// ── Layout ──────────────────────────────────────────────────────────────────

func (s *onboardingState) layout(gtx layout.Context) layout.Dimensions {
	// Background gradient
	s.drawBackground(gtx)

	// Center content with max width
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(520)
		if gtx.Constraints.Max.X > maxW {
			gtx.Constraints.Max.X = maxW
		}
		gtx.Constraints.Min.X = gtx.Constraints.Max.X

		switch s.screen {
		case screenWelcome:
			return s.layoutWelcome(gtx)
		case screenCreate:
			return s.layoutCreate(gtx)
		case screenRestore:
			return s.layoutRestore(gtx)
		case screenConfirm:
			return s.layoutConfirm(gtx)
		case screenConfig:
			return s.layoutConfig(gtx)
		}
		return layout.Dimensions{}
	})
}

func (s *onboardingState) drawBackground(gtx layout.Context) {
	// Fill with gradient-like effect: top is night sky, bottom is sandstone
	w := gtx.Constraints.Max.X
	h := gtx.Constraints.Max.Y
	mid := h / 2

	// Top half
	topRect := image.Rect(0, 0, w, mid)
	paint.FillShape(gtx.Ops, s.palette.bgGradientTop, clip.Rect(topRect).Op())

	// Bottom half
	botRect := image.Rect(0, mid, w, h)
	paint.FillShape(gtx.Ops, s.palette.bgGradientBottom, clip.Rect(botRect).Op())
}

// ── Welcome Screen ──────────────────────────────────────────────────────────

func (s *onboardingState) layoutWelcome(gtx layout.Context) layout.Dimensions {
	pal := s.palette

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(layout.Spacer{Height: unit.Dp(60)}.Layout),

		// Decorative line
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.drawDecorativeLine(gtx, 200)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.H3(s.theme, "BABYLON TOWER")
				l.Color = pal.accent
				l.Font.Weight = font.Bold
				l.Alignment = text.Middle
				return l.Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		// Subtitle
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body1(s.theme, "Decentralized Messenger")
				l.Color = pal.textMuted
				l.Alignment = text.Middle
				return l.Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

		// Decorative line
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.drawDecorativeLine(gtx, 200)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(40)}.Layout),

		// Tagline
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(s.theme, "Your identity is the key to the tower.\nCreate a new one, or restore yours from a mnemonic phrase.")
					l.Color = pal.textBody
					l.Alignment = text.Middle
					return l.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(48)}.Layout),

		// Create button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(40), Right: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPrimaryButton(gtx, &s.createBtn, "Create New Identity")
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Restore button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(40), Right: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutSecondaryButton(gtx, &s.restoreBtn, "Restore from Mnemonic")
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(60)}.Layout),

		// Footer
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(s.theme, "End-to-end encrypted  //  Peer-to-peer  //  No servers")
				l.Color = pal.textMuted
				l.Alignment = text.Middle
				return l.Layout(gtx)
			})
		}),
	)
}

// ── Create Screen ───────────────────────────────────────────────────────────

func (s *onboardingState) layoutCreate(gtx layout.Context) layout.Dimensions {
	pal := s.palette

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(layout.Spacer{Height: unit.Dp(40)}.Layout),

		// Back
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutTextButton(gtx, &s.createBackBtn, "<  Back")
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.H5(s.theme, "Your Secret Mnemonic")
				l.Color = pal.accent
				l.Font.Weight = font.Bold
				l.Alignment = text.Middle
				return l.Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.drawDecorativeLine(gtx, 160)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Warning
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(s.theme, "Write down these 12 words in order and store them safely.\nThis is the only way to recover your identity. Never share it.")
					l.Color = pal.textBody
					l.Alignment = text.Middle
					return l.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

		// Mnemonic card
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutMnemonicCard(gtx, s.generatedMnemonic)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(32)}.Layout),

		// Confirm button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(40), Right: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPrimaryButton(gtx, &s.confirmBackupBtn, "I've Written It Down")
				})
			})
		}),
	)
}

// ── Restore Screen ──────────────────────────────────────────────────────────

func (s *onboardingState) layoutRestore(gtx layout.Context) layout.Dimensions {
	pal := s.palette

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(layout.Spacer{Height: unit.Dp(40)}.Layout),

		// Back
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutTextButton(gtx, &s.restoreBackBtn, "<  Back")
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.H5(s.theme, "Restore Your Identity")
				l.Color = pal.accent
				l.Font.Weight = font.Bold
				l.Alignment = text.Middle
				return l.Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.drawDecorativeLine(gtx, 160)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Instructions
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(s.theme, "Enter your 12-word mnemonic phrase to restore your identity.\nWords should be separated by spaces.")
					l.Color = pal.textBody
					l.Alignment = text.Middle
					return l.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

		// Mnemonic input
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutMnemonicInput(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		// Error message
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.restoreError == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(s.theme, s.restoreError)
					l.Color = pal.errorColor
					l.Alignment = text.Middle
					return l.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

		// Restore button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(40), Right: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPrimaryButton(gtx, &s.restoreSubmitBtn, "Restore Identity")
				})
			})
		}),
	)
}

// ── Confirm Screen ──────────────────────────────────────────────────────────

func (s *onboardingState) layoutConfirm(gtx layout.Context) layout.Dimensions {
	pal := s.palette
	r := s.activeResult()
	if r == nil {
		return layout.Dimensions{}
	}

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(layout.Spacer{Height: unit.Dp(50)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.drawDecorativeLine(gtx, 200)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.H4(s.theme, "Identity Ready")
				l.Color = pal.accent
				l.Font.Weight = font.Bold
				l.Alignment = text.Middle
				return l.Layout(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.drawDecorativeLine(gtx, 200)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(32)}.Layout),

		// Identity card
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutIdentityCard(gtx, r)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(40)}.Layout),

		// Continue button
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(40), Right: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPrimaryButton(gtx, &s.confirmContinueBtn, "Continue")
				})
			})
		}),
	)
}

// ── Config Screen (embeds reusable SettingsScreen) ───────────────────────

func (s *onboardingState) layoutConfig(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Settings screen fills available space
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.settingsScreen.Layout(gtx, s.theme)
		}),

		// Enter button at the bottom
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(24), Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(40), Right: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutPrimaryButton(gtx, &s.enterBtn, "Enter the Tower")
					})
				})
			})
		}),
	)
}

// ── Shared Components ───────────────────────────────────────────────────────

func (s *onboardingState) layoutMnemonicCard(gtx layout.Context, mnemonic string) layout.Dimensions {
	pal := s.palette
	words := strings.Fields(mnemonic)
	tabletStyle := ui.GoldInscriptionTablet()

	return s.layoutCard(gtx, func(gtx layout.Context) layout.Dimensions {
		// Background for mnemonic area
		bounds := image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
		paint.FillShape(gtx.Ops, pal.mnemonicBg, clip.Rect(bounds).Op())

		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Layout words as 4 rows x 3 columns of clay tablets
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				s.layoutMnemonicTabletRow(gtx, words, 0, tabletStyle),
				s.layoutMnemonicTabletRow(gtx, words, 3, tabletStyle),
				s.layoutMnemonicTabletRow(gtx, words, 6, tabletStyle),
				s.layoutMnemonicTabletRow(gtx, words, 9, tabletStyle),
			)
		})
	})
}

func (s *onboardingState) layoutMnemonicTabletRow(gtx layout.Context, words []string, startIdx int, style ui.ClayTabletStyle) layout.FlexChild {
	pal := s.palette
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			var children []layout.FlexChild
			for i := startIdx; i < startIdx+3 && i < len(words); i++ {
				idx := i
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						// Force equal width: fill the entire flex slot
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						tablet := ui.ClayTablet(style)
						return tablet.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
								// Number
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										l := material.Caption(s.theme, itoa2(idx+1))
										l.Color = pal.textMuted
										return l.Layout(gtx)
									})
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
								// Word
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										l := material.Body1(s.theme, words[idx])
										l.Color = pal.accent
										l.Font.Weight = font.Bold
										return l.Layout(gtx)
									})
								}),
							)
						})
					})
				}))
			}
			return layout.Flex{}.Layout(gtx, children...)
		})
	})
}

func (s *onboardingState) layoutMnemonicInput(gtx layout.Context) layout.Dimensions {
	pal := s.palette
	return s.layoutCard(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
		paint.FillShape(gtx.Ops, pal.mnemonicBg, clip.Rect(bounds).Op())

		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			ed := material.Editor(s.theme, &s.mnemonicEditor, "Enter your 12 words separated by spaces...")
			ed.Color = pal.accent
			ed.HintColor = pal.textMuted
			ed.TextSize = unit.Sp(16)
			return ed.Layout(gtx)
		})
	})
}

func (s *onboardingState) layoutIdentityCard(gtx layout.Context, r *btapp.NewIdentityResult) layout.Dimensions {
	pal := s.palette

	return s.layoutCard(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Fingerprint
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutInfoRow(gtx, "Fingerprint", r.Fingerprint)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

				// Public Key (truncated)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					pk := r.PublicKeyBase58
					if len(pk) > 20 {
						pk = pk[:10] + "..." + pk[len(pk)-10:]
					}
					return s.layoutInfoRow(gtx, "Public Key", pk)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

				// Contact Link
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					link := btapp.GenerateContactLink(r.Identity.Ed25519PubKey, r.Identity.X25519PubKey, "")
					if len(link) > 35 {
						link = link[:35] + "..."
					}
					return s.layoutInfoRow(gtx, "Contact Link", link)
				}),

				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

				// Note
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(s.theme, "Share your Contact Link with others so they can reach you.")
						l.Color = pal.textMuted
						l.Alignment = text.Middle
						return l.Layout(gtx)
					})
				}),
			)
		})
	})
}

func (s *onboardingState) layoutInfoRow(gtx layout.Context, label, value string) layout.Dimensions {
	pal := s.palette
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Caption(s.theme, label)
			l.Color = pal.textMuted
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body1(s.theme, value)
				l.Color = pal.textTitle
				l.Font.Weight = font.Medium
				return l.Layout(gtx)
			})
		}),
	)
}

// ── Primitives ──────────────────────────────────────────────────────────────

func (s *onboardingState) layoutCard(gtx layout.Context, content layout.Widget) layout.Dimensions {
	pal := s.palette
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Constraints.Min
			rr := gtx.Dp(unit.Dp(8))
			rect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
			paint.FillShape(gtx.Ops, pal.surface, rect.Op(gtx.Ops))
			// Border
			borderRect := clip.RRect{Rect: image.Rect(0, 0, sz.X, sz.Y), NE: rr, NW: rr, SE: rr, SW: rr}
			paint.FillShape(gtx.Ops, pal.surfaceBorder, clip.Stroke{Path: borderRect.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: sz}
		}),
		layout.Stacked(content),
	)
}

func (s *onboardingState) layoutPrimaryButton(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	pal := s.palette
	b := material.Button(s.theme, btn, label)
	b.Background = pal.primary
	b.Color = pal.textOnPrimary
	b.TextSize = unit.Sp(16)
	b.Inset = layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(32), Right: unit.Dp(32)}
	b.CornerRadius = unit.Dp(6)
	return b.Layout(gtx)
}

func (s *onboardingState) layoutSecondaryButton(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	pal := s.palette
	b := material.Button(s.theme, btn, label)
	b.Background = pal.surface
	b.Color = pal.accent
	b.TextSize = unit.Sp(16)
	b.Inset = layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(32), Right: unit.Dp(32)}
	b.CornerRadius = unit.Dp(6)
	return b.Layout(gtx)
}

func (s *onboardingState) layoutTextButton(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	pal := s.palette
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		l := material.Body2(s.theme, label)
		l.Color = pal.textMuted
		return l.Layout(gtx)
	})
}

func (s *onboardingState) drawDecorativeLine(gtx layout.Context, widthDp int) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Dp(unit.Dp(widthDp))
		h := gtx.Dp(unit.Dp(1))
		rect := image.Rect(0, 0, w, h)
		paint.FillShape(gtx.Ops, s.palette.accent, clip.Rect(rect).Op())
		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}

// itoa2 formats a number as a string (simple, small numbers only).
func itoa2(n int) string {
	if n < 10 {
		return string(rune('0'+n%10))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
