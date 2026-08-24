package tail

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchEntriesDiscoversTimestampRotatedFiles(t *testing.T) {
	tmpdir, err := ioutil.TempDir(os.TempDir(), "watch-entries")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	stateDir := filepath.Join(tmpdir, "states") + string(os.PathSeparator)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	staleState := filepath.Join(stateDir, "postgresql-2026-08-23_000000.leash.state")
	if err := ioutil.WriteFile(staleState, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(tmpdir, "postgresql-2026-08-24_102517.log")
	if err := ioutil.WriteFile(first, []byte("first line\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conf := Config{
		Paths: []string{filepath.Join(tmpdir, "postgresql-*.log")},
		Type:  RotateStyleSyslog,
		Options: TailOptions{
			ReadFrom:  "last",
			StateFile: stateDir,
		},
	}
	streams, err := watchEntries(ctx, conf, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	waitForMissing(t, staleState)

	firstStream := receiveStream(t, streams)
	if line := receiveLine(t, firstStream); line != "first line" {
		t.Fatalf("expected first line without a state file, got %q", line)
	}
	second := filepath.Join(tmpdir, "postgresql-2026-08-24_105232.log")
	if err := ioutil.WriteFile(second, []byte("second line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	secondStream := receiveStream(t, streams)
	if line := receiveLine(t, secondStream); line != "second line" {
		t.Fatalf("expected second line, got %q", line)
	}
	waitForClosed(t, firstStream)

	for _, logfile := range []string{first, second} {
		stateFile := filepath.Join(stateDir, stringsTrimLogSuffix(filepath.Base(logfile))+".leash.state")
		waitForFile(t, stateFile)
	}
	firstState := filepath.Join(stateDir, stringsTrimLogSuffix(filepath.Base(first))+".leash.state")
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	waitForMissing(t, firstState)

	secondState := filepath.Join(stateDir, stringsTrimLogSuffix(filepath.Base(second))+".leash.state")
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	waitForClosed(t, secondStream)
	waitForMissing(t, secondState)
}

func receiveStream(t *testing.T, streams <-chan chan string) chan string {
	select {
	case stream := <-streams:
		return stream
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for logfile stream")
	}
	return nil
}

func receiveLine(t *testing.T, stream chan string) string {
	select {
	case line := <-stream:
		return line
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for logfile line")
	}
	return ""
}

func waitForClosed(t *testing.T, stream chan string) {
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("old logfile stream emitted an unexpected line")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("old logfile stream was not closed after rotation")
	}
}

func waitForFile(t *testing.T, path string) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state file %s was not created", path)
}

func waitForMissing(t *testing.T, path string) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state file %s was not removed", path)
}

func stringsTrimLogSuffix(name string) string {
	if len(name) >= 4 && name[len(name)-4:] == ".log" {
		return name[:len(name)-4]
	}
	return name
}
