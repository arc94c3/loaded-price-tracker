package banner

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"golang.org/x/term"
)

var Lines = []string{
	"  ██╗      ██████╗  █████╗ ██████╗ ███████╗██████╗ ",
	"  ██║     ██╔═══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗",
	"  ██║     ██║   ██║███████║██║  ██║█████╗  ██║  ██║",
	"  ██║     ██║   ██║██╔══██║██║  ██║██╔══╝  ██║  ██║",
	"  ███████╗╚██████╔╝██║  ██║██████╔╝███████╗██████╔╝",
	"  ╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═════╝ ╚══════╝╚═════╝ ",
}

const (
	Subtitle = "  price monitor · go"
	Tagline  = "  > track game key prices on loaded.com"
)

func colorEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if _, ok := os.LookupEnv("FORCE_COLOR"); ok {
		return true
	}
	if _, ok := os.LookupEnv("CLICOLOR_FORCE"); ok {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func lerp(a, b uint8, t float64) uint8 {
	v := float64(a) + (float64(b)-float64(a))*t
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v + 0.5)
}

func Print() {
	enabled := colorEnabled()
	color.NoColor = !enabled
	fmt.Println()
	n := len(Lines)
	for i, line := range Lines {
		if enabled {
			t := 0.0
			if n > 1 {
				t = float64(i) / float64(n-1)
			}
			r := lerp(0xff, 0xff, t)
			g := lerp(0x2e, 0xa8, t)
			b := lerp(0x2e, 0x3c, t)
			color.RGB(int(r), int(g), int(b)).Add(color.Bold).Println(line)
		} else {
			fmt.Println(line)
		}
	}
	if enabled {
		color.RGB(0x94, 0xa3, 0xb8).Add(color.Italic).Println(Subtitle)
		color.RGB(0x64, 0x74, 0x8b).Println(Tagline)
	} else {
		fmt.Println(Subtitle)
		fmt.Println(Tagline)
	}
	fmt.Println()
}
