package theme

import (
	"image/color"
	"math"
	"reflect"
	"testing"
)

func TestParseHex(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  color.RGBA
		valid bool
	}{
		{"#000000", color.RGBA{0, 0, 0, 255}, true},
		{"#ffffff", color.RGBA{255, 255, 255, 255}, true},
		{"#22d3ee", color.RGBA{34, 211, 238, 255}, true},
		{"#12345", color.RGBA{}, false},
		{"#1234567", color.RGBA{}, false},
		{"#gg0000", color.RGBA{}, false},
		{"ffffff", color.RGBA{}, false},
	} {
		got, err := ParseHex(tc.input)
		if (err == nil) != tc.valid || tc.valid && got != tc.want {
			t.Errorf("%q: %+v, %v", tc.input, got, err)
		}
	}
}

func TestContrast(t *testing.T) {
	black, _ := ParseHex("#000000")
	white, _ := ParseHex("#ffffff")
	if got := Contrast(black, white); math.Abs(got-21) > 1e-9 {
		t.Errorf("black/white: %v", got)
	}
	if got := Contrast(white, white); math.Abs(got-1) > 1e-9 {
		t.Errorf("same: %v", got)
	}
}

func TestValidateRole(t *testing.T) {
	black, _ := ParseHex("#000000")
	theme := Theme{Background: black}
	for _, tc := range []struct {
		role, hex string
		valid     bool
	}{
		{"text", "#5a5a5a", true},  // 3.044:1
		{"text", "#595959", false}, // 2.998:1
		{"dim", "#595959", false},
		{"accent", "#595959", false},
		{"success", "#595959", false},
		{"error", "#595959", false},
		{"warning", "#595959", false},
		{"border", "#000000", true},
		{"unknown", "#ffffff", false},
		{"text", "no", false},
	} {
		err := ValidateRole(theme, tc.role, tc.hex)
		if (err == nil) != tc.valid {
			t.Errorf("%s %s: %v", tc.role, tc.hex, err)
		}
	}
}

func TestBuiltins(t *testing.T) {
	if !reflect.DeepEqual(Names(), []string{"green", "violet", "mono", "contrast", "auto", "carbon"}) {
		t.Fatalf("names: %v", Names())
	}
	for _, name := range Names() {
		for _, dark := range []bool{false, true} {
			theme, ok := ByName(name, dark)
			if !ok {
				t.Fatalf("missing %s", name)
			}
			for _, role := range []string{"text", "dim", "accent", "success", "error", "warning"} {
				c, ok := theme.Lookup(role)
				if !ok || Contrast(c, theme.Background) < 4.5 {
					t.Errorf("%s dark=%v %s contrast=%v", name, dark, role, Contrast(c, theme.Background))
				}
			}
		}
	}
	if _, ok := ByName("invalid", true); ok {
		t.Error("unknown theme accepted")
	}
}

func TestCarbonPaletteAndSurface(t *testing.T) {
	want := Theme{
		Background: mustHex("#1E1F22"), Surface: mustHex("#26282D"),
		Accent: mustHex("#50FA7B"), Success: mustHex("#50FA7B"),
		Warning: mustHex("#FFB86C"), Text: mustHex("#F8F8F2"),
		Dim: mustHex("#a3a3a3"), Border: mustHex("#a3a3a3"), Error: mustHex("#FF5555"),
	}
	for _, dark := range []bool{false, true} {
		got, ok := ByName("carbon", dark)
		if !ok || got != want {
			t.Errorf("dark=%v: %+v, %v", dark, got, ok)
		}
		for _, role := range []string{"text", "dim", "accent", "success", "error", "warning"} {
			c, _ := got.Lookup(role)
			for _, bg := range []color.RGBA{got.Background, got.Surface} {
				if ratio := Contrast(c, bg); ratio < 4.5 {
					t.Errorf("%s dark=%v on %v: contrast %v", role, dark, bg, ratio)
				}
			}
		}
	}
	for _, name := range []string{"green", "violet", "mono", "contrast", "auto"} {
		for _, dark := range []bool{false, true} {
			got, _ := ByName(name, dark)
			if got.Surface != (color.RGBA{}) {
				t.Errorf("%s dark=%v paints surface", name, dark)
			}
		}
	}
	if _, ok := want.Lookup("surface"); ok {
		t.Error("surface became an editable role")
	}
}

func TestCustomPreservesBaseAndSkipsInvalid(t *testing.T) {
	base, _ := ByName("green", true)
	changed := Custom(base, map[string]string{"accent": "#ffffff", "unknown": "#ffffff", "text": "invalid"})
	white, _ := ParseHex("#ffffff")
	if changed.Accent != white || changed.Text != base.Text || changed.Background != base.Background {
		t.Fatalf("custom: %+v", changed)
	}
}
