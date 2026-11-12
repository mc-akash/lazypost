package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	want := Config{Timeout: 30 * time.Second, History: 10, Keybinds: DefaultKeymap()}
	for name, path := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "config.yaml"),
		"empty":   writeConfig(t, ""),
	} {
		got, err := Load(path)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, %v; want %+v", name, got, err, want)
		}
	}
}

func TestLoadFullFile(t *testing.T) {
	got, err := Load(writeConfig(t, "workspace: /srv/ws\ntimeout: 5s\ninsecure: true\nhistory: 50\nkeybinds:\n  send: ctrl+r\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Workspace: "/srv/ws", Timeout: 5 * time.Second, Insecure: true, History: 50, Keybinds: DefaultKeymap()}
	want.Keybinds.Send = Keys{"ctrl+r"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadKeybindsScalarOrList(t *testing.T) {
	got, err := Load(writeConfig(t, "keybinds:\n  send: [s, ctrl+r]\n  quit: x\n  jump_to_panel: [1, 2, 3, 4, 5]\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultKeymap()
	want.Send, want.Quit, want.JumpToPanel = Keys{"s", "ctrl+r"}, Keys{"x"}, Keys{"1", "2", "3", "4", "5"}
	if !reflect.DeepEqual(got.Keybinds, want) {
		t.Fatalf("got %v, want %v", got.Keybinds, want)
	}
}

func TestLoadExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for in, want := range map[string]string{
		"~/apis/ws": filepath.Join(home, "apis", "ws"),
		`"~"`:       home,
		"~other/ws": "~other/ws",
		"rel/ws":    "rel/ws",
	} {
		got, err := Load(writeConfig(t, "workspace: "+in+"\n"))
		if err != nil || got.Workspace != want {
			t.Errorf("%q: got %q, %v; want %q", in, got.Workspace, err, want)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	for name, tc := range map[string]struct{ content, want string }{
		"unknown key":      {"timout: 5s\n", "timout"},
		"bad timeout":      {"timeout: soon\n", "soon"},
		"zero timeout":     {"timeout: 0s\n", "timeout must be > 0"},
		"negative timeout": {"timeout: -1s\n", "timeout must be > 0"},
		"zero history":     {"history: 0\n", "history must be at least 1"},
		"bad yaml":         {"workspace: [\n", "yaml"},
		"keybind mapping":  {"keybinds:\n  send: {a: b}\n", "cannot unmarshal !!map"},
		"unknown action":   {"keybinds:\n  sned: x\n", "field sned not found"},
	} {
		path := writeConfig(t, tc.content)
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to name %s and %q", name, err, path, tc.want)
		}
	}
}

func TestLoadUnreadable(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), dir) {
		t.Fatalf("err = %v", err)
	}
}
