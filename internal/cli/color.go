package cli

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/orochaa/go-clack/third_party/picocolors"
)

func init() {
	configureNoColor()
}

func configureNoColor() {
	if os.Getenv("NO_COLOR") == "" {
		return
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	// go-clack's picocolors fork can enable color before checking NO_COLOR.
	disableClackColors()
}

func disableClackColors() {
	plain := func(input string) string { return input }
	picocolors.Color = map[string]func(string) string{
		"reset":         plain,
		"bold":          plain,
		"dim":           plain,
		"italic":        plain,
		"underline":     plain,
		"inverse":       plain,
		"hidden":        plain,
		"strikethrough": plain,
		"black":         plain,
		"red":           plain,
		"green":         plain,
		"yellow":        plain,
		"blue":          plain,
		"magenta":       plain,
		"cyan":          plain,
		"white":         plain,
		"gray":          plain,
		"bgBlack":       plain,
		"bgRed":         plain,
		"bgGreen":       plain,
		"bgYellow":      plain,
		"bgBlue":        plain,
		"bgMagenta":     plain,
		"bgCyan":        plain,
		"bgWhite":       plain,
	}
	picocolors.Reset = plain
	picocolors.Bold = plain
	picocolors.Dim = plain
	picocolors.Italic = plain
	picocolors.Underline = plain
	picocolors.Inverse = plain
	picocolors.Hidden = plain
	picocolors.Strikethrough = plain
	picocolors.Black = plain
	picocolors.Red = plain
	picocolors.Green = plain
	picocolors.Yellow = plain
	picocolors.Blue = plain
	picocolors.Magenta = plain
	picocolors.Cyan = plain
	picocolors.White = plain
	picocolors.Gray = plain
	picocolors.BgBlack = plain
	picocolors.BgRed = plain
	picocolors.BgGreen = plain
	picocolors.BgYellow = plain
	picocolors.BgBlue = plain
	picocolors.BgMagenta = plain
	picocolors.BgCyan = plain
	picocolors.BgWhite = plain
}
