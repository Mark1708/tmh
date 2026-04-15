package ui

// Screen identifies which view the app is currently rendering.
type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenDiff
	ScreenPalette
	ScreenHelp
	ScreenError
	ScreenEmpty
	ScreenConfirm
)
