package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// palette is one variant (light or dark) of the app's color system. Kept as
// a small named set rather than scattering literals through theme.Color, so
// the two variants stay visibly parallel and easy to tune together.
type palette struct {
	background    color.Color // window / canvas background
	surface       color.Color // sidebar, preview card, header background
	surfaceBorder color.Color // hairline around surfaces and separators
	foreground    color.Color // primary text
	muted         color.Color // secondary/caption text, placeholders, disabled
	primary       color.Color // accent: primary buttons, focus, links, selected states
	onPrimary     color.Color // text/icon color drawn on top of primary — must be dark: primary is a light gold
	inputBg       color.Color
	inputBorder   color.Color
	disabledBtn   color.Color
	hover         color.Color
	selection     color.Color
	success       color.Color
	warning       color.Color
}

// Swipe direction accents — shared by the category selectors and card drag
// feedback so left/right always read as the same two options.
var (
	swipeLeftAccent  = color.NRGBA{R: 0xB5, G: 0x6F, B: 0x74, A: 0xFF} // dusty rose
	swipeRightAccent = color.NRGBA{R: 0x7A, G: 0x9E, B: 0x7E, A: 0xFF} // sage
)

var lightPalette = palette{
	background:    color.NRGBA{R: 0xF6, G: 0xF6, B: 0xF4, A: 0xFF},
	surface:       color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFE, A: 0xFF},
	surfaceBorder: color.NRGBA{R: 0xE4, G: 0xE4, B: 0xE0, A: 0xFF},
	foreground:    color.NRGBA{R: 0x1B, G: 0x1B, B: 0x1A, A: 0xFF},
	muted:         color.NRGBA{R: 0x73, G: 0x73, B: 0x6E, A: 0xFF},
	primary:       color.NRGBA{R: 0xD6, G: 0xBA, B: 0x7C, A: 0xFF},
	onPrimary:     color.NRGBA{R: 0x24, G: 0x1D, B: 0x10, A: 0xFF},
	inputBg:       color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFE, A: 0xFF},
	inputBorder:   color.NRGBA{R: 0xD7, G: 0xD7, B: 0xD2, A: 0xFF},
	disabledBtn:   color.NRGBA{R: 0xED, G: 0xED, B: 0xEA, A: 0xFF},
	hover:         color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x0A},
	selection:     color.NRGBA{R: 0xD6, G: 0xBA, B: 0x7C, A: 0x40},
	success:       color.NRGBA{R: 0x7A, G: 0x9E, B: 0x7E, A: 0xFF},
	warning:       color.NRGBA{R: 0xD9, G: 0x77, B: 0x06, A: 0xFF},
}

// Dark: warm charcoal, no blue cast. deep = window, elevated = cards/sidebar.
var darkPalette = palette{
	background:    color.NRGBA{R: 0x0F, G: 0x0F, B: 0x0F, A: 0xFF}, // deep
	surface:       color.NRGBA{R: 0x1C, G: 0x1C, B: 0x1A, A: 0xFF}, // elevated
	surfaceBorder: color.NRGBA{R: 0x2A, G: 0x2A, B: 0x27, A: 0xFF},
	foreground:    color.NRGBA{R: 0xF0, G: 0xF0, B: 0xEE, A: 0xFF},
	muted:         color.NRGBA{R: 0x9A, G: 0x9A, B: 0x94, A: 0xFF},
	primary:       color.NRGBA{R: 0xD6, G: 0xBA, B: 0x7C, A: 0xFF},
	onPrimary:     color.NRGBA{R: 0x24, G: 0x1D, B: 0x10, A: 0xFF},
	inputBg:       color.NRGBA{R: 0x22, G: 0x22, B: 0x20, A: 0xFF},
	inputBorder:   color.NRGBA{R: 0x33, G: 0x33, B: 0x30, A: 0xFF},
	disabledBtn:   color.NRGBA{R: 0x26, G: 0x26, B: 0x24, A: 0xFF},
	hover:         color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x10},
	selection:     color.NRGBA{R: 0xD6, G: 0xBA, B: 0x7C, A: 0x40},
	success:       color.NRGBA{R: 0x7A, G: 0x9E, B: 0x7E, A: 0xFF},
	warning:       color.NRGBA{R: 0xF5, G: 0x9E, B: 0x0B, A: 0xFF},
}

func paletteFor(variant fyne.ThemeVariant) palette {
	if variant == theme.VariantDark {
		return darkPalette
	}
	return lightPalette
}

// appTheme is a thin skin over Fyne's default theme: it overrides color and
// spacing/radius tokens for a calmer, more deliberately spaced look, and
// otherwise defers to the default for fonts, icons, and anything unnamed
// here so unrelated widgets keep behaving normally.
type appTheme struct{}

func newAppTheme() fyne.Theme { return appTheme{} }

func (appTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	p := paletteFor(variant)
	switch name {
	case theme.ColorNameBackground:
		return p.background
	case theme.ColorNameHeaderBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return p.surface
	case theme.ColorNameForeground:
		return p.foreground
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return p.muted
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return p.primary
	case theme.ColorNameForegroundOnPrimary:
		return p.onPrimary
	case theme.ColorNameButton:
		return p.surface
	case theme.ColorNameDisabledButton:
		return p.disabledBtn
	case theme.ColorNameInputBackground:
		return p.inputBg
	case theme.ColorNameInputBorder:
		return p.inputBorder
	case theme.ColorNameSeparator:
		return p.surfaceBorder
	case theme.ColorNameHover:
		return p.hover
	case theme.ColorNameSelection:
		return p.selection
	case theme.ColorNameSuccess:
		return p.success
	case theme.ColorNameWarning:
		return p.warning
	case theme.ColorNameScrollBar:
		return p.muted
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (appTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (appTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (appTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 10
	case theme.SizeNameInnerPadding:
		return 16
	case theme.SizeNameLineSpacing:
		return 6
	case theme.SizeNameHeadingText:
		return 26
	case theme.SizeNameSubHeadingText:
		return 18
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameInputRadius:
		return 8
	case theme.SizeNameButtonRadius:
		return 10
	case theme.SizeNameCardRadius:
		return 14
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameScrollBarSmall:
		return 4
	}
	return theme.DefaultTheme().Size(name)
}
