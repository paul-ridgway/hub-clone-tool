package hct

import (
	"fmt"
	"os"
)

const (
	reset        = "\x1b[0m"
	inverse      = "\x1b[7m"
	black        = "\x1b[30m"
	redBright    = "\x1b[91m"
	greenBright  = "\x1b[92m"
	yellowBright = "\x1b[93m"
	bgRed        = "\x1b[101m"
	bgGreen      = "\x1b[102m"
	bgYellow     = "\x1b[103m"
)

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func useColor(f *os.File) bool {
	return os.Getenv("NO_COLOR") == "" && isTTY(f)
}

func paint(f *os.File, codes, s string) string {
	if !useColor(f) {
		return s
	}
	return codes + s + reset
}

func prefix(f *os.File, bg, level string) string {
	return paint(f, inverse, "Hub Clone Tool ") + paint(f, bg+black, " "+level+" ") + " "
}

func info(format string, a ...any) {
	fmt.Fprintln(os.Stdout, prefix(os.Stdout, bgGreen, "I")+paint(os.Stdout, greenBright, fmt.Sprintf(format, a...)))
}

func warn(format string, a ...any) {
	fmt.Fprintln(os.Stdout, prefix(os.Stdout, bgYellow, "W")+paint(os.Stdout, yellowBright, fmt.Sprintf(format, a...)))
}

func logError(format string, a ...any) {
	fmt.Fprintln(os.Stderr, prefix(os.Stderr, bgRed, "E")+paint(os.Stderr, redBright, fmt.Sprintf(format, a...)))
}
