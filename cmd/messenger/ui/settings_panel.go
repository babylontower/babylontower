package ui

import (
	"gioui.org/layout"
)

// layoutSettingsPanel renders the settings panel using the reusable SettingsScreen.
func (a *App) layoutSettingsPanel(gtx layout.Context) layout.Dimensions {
	if a.ui.settingsScreen == nil {
		return layout.Dimensions{}
	}
	return a.ui.settingsScreen.Layout(gtx, a.theme)
}
