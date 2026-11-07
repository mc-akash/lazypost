package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/config"
	"github.com/mc-akash/lazypost/internal/paths"
	"github.com/mc-akash/lazypost/internal/ui"
)

func newBuilder(o options) (*appBuilder, error) {
	b := newAppBuilder(o)
	return b, b.Init(initFromConfigFile)
}

func build(o options) (*appRunner, error) {
	b, err := newBuilder(o)
	if err != nil {
		return nil, err
	}
	return b.Build()
}

func TestBuildReportsUnreadableState(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(stateHome, "lazypost"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateHome, "lazypost", "state.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := build(options{workspace: filepath.Join(t.TempDir(), "ws")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.model.Status(), "session state unreadable: ") || !strings.Contains(r.model.Status(), " (starting fresh)") {
		t.Fatalf("status = %q", r.model.Status())
	}
}

func TestBuildNoStartupStatusWhenStateFine(t *testing.T) {
	isolate(t)
	r, err := build(options{workspace: filepath.Join(t.TempDir(), "ws")})
	if err != nil || strings.Contains(r.model.Status(), "session state") {
		t.Fatalf("err = %v", err)
	}
}

type otherModel struct{}

func (otherModel) Init() tea.Cmd                       { return nil }
func (otherModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return otherModel{}, nil }
func (otherModel) View() tea.View                      { return tea.NewView("") }

func TestSaveFinalSession(t *testing.T) {
	isolate(t)
	r, err := build(options{workspace: filepath.Join(t.TempDir(), "ws")})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveFinalSession(r.model); err != nil {
		t.Fatal(err)
	}
	dir, err := paths.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("session not saved: %v", err)
	}
	if err := saveFinalSession(otherModel{}); err != nil {
		t.Fatal(err)
	}
	if err := saveFinalSession(nil); err != nil {
		t.Fatal(err)
	}
}

func isolate(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	return filepath.Join(configHome, "lazypost", "config.yaml")
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func isInsecure(c *http.Client) bool {
	tr, ok := c.Transport.(*http.Transport)
	return ok && tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify
}

func TestBuildClientDefaultsWithoutConfig(t *testing.T) {
	isolate(t)
	b, err := newBuilder(options{})
	if err != nil {
		t.Fatal(err)
	}
	c := b.buildClient()
	if c.Timeout != 30*time.Second || isInsecure(c) {
		t.Fatalf("timeout = %v, insecure = %v", c.Timeout, isInsecure(c))
	}
}

func TestBuildUsesConfig(t *testing.T) {
	cfg := isolate(t)
	t.Setenv("LAZYPOST_HOME", "")
	ws := filepath.Join(t.TempDir(), "from-config")
	writeConfig(t, cfg, "workspace: "+ws+"\ntimeout: 7s\ninsecure: true\n")
	b, err := newBuilder(options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Build(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws, "collections")); err != nil {
		t.Fatalf("workspace not created from config: %v", err)
	}
	c := b.buildClient()
	if c.Timeout != 7*time.Second || !isInsecure(c) {
		t.Fatalf("timeout = %v, insecure = %v", c.Timeout, isInsecure(c))
	}
}

func TestBuildClientInsecureIsFlagOrConfig(t *testing.T) {
	for _, tc := range []struct{ config, flag, want bool }{
		{false, false, false},
		{true, false, true},
		{false, true, true},
		{true, true, true},
	} {
		cfg := isolate(t)
		if tc.config {
			writeConfig(t, cfg, "insecure: true\n")
		}
		b, err := newBuilder(options{insecure: tc.flag})
		if err != nil || isInsecure(b.buildClient()) != tc.want {
			t.Errorf("config=%v flag=%v: err = %v", tc.config, tc.flag, err)
		}
	}
}

func TestBuildInvalidConfigFailsFast(t *testing.T) {
	cfg := isolate(t)
	writeConfig(t, cfg, "timout: 5s\n")
	ws := filepath.Join(t.TempDir(), "ws")
	_, err := build(options{workspace: ws})
	if err == nil || !strings.Contains(err.Error(), cfg) || !strings.Contains(err.Error(), "timout") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(ws); !os.IsNotExist(statErr) {
		t.Fatal("workspace created despite an invalid config")
	}
}

func TestBuildAppliesKeybinds(t *testing.T) {
	cfg := isolate(t)
	t.Cleanup(func() {
		if err := ui.ApplyKeybinds(config.DefaultKeymap()); err != nil {
			t.Fatal(err)
		}
	})
	writeConfig(t, cfg, "keybinds:\n  help: x\n")
	r, err := build(options{workspace: filepath.Join(t.TempDir(), "ws")})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := r.model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !strings.Contains(m.(ui.Model).Render(), "Keybindings") {
		t.Fatal("rebound help key does not open help")
	}
}

func TestBuildInvalidKeybindFailsFast(t *testing.T) {
	cfg := isolate(t)
	writeConfig(t, cfg, "keybinds:\n  sned: x\n")
	_, err := build(options{workspace: filepath.Join(t.TempDir(), "ws")})
	if err == nil || !strings.Contains(err.Error(), cfg) || !strings.Contains(err.Error(), "field sned not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildEmptyKeyListFailsFast(t *testing.T) {
	cfg := isolate(t)
	writeConfig(t, cfg, "keybinds:\n  send: []\n")
	_, err := build(options{workspace: filepath.Join(t.TempDir(), "ws")})
	if err == nil || !strings.Contains(err.Error(), cfg) || !strings.Contains(err.Error(), "send: empty key list") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckConfig(t *testing.T) {
	cfg := isolate(t)
	var out strings.Builder
	if err := run([]string{"-check-config"}, &out); err != nil || out.String() != cfg+": ok\n" {
		t.Fatalf("missing config: err = %v, out = %q", err, out.String())
	}
	writeConfig(t, cfg, "keybinds:\n  paste: s\n")
	err := run([]string{"-check-config"}, &out)
	if err == nil || !strings.Contains(err.Error(), cfg) || !strings.Contains(err.Error(), `send and paste both use "s"`) {
		t.Fatalf("err = %v", err)
	}
}
