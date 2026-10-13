// Package fontwidth measures how wide text renders in i3's title font, so a
// title can be padded out to put text at the right edge of its title bar.
package fontwidth

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// Measure returns the width of s in pixels, in the bold face if bold.
type Measure func(s string, bold bool) float64

// Mono measures every character as px wide.
func Mono(px float64) Measure {
	return func(s string, _ bool) float64 { return px * float64(utf8.RuneCountInString(s)) }
}

// FromI3Config measures with the font of the last `font pango:FAMILIES SIZE`
// line of an i3 config, found through fontconfig. SIZE is in points (at 96
// DPI) or, with a px suffix, pixels.
func FromI3Config(conf string) (Measure, error) {
	families, px := "", 0.0
	for _, line := range strings.Split(conf, "\n") {
		if f, p, ok := parseFont(line); ok {
			families, px = f, p
		}
	}
	if families == "" {
		return nil, fmt.Errorf("no pango font in the i3 config")
	}
	regular, err := load(families, "", px)
	if err != nil {
		return nil, err
	}
	bold, err := load(families, ":weight=bold", px)
	if err != nil {
		bold = regular
	}
	return func(s string, b bool) float64 {
		if b {
			return bold.width(s)
		}
		return regular.width(s)
	}, nil
}

func parseFont(line string) (families string, px float64, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(line), "font pango:")
	if !found {
		return "", 0, false
	}
	i := strings.LastIndexAny(rest, " \t")
	if i < 0 {
		return "", 0, false
	}
	size := rest[i+1:]
	isPx := strings.HasSuffix(size, "px")
	v, err := strconv.ParseFloat(strings.TrimSuffix(size, "px"), 64)
	if err != nil || v <= 0 {
		return "", 0, false
	}
	if !isPx {
		v = v * 96 / 72
	}
	return strings.TrimSpace(rest[:i]), v, true
}

type face struct {
	f     font.Face
	fnt   *sfnt.Font
	other fixedWidth // for characters the font lacks (pango takes them from another font)
}

type fixedWidth float64

func load(families, style string, px float64) (*face, error) {
	pattern := strings.ReplaceAll(families, ", ", ",") + style
	out, err := exec.Command("fc-match", "-f", "%{file}", pattern).Output()
	if err != nil {
		return nil, fmt.Errorf("fc-match %q: %v", pattern, err)
	}
	b, err := os.ReadFile(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, err
	}
	fnt, err := opentype.Parse(b)
	if err != nil {
		return nil, err
	}
	f, err := opentype.NewFace(fnt, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	return &face{f: f, fnt: fnt, other: fixedWidth(px * 0.6)}, nil
}

func (f *face) width(s string) float64 {
	var w float64
	var buf sfnt.Buffer
	known := make([]rune, 0, len(s))
	for _, r := range s {
		if i, err := f.fnt.GlyphIndex(&buf, r); err != nil || i == 0 {
			w += float64(f.other)
			continue
		}
		known = append(known, r)
	}
	return w + float64(font.MeasureString(f.f, string(known)))/64
}
