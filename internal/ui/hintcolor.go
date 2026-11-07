package ui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
)

const hintSep = " | "

type hintKind int

const (
	hintDefault hintKind = iota
	hintCreate
	hintDestroy
)

var hintStyles = map[hintKind]lipgloss.Style{
	hintDefault: lipgloss.NewStyle().Foreground(lipgloss.Blue),
	hintCreate:  lipgloss.NewStyle().Foreground(lipgloss.Green),
	hintDestroy: lipgloss.NewStyle().Foreground(lipgloss.Red),
}

func kindOf(b binding) hintKind {
	switch {
	case b.keys[0] == &km.Delete:
		return hintDestroy
	case slices.Contains(keys(&km.New, &km.Add, &km.NewUnsaved, &km.SaveAs, &km.ImportCurl), b.keys[0]):
		return hintCreate
	}
	return hintDefault
}

func styledHintLine(bs []binding, w int) string {
	if n := len(bs); n > 1 && bs[n-1].keys[0] == &km.Help {
		tail := hintText(bs[n-1])
		room := w - displayCells(hintSep) - displayCells(tail)
		if room > 0 {
			return fitHints(bs[:n-1], room) + mutedStyle.Render(hintSep) + hintStyles[hintDefault].Render(tail)
		}
	}
	return fitHints(bs, w)
}

func fitHints(bs []binding, w int) string {
	sep := mutedStyle.Render(hintSep)
	sepW := displayCells(hintSep)
	var b strings.Builder
	used := 0
	for i, bd := range bs {
		text := cleanText(hintText(bd))
		gap, gapW := "", 0
		if i > 0 {
			gap, gapW = sep, sepW
		}
		style := hintStyles[kindOf(bd)]
		if used+gapW+displayCells(text) <= w {
			b.WriteString(gap + style.Render(text))
			used += gapW + displayCells(text)
			continue
		}

		if room := w - used - gapW; room > 0 {
			b.WriteString(gap + style.Render(truncate(text, room)))
		}
		break
	}
	return b.String()
}
