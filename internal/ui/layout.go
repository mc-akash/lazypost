package ui

import (
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/mc-akash/lazypost/internal/bodyview"
	"github.com/mc-akash/lazypost/internal/history"
)

const (
	minWidth      = 80
	minHeight     = 20
	minLeftWidth  = 26
	leftPercent   = 30
	reqPercent    = 40
	footerLines   = 2
	minPanelLines = 3
	boxPad        = 1
	tooSmallMsg   = "terminal too small (need 80×20)"
	ellipsis      = "…"
)

var (
	accent          = lipgloss.Blue
	plainStyle      = lipgloss.NewStyle()
	focusColor      = lipgloss.Green
	focusBorder     = lipgloss.NewStyle().Foreground(focusColor)
	focusTitle      = lipgloss.NewStyle().Foreground(focusColor).Bold(true)
	dimBorder       = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	mutedStyle      = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	selFocusStyle   = lipgloss.NewStyle().Reverse(true)
	selUnfocusStyle = lipgloss.NewStyle().Bold(true)
	activeEnvStyle  = lipgloss.NewStyle().Foreground(lipgloss.Green)
)

type geometry struct {
	leftW, rightW      int
	envH, collH, histH int
	reqH, respH        int
	panelsH            int
}

func computeGeometry(w, h, envCount int) geometry {
	ph := h - footerLines
	g := geometry{panelsH: ph}
	g.leftW = max(minLeftWidth, w*leftPercent/100)
	g.rightW = w - g.leftW
	g.envH = max(minPanelLines, min(envCount+2, ph/4))
	g.histH = max(minPanelLines, ph/4)
	g.collH = ph - g.envH - g.histH
	g.reqH = ph * reqPercent / 100
	g.respH = ph - g.reqH
	return g
}

func (m Model) Render() string {
	if m.width < minWidth || m.height < minHeight {
		return tooSmallMsg
	}
	ws := m.ws()
	g := computeGeometry(m.width, m.height, len(ws.Envs))

	inner := func(w, h int) (int, int) { return w - 2 - boxPad, h - 2 }

	ew, eh := inner(g.leftW, g.envH)
	cw, ch := inner(g.leftW, g.collH)
	hw, hh := inner(g.leftW, g.histH)
	left := strings.Join([]string{
		box("[1] Environments", m.environments.render(ws.Envs, m.envName, ew, eh, m.focus == PanelEnvs), g.leftW, g.envH, m.focus == PanelEnvs),
		box("[2] Collections", m.collections.render(ws.Collections, cw, ch, m.focus == PanelCollections, m.treeDecor), g.leftW, g.collH, m.focus == PanelCollections),
		box("[3] History", m.history.render(time.Now(), hw, hh, m.focus == PanelHistory), g.leftW, g.histH, m.focus == PanelHistory),
	}, "\n")
	var right string
	if m.showsVars() {
		vw, vh := inner(g.rightW, g.panelsH)
		right = box(m.varsTitle(), m.varsLines(vw, vh), g.rightW, g.panelsH, m.focus == PanelVars)
	} else {
		rw, _ := inner(g.rightW, g.reqH)
		_, rh := inner(g.rightW, g.respH)
		resp := m.currentResponse()
		right = strings.Join([]string{
			box(m.requestTitle(), m.requestLines(rw), g.rightW, g.reqH, m.focus == PanelRequest),
			box(responseTitleBar(resp), m.responseBody(resp, rw, rh), g.rightW, g.respH, m.focus == PanelResponse),
		}, "\n")
	}

	screen := strings.Join([]string{
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		footerLine(m.status, m.width),
		" " + styledHintLine(m.hints(), m.width-1),
	}, "\n")
	if m.overlay != nil {
		screen = m.overlay.draw(m, screen)
	}
	return screen
}

func footerLine(text string, w int) string {
	return truncate(" "+text, w)
}

func box(title string, body []string, w, h int, focused bool) string {
	inner := w - 2
	border, titleStyle := dimBorder, plainStyle
	if focused {
		border, titleStyle = focusBorder, focusTitle
	}
	t := truncate(title, inner)
	lines := make([]string, 0, h)
	lines = append(lines, border.Render("┌")+titleStyle.Render(t)+border.Render(strings.Repeat("─", inner-displayCells(t))+"┐"))
	side := border.Render("│")
	for i := 0; i < h-2; i++ {
		row := strings.Repeat(" ", boxPad)
		if i < len(body) {
			row += body[i]
		}
		lines = append(lines, side+row+strings.Repeat(" ", max(0, inner-displayCells(row)))+side)
	}
	lines = append(lines, border.Render("└"+strings.Repeat("─", inner)+"┘"))
	return strings.Join(lines, "\n")
}

func selectedRow(text string, w int, focused bool) string {
	t := truncate(text, w)
	padded := t + strings.Repeat(" ", max(0, w-displayCells(t)))
	if focused {
		return selFocusStyle.Render(padded)
	}
	return selUnfocusStyle.Render(padded)
}

func methodLabel(method string) string {
	short := map[string]string{"DELETE": "DEL", "PATCH": "PTCH", "OPTIONS": "OPTS"}
	m := strings.ToUpper(cleanText(method))
	if s, ok := short[m]; ok {
		m = s
	}
	if len(m) < 4 {
		m += strings.Repeat(" ", 4-len(m))
	}
	return m
}

func methodStyle(method string) lipgloss.Style {
	colors := map[string]color.Color{
		"GET": lipgloss.Green, "POST": lipgloss.Yellow, "PUT": lipgloss.Blue,
		"PATCH": lipgloss.Magenta, "DELETE": lipgloss.Red,
	}
	c, ok := colors[strings.ToUpper(method)]
	if !ok {
		c = lipgloss.Cyan
	}
	return lipgloss.NewStyle().Foreground(c)
}

func statusStyle(e history.Entry) lipgloss.Style {
	switch {
	case e.Err != "" || e.Code == 0 || e.Code >= 500:
		return lipgloss.NewStyle().Foreground(lipgloss.Red)
	case e.Code >= 400:
		return lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	case e.Code >= 300:
		return lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Green)
}

func displayCells(s string) int { return lipgloss.Width(s) }

func cleanText(s string) string { return bodyview.Sanitize(s) }

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = cleanText(s)
	if displayCells(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := displayCells(string(r))
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + ellipsis
}

func clampCursor(c, n int) int {
	if n <= 0 {
		return 0
	}
	return max(0, min(c, n-1))
}

func step(cur, delta, n int) int { return clampCursor(clampCursor(cur, n)+delta, n) }

func listWindow(n, cursor, h int) (int, int) {
	if h <= 0 || n == 0 {
		return 0, 0
	}
	start := 0
	if cursor >= h {
		start = cursor - h + 1
	}
	return start, min(n, start+h)
}
