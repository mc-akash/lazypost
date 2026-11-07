package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDirXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	got, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, "lazypost"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v, want dir 0700", info.Mode())
	}

	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)
	got, err = StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "state", "lazypost"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatal(err)
	}
}

func TestConfigDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, "lazypost"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("ConfigDir must not create the dir: %v", err)
	}

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	got, err = ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "lazypost"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	for in, want := range map[string]string{
		"~":        home,
		"~/x/y":    filepath.Join(home, "x/y"),
		"/abs/~/p": "/abs/~/p",
		"~other/p": "~other/p",
	} {
		if got, err := ExpandHome(in); err != nil || got != want {
			t.Errorf("ExpandHome(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
}

func TestWriteAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	for _, data := range []string{"one", "two"} {
		if err := WriteAtomic(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != data {
			t.Fatalf("content = %q, %v", got, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, %v", info.Mode().Perm(), err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}
