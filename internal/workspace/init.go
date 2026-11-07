package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var secretPatterns = []string{"*.secret.yaml", "/secret.yaml"}

func ResolveRoot(flag, configured string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := os.Getenv("LAZYPOST_HOME"); env != "" {
		return env, nil
	}
	if configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	return filepath.Join(home, "lazypost"), nil
}

func Init(root string) (created bool, err error) {
	if _, statErr := os.Stat(root); errors.Is(statErr, os.ErrNotExist) {
		created = true
	} else if statErr != nil {
		return false, statErr
	}
	for _, d := range []string{"envs", "collections"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return created, err
		}
	}
	return created, ensureGitignore(filepath.Join(root, ".gitignore"))
}

func ensureGitignore(path string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(existing), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var add strings.Builder
	for _, p := range secretPatterns {
		if !have[p] {
			add.WriteString(p + "\n")
		}
	}
	if add.Len() == 0 {
		return nil
	}
	out := string(existing)
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(path, []byte(out+add.String()), 0o644)
}
