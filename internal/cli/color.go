package cli

import (
	"os"
	"strings"
)

var (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorGray    = "\033[90m"

	// Icon (symbol) constants – keep ASCII fallbacks for non-Unicode terminals if needed in future.
	IconSuccess = "✓"
	IconError   = "✗"
	IconPrompt  = "➤"
	IconInfo    = "ℹ"
)

func init() {
	if disableColor() {
		ColorReset = ""
		ColorBold = ""
		ColorDim = ""
		ColorRed = ""
		ColorGreen = ""
		ColorYellow = ""
		ColorBlue = ""
		ColorMagenta = ""
		ColorCyan = ""
		ColorGray = ""
	}
}

func disableColor() bool {
	if os.Getenv("AIC_NO_COLOR") != "" || os.Getenv("NO_COLOR") != "" { return true }
	fi, err := os.Stdout.Stat()
	if err != nil { return false }
	// if not a character device assume no color
	return (fi.Mode() & os.ModeCharDevice) == 0 || strings.Contains(strings.ToLower(os.Getenv("TERM")), "dumb")
}
