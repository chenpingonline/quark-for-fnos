package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSpeedWindowTracksRecentTransferAndStall(t *testing.T) {
	start := time.Unix(0, 0)
	var window speedWindow
	window.reset(start, 9000) // Previously downloaded bytes are not counted.
	for second := 1; second <= 4; second++ {
		if rate := window.sample(start.Add(time.Duration(second)*time.Second), 9000+int64(second*1000)); rate != 1000 {
			t.Fatalf("continuous transfer rate: %d", rate)
		}
	}
	for second := 5; second <= 8; second++ {
		want := int64((8 - second) * 250)
		if rate := window.sample(start.Add(time.Duration(second)*time.Second), 13000); rate != want {
			t.Fatalf("stall at %d: got %d want %d", second, rate, want)
		}
	}
	if rate := window.sample(start.Add(9*time.Second), 14000); rate != 250 {
		t.Fatalf("restart rate: %d", rate)
	}
	window.reset(start.Add(10*time.Second), 14000)
	if rate := window.sample(start.Add(12*time.Second), 16000); rate != 1000 {
		t.Fatalf("elapsed time accounting: %d", rate)
	}
}

func TestTransferMeterCombinesConcurrentWorkers(t *testing.T) {
	meter := &transferMeter{}
	ctx := context.WithValue(context.Background(), transferMeterKey{}, meter)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for n := 0; n < 1000; n++ {
				recordTransfer(ctx, 128)
			}
		}()
	}
	workers.Wait()
	if meter.bytes.Load() != 8*1000*128 {
		t.Fatal("lost concurrent transfer bytes")
	}
	fresh := &transferMeter{}
	if fresh.bytes.Load() != 0 {
		t.Fatal("new run inherited transfer bytes")
	}
}

func TestSpeedBeforeChunkCommitAndPause(t *testing.T) {
	content := bytes.Repeat([]byte{42}, 2*1024*1024)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			t.Errorf("missing Range: %v", err)
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
		w.WriteHeader(206)
		if end-start < 128*1024 {
			_, _ = w.Write(content[start : end+1])
			return
		}
		_, _ = w.Write(content[start : start+128*1024])
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = w.Write(content[start+128*1024 : end+1])
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	store, err := newStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	task := &Task{ID: makeID(), Name: "slow.bin", Relative: "slow.bin", Destination: dir, Size: int64(len(content)), Status: "QUEUED", Kind: "personal"}
	_ = store.mutate(func(p *Persistent) error {
		p.Settings.ReserveGiB = 0
		p.Settings.Connections = 1
		p.Settings.ChunkMiB = 2
		p.Tasks = append(p.Tasks, task)
		return nil
	})
	engine := newEngine(store, nil)
	engine.client = server.Client()
	engine.sourceOverride = func(context.Context, *Task) (string, error) { return server.URL, nil }
	defer engine.stop()
	engine.launch(task.ID)
	deadline := time.Now().Add(5 * time.Second)
	for {
		current := store.task(task.ID)
		if current.Speed > 0 {
			if current.Downloaded != 0 || len(current.Chunks) != 1 || current.Chunks[0].Done {
				t.Fatal("unverified partial data entered durable checkpoint")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("speed not reported before chunk completion: %+v", current)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := engine.pause(task.ID); err != nil {
		t.Fatal(err)
	}
	if current := store.task(task.ID); current.Status != "PAUSED" || current.Speed != 0 || current.Downloaded != 0 {
		t.Fatalf("pause kept transient speed or progress: %+v", current)
	}
	once.Do(func() { close(release) })
	_ = store.setTask(task.ID, func(task *Task) { task.Status = "QUEUED" })
	engine.launch(task.ID)
	deadline = time.Now().Add(5 * time.Second)
	for {
		current := store.task(task.ID)
		if current.Status == "COMPLETED" {
			if current.Speed != 0 || current.Downloaded != current.Size {
				t.Fatal("completion kept transient speed")
			}
			break
		}
		if current.Status == "FAILED" || time.Now().After(deadline) {
			t.Fatalf("resume failed: %+v", current)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
