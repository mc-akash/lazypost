package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mc-akash/lazypost/internal/paths"
)

const defaultTimeout = 30 * time.Second

type Config struct {
	Workspace string        `yaml:"workspace"`
	Timeout   time.Duration `yaml:"timeout"`
	Insecure  bool          `yaml:"insecure"`
	Keybinds  Keymap        `yaml:"keybinds"`
}

func Load(path string) (Config, error) {
	c := Config{Timeout: defaultTimeout, Keybinds: DefaultKeymap()}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.Timeout <= 0 {
		return Config{}, fmt.Errorf("%s: timeout must be > 0, got %s", path, c.Timeout)
	}
	if c.Workspace, err = paths.ExpandHome(c.Workspace); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}
