package main

import (
	"fmt"
	"net/http"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/mc-akash/lazypost/internal/config"
	"github.com/mc-akash/lazypost/internal/history"
	"github.com/mc-akash/lazypost/internal/httpx"
	"github.com/mc-akash/lazypost/internal/paths"
	"github.com/mc-akash/lazypost/internal/state"
	"github.com/mc-akash/lazypost/internal/ui"
	"github.com/mc-akash/lazypost/internal/workspace"
)

type appBuilder struct {
	opts       options
	configPath string
	cfg        config.Config
}

func newAppBuilder(o options) *appBuilder {
	return &appBuilder{opts: o}
}

func (b *appBuilder) Init(f func(*appBuilder) error) error {
	return f(b)
}

func initFromConfigFile(b *appBuilder) error {
	dir, err := paths.ConfigDir()
	if err != nil {
		return err
	}
	b.configPath = filepath.Join(dir, "config.yaml")
	cfg, err := config.Load(b.configPath)
	if err != nil {
		return err
	}
	b.cfg = cfg
	return nil
}

func (b *appBuilder) Build() (*appRunner, error) {
	if err := ui.ApplyKeybinds(b.cfg.Keybinds); err != nil {
		return nil, fmt.Errorf("%s: %w", b.configPath, err)
	}
	ws, err := b.buildWorkspace()
	if err != nil {
		return nil, err
	}
	dir, err := paths.StateDir()
	if err != nil {
		return nil, err
	}
	store := state.Open(filepath.Join(dir, "state.json"))
	sess, err := store.Load(ws.Root)
	var startup string
	if err != nil {
		startup = stateErrStatus(err)
	}
	m, err := ui.NewModelBuilder().
		WithWorkspace(ws).
		WithState(store, sess).
		WithHistory(history.Open(filepath.Join(dir, "history.jsonl")).WithMax(b.cfg.History)).
		WithClipboard(systemClipboard{}).
		WithClient(b.buildClient()).
		WithStartupStatus(startup).
		Build()
	if err != nil {
		return nil, err
	}
	return &appRunner{model: m}, nil
}

func (b *appBuilder) buildWorkspace() (*workspace.Workspace, error) {
	root, err := workspace.ResolveRoot(b.opts.workspace, b.cfg.Workspace)
	if err != nil {
		return nil, err
	}
	if root, err = filepath.Abs(root); err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	ws, err := workspace.Load(root)
	if err != nil {
		return nil, fmt.Errorf("load workspace %s: %w", root, err)
	}
	return ws, nil
}

func (b *appBuilder) buildClient() *http.Client {
	return httpx.NewClient(httpx.Options{Timeout: b.cfg.Timeout, Insecure: b.cfg.Insecure || b.opts.insecure})
}

type appRunner struct {
	model ui.Model
}

func (r *appRunner) Run() error {
	final, err := tea.NewProgram(r.model).Run()
	saveErr := saveFinalSession(final)
	if err != nil {
		return fmt.Errorf("run ui: %w", err)
	}
	return saveErr
}

func saveFinalSession(final tea.Model) error {
	m, ok := final.(ui.Model)
	if !ok {
		return nil
	}
	if err := m.SaveSession(); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func stateErrStatus(err error) string {
	return fmt.Sprintf("session state unreadable: %v (starting fresh)", err)
}

type systemClipboard struct{}

func (systemClipboard) Read() (string, error) { return clipboard.ReadAll() }

func (systemClipboard) Write(s string) error { return clipboard.WriteAll(s) }
