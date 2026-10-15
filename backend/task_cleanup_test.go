package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearTaskRecordsPreservesFilesAndUnfinishedTasks(t *testing.T) {
	dir := t.TempDir()
	store, err := newStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	statuses := []string{"COMPLETED", "FAILED", "PAUSED", "DOWNLOADING", "QUEUED", "VERIFYING", "COMPLETED"}
	ids := []string{}
	for i, status := range statuses {
		id := string(rune('a' + i))
		ids = append(ids, id)
		file := filepath.Join(dir, id+".bin")
		if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		store.state.Tasks = append(store.state.Tasks, &Task{ID: id, Status: status, Destination: dir, Relative: id + ".bin"})
	}
	if err := store.save(); err != nil {
		t.Fatal(err)
	}
	engine := newEngine(store, nil)
	// A completed status can still be cleaning up a cloud transfer.
	engine.running["g"] = &runningTask{}
	n, err := engine.clearRecords(append(ids, "missing", "a"))
	if err != nil || n != 2 {
		t.Fatalf("removed=%d err=%v", n, err)
	}
	if len(store.snapshot().Tasks) != 5 {
		t.Fatal("removed unfinished/busy tasks")
	}
	for _, id := range ids {
		b, err := os.ReadFile(filepath.Join(dir, id+".bin"))
		if err != nil || string(b) != "keep" {
			t.Fatalf("file changed: %s %v", id, err)
		}
	}
	again, err := newStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.snapshot().Tasks) != 5 {
		t.Fatal("cleanup not durable")
	}
	if n, err = engine.clearRecords([]string{"a", "b"}); err != nil || n != 0 {
		t.Fatalf("repeat=%d %v", n, err)
	}
	// A failed task resumed after the dialog opened must survive.
	if err := store.setTask("c", func(t *Task) { t.Status = "QUEUED" }); err != nil {
		t.Fatal(err)
	}
	if n, err = engine.clearRecords([]string{"c"}); err != nil || n != 0 {
		t.Fatal("removed resumed task")
	}
}

func TestClearTaskRecordsRollsBackOnSaveFailure(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.state.Tasks = []*Task{{ID: "done", Status: "COMPLETED"}}
	store.path = filepath.Join(t.TempDir(), "missing", "state.json")
	engine := newEngine(store, nil)
	if n, err := engine.clearRecords([]string{"done"}); err == nil || n != 0 {
		t.Fatalf("expected save failure: %d %v", n, err)
	}
	if len(store.snapshot().Tasks) != 1 {
		t.Fatal("failed persistence removed in-memory records")
	}
}

func TestTaskCleanupEndpoint(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.state.Tasks = []*Task{{ID: "done", Status: "COMPLETED"}, {ID: "paused", Status: "PAUSED"}}
	server := &Server{store: store, engine: newEngine(store, nil), dev: true}
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/tasks/cleanup", strings.NewReader(body))
		r.Header.Set("X-QuarkDrive-Request", "1")
		w := httptest.NewRecorder()
		server.serve(w, r)
		return w
	}
	if w := request(`{"ids":[]}`); w.Code != 400 {
		t.Fatalf("empty request=%d %s", w.Code, w.Body.String())
	}
	if w := request(`{"ids":["done","paused"]}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"removed":1`) {
		t.Fatalf("cleanup=%d %s", w.Code, w.Body.String())
	}
	if len(store.snapshot().Tasks) != 1 || store.task("paused") == nil {
		t.Fatal("endpoint removed paused task")
	}
}

func cleanupTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func TestCleanupFilesOnlyExplicitlySelectedAndAuthorized(t *testing.T) {
	for _, deleteFiles := range []bool{false, true} {
		t.Run(fmt.Sprint(deleteFiles), func(t *testing.T) {
			root := cleanupTestRoot(t)
			dir := t.TempDir()
			store, err := newStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, ".quarkdrive-part"), 0700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"done.bin", "paused.bin", "other.bin", ".quarkdrive-part/fail.part"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte("data"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			store.state.Tasks = []*Task{{ID: "done", Status: "COMPLETED", Destination: root, Relative: "done.bin", Size: 4}, {ID: "fail", Status: "FAILED", Destination: root, Relative: "other.bin", Size: 4}, {ID: "pause", Status: "PAUSED", Destination: root, Relative: "paused.bin", Size: 4}}
			engine := newEngine(store, nil)
			result, err := engine.cleanupTasks([]string{"done", "fail", "pause"}, deleteFiles, []string{root})
			if err != nil || result.Removed != 2 || result.Skipped != 1 || len(result.Failures) != 0 {
				t.Fatalf("%+v %v", result, err)
			}
			for _, name := range []string{"done.bin", ".quarkdrive-part/fail.part"} {
				_, err := os.Stat(filepath.Join(root, name))
				if deleteFiles && !os.IsNotExist(err) || !deleteFiles && err != nil {
					t.Fatalf("%s deletion=%v err=%v", name, deleteFiles, err)
				}
			}
			for _, name := range []string{"paused.bin", "other.bin"} {
				if _, err := os.Stat(filepath.Join(root, name)); err != nil {
					t.Fatalf("unrelated file removed: %s", name)
				}
			}
			reloaded, err := newStore(dir)
			if err != nil || len(reloaded.snapshot().Tasks) != 1 {
				t.Fatal("cleanup not persisted", err)
			}
		})
	}
}
func TestCleanupFileGuardsRetainRecords(t *testing.T) {
	for _, scenario := range []string{"unauthorized", "symlink-parent", "symlink-root", "symlink-file", "directory", "size-changed", "shared-file", "traversal", "bad-id", "busy", "save-failure"} {
		t.Run(scenario, func(t *testing.T) {
			root := cleanupTestRoot(t)
			store, err := newStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			task := &Task{ID: "done", Status: "COMPLETED", Destination: root, Relative: "done.bin", Size: 4}
			if err := os.WriteFile(filepath.Join(root, "done.bin"), []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			allowed := []string{root}
			outside := cleanupTestRoot(t)
			if err := os.WriteFile(filepath.Join(outside, "external.bin"), []byte("safe"), 0600); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "unauthorized":
				allowed = []string{outside}
			case "symlink-parent":
				os.Symlink(outside, filepath.Join(root, "link"))
				task.Relative = "link/external.bin"
			case "symlink-root":
				link := filepath.Join(outside, "root-link")
				os.Symlink(root, link)
				task.Destination = link
				allowed = []string{link}
			case "symlink-file":
				os.Symlink(filepath.Join(outside, "external.bin"), filepath.Join(root, "link.bin"))
				task.Relative = "link.bin"
			case "directory":
				os.Mkdir(filepath.Join(root, "dir"), 0700)
				task.Relative = "dir"
			case "size-changed":
				task.Size = 9
			case "shared-file":
				store.state.Tasks = append(store.state.Tasks, &Task{ID: "other", Status: "PAUSED", Destination: root, Relative: task.Relative})
			case "traversal":
				task.Relative = "../external.bin"
			case "bad-id":
				task.ID = "../bad"
			case "save-failure":
				store.path = filepath.Join(root, "missing", "state.json")
			}
			store.state.Tasks = append(store.state.Tasks, task)
			engine := newEngine(store, nil)
			if scenario == "busy" {
				engine.running[task.ID] = &runningTask{}
			}
			result, err := engine.cleanupTasks([]string{task.ID}, true, allowed)
			if result.Removed != 0 || store.task(task.ID) == nil {
				t.Fatalf("record removed: %+v %v", result, err)
			}
			if scenario != "save-failure" && scenario != "busy" && len(result.Failures) != 1 {
				t.Fatalf("missing failure: %+v %v", result, err)
			}
			for _, file := range []string{filepath.Join(root, "done.bin"), filepath.Join(outside, "external.bin")} {
				if _, err := os.Stat(file); err != nil {
					t.Fatal("protected file removed", err)
				}
			}
		})
	}
}
func TestCleanupMissingFilesAndLegacyDestination(t *testing.T) {
	root := cleanupTestRoot(t)
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.state.Settings.DownloadDir = root
	store.state.Tasks = []*Task{{ID: "missing", Status: "COMPLETED", Relative: "absent.bin", Size: 4}}
	result, err := newEngine(store, nil).cleanupTasks([]string{"missing"}, true, []string{root})
	if err != nil || result.Removed != 1 || len(result.Failures) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestTaskCleanupEndpointDeletesFilesOnlyWhenRequested(t *testing.T) {
	root := cleanupTestRoot(t)
	t.Setenv("QUARKDRIVE_DOWNLOAD_DIR", root)
	t.Setenv("TRIM_DATA_SHARE_PATHS", "")
	t.Setenv("TRIM_DATA_ACCESSIBLE_PATHS", "")
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"kept", "removed"} {
		if err := os.WriteFile(filepath.Join(root, id), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		store.state.Tasks = append(store.state.Tasks, &Task{ID: id, Status: "COMPLETED", Relative: id, Destination: root, Size: 4})
	}
	server := &Server{store: store, engine: newEngine(store, nil), dev: true}
	for _, body := range []string{`{"ids":["kept"]}`, `{"ids":["removed"],"delete_files":true}`} {
		r := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/tasks/cleanup", strings.NewReader(body))
		r.Header.Set("X-QuarkDrive-Request", "1")
		w := httptest.NewRecorder()
		server.serve(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"removed":1`) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "kept")); err != nil {
		t.Fatal("default deleted a file")
	}
	if _, err := os.Stat(filepath.Join(root, "removed")); !os.IsNotExist(err) {
		t.Fatal("explicit delete did not delete file")
	}
}
