package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRootPrecedence(t *testing.T) {
	t.Setenv("LAZYPOST_HOME", "/env/ws")
	if got, _ := ResolveRoot("/flag/ws", "/config/ws"); got != "/flag/ws" {
		t.Fatalf("flag: %q", got)
	}
	if got, _ := ResolveRoot("", "/config/ws"); got != "/env/ws" {
		t.Fatalf("env: %q", got)
	}
	t.Setenv("LAZYPOST_HOME", "")
	if got, _ := ResolveRoot("", "/config/ws"); got != "/config/ws" {
		t.Fatalf("config: %q", got)
	}
	t.Setenv("HOME", "/home/u")
	if got, _ := ResolveRoot("", ""); got != "/home/u/lazypost" {
		t.Fatalf("home: %q", got)
	}
}

func gitignoreLines(t *testing.T, root string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func count(lines []string, want string) int {
	n := 0
	for _, l := range lines {
		if l == want {
			n++
		}
	}
	return n
}

func TestInitCreatesSkeleton(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	created, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	for _, d := range []string{"envs", "collections"} {
		fi, err := os.Stat(filepath.Join(root, d))
		if err != nil || !fi.IsDir() {
			t.Fatalf("dir %s missing: %v", d, err)
		}
	}
	lines := gitignoreLines(t, root)
	for _, p := range []string{"*.secret.yaml", "/secret.yaml"} {
		if count(lines, p) != 1 {
			t.Fatalf("%q not exactly once in %v", p, lines)
		}
	}
}

func TestInitIsIdempotentAndKeepsUserGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("created = true, want false (dir existed)")
	}
	lines := gitignoreLines(t, root)
	for _, p := range []string{"node_modules", "*.secret.yaml", "/secret.yaml"} {
		if count(lines, p) != 1 {
			t.Fatalf("%q not exactly once in %v", p, lines)
		}
	}
}

func TestInitGitignoreWithoutTrailingNewline(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("a"), 0o644)
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	lines := gitignoreLines(t, root)
	if count(lines, "a") != 1 || count(lines, "/secret.yaml") != 1 {
		t.Fatalf("bad lines: %v", lines)
	}
}

func TestInitAddsRootSecretPatternToOldGitignore(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.secret.yaml\nsecret.yaml\n"), 0o644)
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	lines := gitignoreLines(t, root)
	if count(lines, "/secret.yaml") != 1 || count(lines, "*.secret.yaml") != 1 {
		t.Fatalf("bad lines: %v", lines)
	}
}
