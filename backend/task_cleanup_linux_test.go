//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupTraverseOnlyAncestor(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires a non-root user to exercise directory access checks")
	}
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ancestor := filepath.Join(t.TempDir(), "traverse-only")
	root := filepath.Join(ancestor, "downloads")
	if err := os.MkdirAll(filepath.Join(root, ".quarkdrive-part"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.bin"), []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".quarkdrive-part", "done.part"), []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ancestor, 0111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ancestor, 0700) })
	if f, err := os.Open(ancestor); err == nil {
		f.Close()
		t.Fatal("fixture must deny directory listing")
	}
	store.state.Tasks = []*Task{{ID: "done", Status: "COMPLETED", Destination: root, Relative: "file.bin", Size: 4}}
	result, err := newEngine(store, nil).cleanupTasks([]string{"done"}, true, []string{root})
	if err != nil || result.Removed != 1 || len(result.Failures) != 0 {
		t.Fatalf("cannot clean authorized download below traverse-only ancestor: %+v %v", result, err)
	}
	for _, relative := range []string{"file.bin", ".quarkdrive-part/done.part"} {
		if _, err := os.Stat(filepath.Join(root, relative)); !os.IsNotExist(err) {
			t.Fatalf("file retained %s: %v", relative, err)
		}
	}
}

func TestCleanupCannotDeleteWithoutParentWritePermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires a non-root user")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.bin"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0700) })
	size := int64(4)
	if err := taskFile(root, "file.bin", &size, true); err == nil {
		t.Fatal("must honor missing delete permission")
	}
	if b, err := os.ReadFile(filepath.Join(root, "file.bin")); err != nil || string(b) != "keep" {
		t.Fatal("file was removed", err)
	}
}
