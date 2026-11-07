package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRequestModTimeTracksLoadAndSave(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ws.CreateRequest(ws.Collections, "get user")
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(r.Path); !r.ModTime.Equal(fi.ModTime()) {
		t.Fatalf("after create ModTime %v, file %v", r.ModTime, fi.ModTime())
	}

	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(r.Path, old, old); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.FindRequest(r.Path)
	if got == nil || !got.ModTime.Equal(old) {
		t.Fatalf("loaded ModTime = %v, want %v", got, old)
	}

	if err := loaded.SaveRequest(got); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(got.Path); got.ModTime.Equal(old) || !got.ModTime.Equal(fi.ModTime()) {
		t.Fatalf("after save ModTime %v, file %v", got.ModTime, fi.ModTime())
	}
}
