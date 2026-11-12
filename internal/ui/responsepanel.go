package ui

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mc-akash/lazypost/internal/bodyview"
	"github.com/mc-akash/lazypost/internal/history"
)

const (
	searchPrompt  = "/"
	statusCopied  = "copied"
	respTabLines  = 1
	borderLines   = 2
	halfPageRatio = 2
)

var respTabs = []string{"Body", "Headers", "Cookies"}

const (
	_ = iota
	respTabHeaders
	respTabCookies
)

var (
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Red)
	jsonPalette = bodyview.Palette{
		Key:    styleFunc(lipgloss.NewStyle().Foreground(accent)),
		String: styleFunc(lipgloss.NewStyle().Foreground(lipgloss.Green)),
		Number: styleFunc(lipgloss.NewStyle().Foreground(lipgloss.Cyan)),
		Bool:   styleFunc(lipgloss.NewStyle().Foreground(lipgloss.Yellow)),
		Null:   styleFunc(mutedStyle),
	}
)

func styleFunc(s lipgloss.Style) func(string) string {
	return func(t string) string { return s.Render(t) }
}

type responsePanel struct {
	key     string
	tab     int
	scroll  int
	matches []int
	match   int
}

type respData struct {
	key    string
	title  string
	err    string
	body   []byte
	header http.Header
}

type bodyMemo struct {
	key   string
	lines []string
}

func (c *bodyMemo) get(key string, render func() []string) []string {
	if c == nil {
		return render()
	}
	if c.key != key || c.lines == nil {
		c.key, c.lines = key, render()
	}
	return c.lines
}

func (m Model) currentResponse() respData {
	sel, ok := m.selection()
	if !ok {
		return respData{}
	}
	key := sel.key()
	if r, ok := m.results[key]; ok {
		d := resultData(key, r)
		if r.phase == phaseSending {
			d.title = m.spin.View() + " " + d.title
		}
		return d
	}
	if sel.entry != nil {
		return entryData(key, *sel.entry)
	}
	return respData{}
}

func resultData(key string, r sendResult) respData {
	switch {
	case r.phase == phaseSending:
		return respData{key: key, title: "sending… (" + km.Return[0] + " to cancel)"}
	case r.phase == phaseCancelled:
		return respData{key: key, title: titleCancelled}
	case r.err != nil:
		return respData{key: key + "#" + strconv.Itoa(r.seq), title: titleError, err: r.err.Error()}
	}
	return respData{
		key:    key + "#" + strconv.Itoa(r.seq),
		title:  responseTitle(r.resp.Status, r.resp.Duration, r.resp.Size, r.resp.Truncated),
		body:   r.resp.Body,
		header: r.resp.Header,
	}
}

func entryData(key string, e history.Entry) respData {
	if e.Err != "" {
		return respData{key: key, title: titleError, err: e.Err}
	}
	status := e.Status
	if status == "" {
		status = fmt.Sprintf("%d %s", e.Code, http.StatusText(e.Code))
	}
	d := time.Duration(e.DurationMS) * time.Millisecond
	return respData{
		key:    key,
		title:  responseTitle(status, d, int64(len(e.Body)), e.Truncated),
		body:   e.Body,
		header: e.Header,
	}
}

func (m Model) respViewFor(d respData) responsePanel {
	if m.response.key != d.key {
		return responsePanel{key: d.key, tab: m.response.tab}
	}
	return m.response
}

func (m Model) respLines(d respData, tab int) []string {
	if d.title == "" {
		return nil
	}
	switch tab {
	case respTabHeaders:
		return headerLines(d.header)
	case respTabCookies:
		return cookieLines(d.header)
	}
	if d.err != "" {
		return []string{errorStyle.Render("error: " + cleanText(d.err))}
	}
	if d.body == nil {
		return nil
	}
	return m.memo.get(d.key, func() []string {
		return bodyview.Render(d.body, d.header.Get("Content-Type"), jsonPalette)
	})
}

func headerLines(h http.Header) []string {
	keys := slices.Sorted(func(yield func(string) bool) {
		for k := range h {
			if !yield(k) {
				return
			}
		}
	})
	var out []string
	for _, k := range keys {
		for _, v := range h[k] {
			out = append(out, cleanText(k+": "+v))
		}
	}
	return out
}

func cookieLines(h http.Header) []string {
	var out []string
	for _, c := range (&http.Response{Header: h}).Cookies() {
		out = append(out, cleanText(c.String()))
	}
	return out
}

func (m Model) respRows() int {
	g := computeGeometry(m.width, m.height, len(m.ws().Envs))
	return max(1, g.respH-borderLines-respTabLines)
}

func (m Model) responseBody(d respData, w, h int) []string {
	if d.title == "" {
		return nil
	}
	v := m.respViewFor(d)
	lines := m.respLines(d, v.tab)
	rows := max(0, h-respTabLines)
	start := max(0, min(v.scroll, len(lines)-rows))
	out := []string{tabBar(respTabs, v.tab, w)}
	for _, l := range lines[start:min(len(lines), start+rows)] {
		if displayCells(l) > w {
			l = lipgloss.NewStyle().MaxWidth(w).Render(l)
		}
		out = append(out, l)
	}
	return out
}

func responseTitleBar(d respData) string {
	const base = "[5] Response"
	if d.title == "" {
		return base
	}
	return base + " ── " + d.title + " "
}

func (Model) responseBindings() []binding {
	return []binding{
		{keys: keys(&km.Search), run: scrollResp(func(m Model, v responsePanel, _ int) (Model, responsePanel) {
			in := newBottomInput(searchPrompt, "", submitSearch)
			m.overlay = &in
			return m, v
		}), help: "search", hint: "search"},
		{keys: keys(&km.NextMatch, &km.PrevMatch), run: scrollResp(func(m Model, v responsePanel, rows int) (Model, responsePanel) {
			return m, v.step(is(m.lastKey, km.NextMatch), rows)
		}), help: "next / previous match", hint: "next/prev"},
		{keys: keys(&km.CopyBody), run: func(m Model) (Model, tea.Cmd) {
			return m.copyBody(m.currentResponse())
		}, help: "copy body", hint: "copy body"},
		{keys: keys(&km.Send), run: Model.send, help: "send the selected request", hint: "send"},
		{keys: keys(esc), run: noCmd(Model.handleEsc), help: "back to the side panel"},
		{keys: keys(&km.PrevTab, &km.NextTab), run: scrollResp(func(m Model, v responsePanel, _ int) (Model, responsePanel) {
			return m, responsePanel{key: v.key, tab: cycleTab(v.tab, direction(m.lastKey, km.NextTab), len(respTabs))}
		}), help: "previous / next tab"},
		{keys: listMoves, run: scrollResp(func(m Model, v responsePanel, _ int) (Model, responsePanel) {
			v.scroll += direction(m.lastKey, km.NextItem)
			return m, v
		}), help: "scroll down / up"},
		{keys: keys(&km.HalfPageDown, &km.HalfPageUp), run: scrollResp(func(m Model, v responsePanel, rows int) (Model, responsePanel) {
			v.scroll += direction(m.lastKey, km.HalfPageDown) * (rows / halfPageRatio)
			return m, v
		}), help: "half page down / up"},
		{keys: keys(&km.GotoTop, &km.GotoBottom), run: scrollResp(func(m Model, v responsePanel, _ int) (Model, responsePanel) {
			v.scroll = 0
			if is(m.lastKey, km.GotoBottom) {
				v.scroll = len(m.respLines(m.currentResponse(), v.tab))
			}
			return m, v
		})},
	}
}

func scrollResp(f func(m Model, v responsePanel, rows int) (Model, responsePanel)) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) {
		d := m.currentResponse()
		rows := m.respRows()
		m, v := f(m, m.respViewFor(d), rows)
		v.scroll = max(0, min(v.scroll, max(0, len(m.respLines(d, v.tab))-rows)))
		m.response = v
		return m, nil
	}
}

func (v responsePanel) step(forward bool, rows int) responsePanel {
	n := len(v.matches)
	if n == 0 {
		return v
	}
	delta := -1
	if forward {
		delta = 1
	}
	v.match = cycleTab(v.match, delta, n)
	v.scroll = reveal(v.matches[v.match], v.scroll, rows)
	return v
}

func reveal(line, scroll, rows int) int {
	if line >= scroll && line < scroll+rows {
		return scroll
	}
	return line
}

func submitSearch(m Model, query string) (Model, tea.Cmd) {
	d := m.currentResponse()
	v := m.respViewFor(d)
	lines := m.respLines(d, v.tab)
	rows := m.respRows()
	v.matches = bodyview.Search(lines, query)
	if len(v.matches) == 0 {
		m.response = v
		m.status = "no match: " + query
		return m, nil
	}
	v.match = 0
	for i, l := range v.matches {
		if l >= v.scroll {
			v.match = i
			break
		}
	}
	v.scroll = max(0, min(reveal(v.matches[v.match], v.scroll, rows), len(lines)-rows))
	m.response = v
	return m, nil
}

func (m Model) copyBody(d respData) (Model, tea.Cmd) {
	if len(d.body) == 0 {
		return m, nil
	}
	text := string(d.body)
	m.status = statusCopied
	if err := m.deps.Clip.Write(text); err != nil {
		m.status = statusCopied + " (OSC 52 only): " + err.Error()
	}
	return m, tea.SetClipboard(text)
}
