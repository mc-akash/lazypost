package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/config"
)

var km config.Keymap

func init() {
	if err := ApplyKeybinds(config.DefaultKeymap()); err != nil {
		panic(err)
	}
}

func ApplyKeybinds(k config.Keymap) error {
	if err := k.Validate(); err != nil {
		return err
	}
	km = k
	return nil
}

func is(key string, k config.Keys) bool { return slices.Contains(k, key) }

type binding struct {
	keys []*config.Keys
	run  func(Model) (Model, tea.Cmd)
	help string
	hint string
}

func dispatch(m Model, key string, bs []binding) (Model, tea.Cmd, bool) {
	m.lastKey = key
	for _, b := range bs {
		if b.run != nil && slices.ContainsFunc(b.keys, func(k *config.Keys) bool { return k == &fixed.Any || is(key, *k) }) {
			next, cmd := b.run(m)
			return next, cmd, true
		}
	}
	return m, nil, false
}

func noCmd(f func(Model) Model) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) { return f(m), nil }
}

func direction(key string, forward config.Keys) int {
	if is(key, forward) {
		return 1
	}
	return -1
}

func hinted(bs []binding) []binding {
	return slices.DeleteFunc(slices.Clone(bs), func(b binding) bool { return b.hint == "" })
}

func keys(ks ...*config.Keys) []*config.Keys { return ks }

func (b binding) keyText() string {
	if len(b.keys) == 1 {
		return strings.Join(*b.keys[0], "/")
	}
	first := make([]string, len(b.keys))
	for i, k := range b.keys {
		first[i] = (*k)[0]
	}
	return strings.Join(first, "/")
}

var (
	fixed = struct{ Yes, No, Type, Any, Backspace, NextLine, PrevLine config.Keys }{
		Yes:       config.Keys{"y"},
		No:        config.Keys{"n"},
		Type:      config.Keys{"type"},
		Any:       config.Keys{"any key"},
		Backspace: config.Keys{"backspace"},
		NextLine:  config.Keys{"ctrl+n"},
		PrevLine:  config.Keys{"ctrl+p"},
	}

	enter, esc, space = &km.Confirm, &km.Return, &km.Select

	listMoves = keys(&km.NextItem, &km.PrevItem)
)

func (m Model) hints() []binding {
	if m.overlay != nil {
		return m.overlay.hints(m)
	}
	return m.focusedHints()
}

func (m Model) focusedHints() []binding {
	return slices.Concat(hinted(m.panelBindings()), hinted(m.globalBindings()))
}

func hintText(b binding) string { return b.hint + ": " + b.keyText() }

func hintLine(bs []binding) string {
	parts := make([]string, len(bs))
	for i, b := range bs {
		parts[i] = hintText(b)
	}
	return strings.Join(parts, "  ")
}
