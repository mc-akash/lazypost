package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	dialogPercent  = 60
	dialogMinWidth = 40
	dialogMaxWidth = 80
)

const (
	dialogNewRequest  = "New request"
	dialogNewEnv      = "New environment"
	dialogSaveAs      = "Save as"
	dialogEditURL     = "Edit URL"
	dialogEditHeader  = "Edit header"
	dialogEditParam   = "Edit param"
	dialogAddHeader   = "Add header"
	dialogAddParam    = "Add param"
	dialogEditPrefix  = "Edit "
	dialogAddVar      = "Add variable"
	dialogVarValue    = "Variable value"
	dialogPasteValue  = "Paste value"
	dialogPasteCurl   = "Paste cURL"
	dialogRequestName = "Request name"
	dialogSecret      = "Secret?"
	dialogDelRequest  = "Delete request"
	dialogDelFolder   = "Delete folder"
	dialogDelEnv      = "Delete environment"
	dialogDelVar      = "Delete variable"

	hintNewItem   = "end with / to create a folder"
	hintSaveAs    = "folder/name"
	hintKV        = "key: value"
	hintVarName   = "name"
	hintKeepValue = "leave empty to keep the current value"
	hintCurl      = "curl command"
)

func dialogWidth(w int) int {
	return min(w, max(dialogMinWidth, min(dialogMaxWidth, w*dialogPercent/100)))
}

func dialog(screen string, w, h int, title string, lines func(int) []string) string {
	bw := dialogWidth(w)
	body := lines(bw - 2 - boxPad)
	bh := min(len(body)+2, h)
	return centered(screen, box(title, body, bw, bh, true), w, h, bw, bh)
}

func centered(screen, popup string, w, h, bw, bh int) string {
	canvas := lipgloss.NewCanvas(w, h)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(screen),
		lipgloss.NewLayer(popup).X((w-bw)/2).Y((h-bh)/2).Z(1),
	))
	return canvas.Render()
}

func (in inputModel) dialogLines(w int) []string {
	var lines []string
	if in.hint != "" {
		lines = append(lines, mutedStyle.Render(truncate(in.hint, w)))
	}
	return append(lines, in.view(w), dialogFooter(in.bindings(), w))
}

func (p keyPrompt) dialogLines(w int) []string {
	var lines []string
	for _, l := range strings.Split(p.message, "\n") {
		for _, wl := range strings.Split(lipgloss.Wrap(cleanText(l), w, ""), "\n") {
			lines = append(lines, truncate(wl, w))
		}
	}
	return append(lines, dialogFooter(p.bindings, w))
}

func dialogFooter(bs []binding, w int) string {
	return mutedStyle.Render(truncate(hintLine(hinted(bs)), w))
}
