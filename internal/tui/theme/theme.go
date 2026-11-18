// Package theme defines readable interface palettes and custom color validation.
package theme

import (
	"encoding/hex"
	"fmt"
	"image/color"
	"math"
)

// Theme contains seven editable foreground roles and additional display properties.
type Theme struct {
	Accent, Text, Dim, Success, Error, Warning, Border color.RGBA
	Info, Special, Muted                               color.RGBA
	Background, Surface, Raised                        color.RGBA
}

// Lookup returns a named role's color.
func (t Theme) Lookup(role string) (color.RGBA, bool) {
	switch role {
	case "accent":
		return t.Accent, true
	case "text":
		return t.Text, true
	case "dim":
		return t.Dim, true
	case "success":
		return t.Success, true
	case "error":
		return t.Error, true
	case "warning":
		return t.Warning, true
	case "border":
		return t.Border, true
	}
	return color.RGBA{}, false
}

// Names lists the built-in themes in display order.
func Names() []string { return []string{"green", "violet", "mono", "contrast", "auto", "carbon"} }

// ParseHex parses an opaque #rrggbb color.
func ParseHex(s string) (color.RGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	b, err := hex.DecodeString(s[1:])
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid hex color %q: %w", s, err)
	}
	return color.RGBA{R: b[0], G: b[1], B: b[2], A: 255}, nil
}

func mustHex(s string) color.RGBA { c, _ := ParseHex(s); return c }

// ByName returns a built-in palette. Auto follows the supplied dark background hint.
func ByName(name string, dark bool) (Theme, bool) {
	if name == "carbon" {
		return Theme{
			Background: mustHex("#1E1F22"), Surface: mustHex("#26282D"),
			Accent: mustHex("#50FA7B"), Success: mustHex("#50FA7B"),
			Warning: mustHex("#FFB86C"), Text: mustHex("#F8F8F2"),
			Dim: mustHex("#a3a3a3"), Border: mustHex("#a3a3a3"), Error: mustHex("#FF5555"),
			Info: mustHex("#8be9fd"), Special: mustHex("#bd93f9"), Muted: mustHex("#8b92a5"), Raised: mustHex("#2A2D34"),
		}, true
	}
	if name == "auto" {
		name = "green"
	}
	if name != "green" && name != "violet" && name != "mono" && name != "contrast" {
		return Theme{}, false
	}
	var t Theme
	if dark {
		t = Theme{
			Background: mustHex("#000000"), Text: mustHex("#f5f5f5"), Dim: mustHex("#a3a3a3"),
			Accent: mustHex("#68b78d"), Success: mustHex("#4ade80"), Error: mustHex("#fb7185"),
			Warning: mustHex("#facc15"), Border: mustHex("#a3a3a3"),
		}
		switch name {
		case "violet":
			t.Accent = mustHex("#c4b5fd")
		case "mono":
			t.Accent = mustHex("#ffffff")
			t.Success = t.Text
			t.Error = t.Text
			t.Warning = t.Text
		case "contrast":
			t.Text = mustHex("#ffffff")
			t.Dim = mustHex("#ffffff")
			t.Accent = mustHex("#ffffff")
			t.Border = t.Text
		}
	} else {
		t = Theme{
			Background: mustHex("#ffffff"), Text: mustHex("#171717"), Dim: mustHex("#525252"),
			Accent: mustHex("#1f6b47"), Success: mustHex("#166534"), Error: mustHex("#be123c"),
			Warning: mustHex("#854d0e"), Border: mustHex("#525252"),
		}
		switch name {
		case "violet":
			t.Accent = mustHex("#6d28d9")
		case "mono":
			t.Accent = mustHex("#000000")
			t.Success = t.Text
			t.Error = t.Text
			t.Warning = t.Text
		case "contrast":
			t.Background = mustHex("#000000")
			t.Text = mustHex("#ffffff")
			t.Dim = t.Text
			t.Accent = t.Text
			t.Success = mustHex("#4ade80")
			t.Error = mustHex("#fb7185")
			t.Warning = mustHex("#facc15")
			t.Border = t.Text
		}
	}
	t.Info, t.Special, t.Muted = t.Accent, t.Accent, t.Dim
	return t, true
}

// Custom applies valid hexadecimal role overrides while retaining the base background.
func Custom(base Theme, colors map[string]string) Theme {
	for role, hex := range colors {
		c, err := ParseHex(hex)
		if err != nil {
			continue
		}
		switch role {
		case "accent":
			base.Accent = c
		case "text":
			base.Text = c
		case "dim":
			base.Dim = c
		case "success":
			base.Success = c
		case "error":
			base.Error = c
		case "warning":
			base.Warning = c
		case "border":
			base.Border = c
		}
	}
	base.Info, base.Special, base.Muted = base.Accent, base.Accent, base.Dim
	base.Surface, base.Raised = color.RGBA{}, color.RGBA{}
	return base
}

func linear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return 0.2126*linear(float64(r)/65535) + 0.7152*linear(float64(g)/65535) + 0.0722*linear(float64(b)/65535)
}

// Contrast computes the WCAG relative luminance contrast ratio.
func Contrast(a, b color.Color) float64 {
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// ValidateRole rejects unknown roles, malformed colors and foregrounds below 3:1.
func ValidateRole(t Theme, role, hex string) error {
	if _, ok := t.Lookup(role); !ok {
		return fmt.Errorf("unknown color role %q", role)
	}
	c, err := ParseHex(hex)
	if err != nil {
		return err
	}
	if role != "border" && Contrast(c, t.Background) < 3 {
		return fmt.Errorf("%s contrast below 3:1", role)
	}
	return nil
}
