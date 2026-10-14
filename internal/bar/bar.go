// Package bar formats workspace names and agent status logos for status bars.
package bar

import (
	"fmt"
	"sort"
	"strings"

	"github.com/murrayb52/flockr/internal/model"
	"github.com/murrayb52/flockr/internal/ui"
)

type Style int

// An urgent workspace flashes between a solid green block and green text,
// swapping every flashFrames frames (frames tick every 250ms).
const (
	colFlash     = "#4ade80"
	colFlashText = "#0d1f14"
	flashFrames  = 2
)

const (
	Polybar Style = iota
	Plain
)

// Format renders one line for the bar; frame animates the "working" spinner.
func Format(s model.Snapshot, style Style, frame int) string {
	ws := append([]model.Workspace(nil), s.Workspaces...)
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].Num < ws[j].Num })
	parts := make([]string, 0, len(ws))
	for _, w := range ws {
		label := fmt.Sprint(w.Num)
		name := w.WSLabel
		if name == "" {
			name = w.Name
		}
		if name != "" {
			label += " " + name
		}
		glyph, colour := ui.Logo(ui.WorkspaceStatus(w), frame)
		if style == Plain {
			if glyph != "" {
				label += " " + glyph
			}
			if w.Focused {
				label = "[" + label + "]"
			}
			parts = append(parts, label)
			continue
		}
		urgent := !w.Focused && (w.Urgent || ui.WorkspaceStatus(w) == model.StatusInput)
		label = " " + escape(label) + " "
		switch {
		case glyph == "":
		case urgent: // the glyph takes the flash colour
			label += glyph + " "
		default:
			label += "%{F" + colour + "}" + glyph + "%{F-} "
		}
		switch {
		case w.Focused:
			label = "%{B#262b33}%{u#40916c}%{+u}%{F" + ui.ColText + "}" + label + "%{F-}%{-u}%{B-}"
		case urgent && frame/flashFrames%2 == 0:
			label = "%{B" + colFlash + "}%{F" + colFlashText + "}" + label + "%{F-}%{B-}"
		case urgent:
			label = "%{F" + colFlash + "}" + label + "%{F-}"
		default:
			label = "%{F" + ui.ColDim + "}" + label + "%{F-}"
		}
		parts = append(parts, fmt.Sprintf("%%{A1:i3-msg workspace number %d:}%s%%{A}", w.Num, label))
	}
	if style == Plain {
		return strings.Join(parts, " · ")
	}
	return strings.Join(parts, "")
}

// escape stops workspace names from injecting polybar formatting tags by
// breaking up "%{" (a lossy but safe choice for an absurd edge case).
func escape(s string) string { return strings.ReplaceAll(s, "%{", "% {") }
