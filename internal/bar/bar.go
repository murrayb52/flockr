// Package bar formats workspace names and agent status logos for status bars.
package bar

import (
	"fmt"
	"sort"
	"strings"

	"github.com/murrayb52/flock/internal/model"
	"github.com/murrayb52/flock/internal/ui"
)

type Style int

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
		if w.Name != "" {
			label += " " + w.Name
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
		label = " " + escape(label) + " "
		if glyph != "" {
			label += "%{F" + colour + "}" + glyph + "%{F-} "
		}
		switch {
		case w.Focused:
			label = "%{B#262b33}%{u#40916c}%{+u}%{F" + ui.ColText + "}" + label + "%{F-}%{-u}%{B-}"
		case w.Urgent || ui.WorkspaceStatus(w) == model.StatusInput:
			label = "%{F#ff9a94}" + label + "%{F-}"
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
