package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const appName = "lazypost"

func StateDir() (string, error) {
	dir, err := xdgDir("XDG_STATE_HOME", ".local", "state")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir: %w", err)
	}
	return dir, nil
}

func ConfigDir() (string, error) {
	return xdgDir("XDG_CONFIG_HOME", ".config")
}

func xdgDir(env string, fallback ...string) (string, error) {
	base := os.Getenv(env)
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(append([]string{home}, fallback...)...)
	}
	return filepath.Join(base, appName), nil
}

func ExpandHome(p string) (string, error) {
	rest, ok := strings.CutPrefix(p, "~")
	if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand ~: %w", err)
	}
	return filepath.Join(home, rest), nil
}

func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	return os.Rename(name, path)
}
