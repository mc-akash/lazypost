package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestKeymapActionsComplete(t *testing.T) {
	scopes := []string{"all", "envs", "collections", "history", "request", "response", "vars"}
	typ := reflect.TypeFor[Keymap]()
	var names []string
	for i := range typ.NumField() {
		tag := typ.Field(i).Tag
		name := tag.Get("yaml")
		if name == "" || tag.Get("default") == "" || tag.Get("scope") == "" || slices.Contains(names, name) {
			t.Errorf("%s: needs a unique yaml name, a default and a scope", typ.Field(i).Name)
		}
		names = append(names, name)
		for _, s := range strings.Split(tag.Get("scope"), ",") {
			if !slices.Contains(scopes, s) {
				t.Errorf("%s: unknown scope %q", name, s)
			}
		}
	}
}

func TestKeymapValidate(t *testing.T) {
	if err := DefaultKeymap().Validate(); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	for _, tc := range []struct {
		set  func(*Keymap)
		want string
	}{
		{func(k *Keymap) { k.Send = Keys{} }, "send: empty key list"},
		{func(k *Keymap) { k.Send = Keys{""} }, "send: empty key"},
		{func(k *Keymap) { k.Send = Keys{"s", "s"} }, `send: duplicate key "s"`},
		{func(k *Keymap) { k.GotoTop = Keys{"gg"} }, `goto_top: unknown key "gg"`},
		{func(k *Keymap) { k.Send = Keys{"ctrl+R"} }, `send: unknown key "ctrl+R"`},
		{func(k *Keymap) { k.NextItem = Keys{"j down"} }, `next_item: unknown key "j down"`},
		{func(k *Keymap) { k.Select = Keys{" "} }, `select: unknown key " "`},
		{func(k *Keymap) { k.Confirm = Keys{"x"} }, `confirm: "x" is printable`},
		{func(k *Keymap) { k.JumpToPanel = Keys{"1", "2", "3", "4"} }, "jump_to_panel: needs 5 keys, got 4"},
		{func(k *Keymap) { k.Send = Keys{"j"} }, `next_item and send both use "j"`},
		{func(k *Keymap) { k.Send = Keys{"g"} }, `goto_top and send both use "g"`},
		{func(k *Keymap) { k.Reload = Keys{"3"} }, `jump_to_panel and reload both use "3"`},
		{func(k *Keymap) { k.Save = Keys{"ctrl+s"} }, `body_save and save both use "ctrl+s"`},
		{func(k *Keymap) { k.Send = Keys{"ctrl+r", "R"} }, `send and reload both use "R"`},
		{func(k *Keymap) { k.CopyBody = Keys{"/"} }, `search and copy_body both use "/"`},
		{func(k *Keymap) { k.Paste = Keys{"s"} }, `send and paste both use "s"`},
		{func(k *Keymap) { k.HalfPageDown = Keys{"y"} }, `half_page_down and copy_body both use "y"`},
	} {
		k := DefaultKeymap()
		tc.set(&k)
		if err := k.Validate(); err == nil || !strings.Contains(err.Error(), "keybinds: "+tc.want) {
			t.Errorf("err = %v, want %q", err, tc.want)
		}
	}
	k := DefaultKeymap()
	k.New, k.NextMatch, k.Send = Keys{"x"}, Keys{"x"}, Keys{"shift+tab", "f5", "alt+enter", "é"}
	k.NextPanel, k.PrevPanel = Keys{"tab"}, Keys{"ctrl+alt+p"}
	if err := k.Validate(); err != nil {
		t.Fatalf("valid keys rejected: %v", err)
	}
}
