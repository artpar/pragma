package background

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn process: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		// Exit status is irrelevant; the process is gone either way.
		_ = err
	}
	return cmd.Process.Pid
}

func TestRegisterRejectsInvalidPID(t *testing.T) {
	r := &Registry{dir: t.TempDir()}
	if err := r.Register(ProcessInfo{PID: -1, Status: "starting"}); err == nil {
		t.Fatal("Register must refuse PID <= 0 instead of writing an unreachable record")
	}
	if _, err := os.Stat(filepath.Join(r.dir, "-1.json")); !os.IsNotExist(err) {
		t.Fatal("no -1.json must be written for an invalid PID")
	}
}

func TestListProcessesSweepsOrphanedRecords(t *testing.T) {
	dir := t.TempDir()
	r := &Registry{dir: dir}
	stale := time.Now().Add(-48 * time.Hour)
	// Observed artifact shape: gogent-era record with invalid PID.
	writeFile(t, dir, "-1.json", ProcessInfo{PID: -1, PGID: -1,
		StartedAt: stale, UpdatedAt: stale, Status: "starting"})
	// Pattern-matching record whose process is dead and heartbeat is stale.
	pid := deadPID(t)
	writeFile(t, dir, filepath.Base(r.pidPath(pid)), ProcessInfo{PID: pid,
		HeartbeatAt: stale, StartedAt: stale, UpdatedAt: stale, Status: "running"})
	// Live record with a fresh heartbeat must survive and be listed.
	now := time.Now()
	live := os.Getpid()
	writeFile(t, dir, filepath.Base(r.pidPath(live)), ProcessInfo{PID: live,
		HeartbeatAt: now, StartedAt: now, UpdatedAt: now, Status: "running"})
	got, err := r.ListProcesses()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PID != live {
		t.Fatalf("ListProcesses() = %v, want exactly the live PID %d", got, live)
	}
	for _, gone := range []string{"-1.json", filepath.Base(r.pidPath(pid))} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("orphaned record %s must be swept", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.Base(r.pidPath(live)))); err != nil {
		t.Errorf("live record must survive: %v", err)
	}
}

func TestListProcessesKeepsAliveButStaleHeartbeat(t *testing.T) {
	dir := t.TempDir()
	r := &Registry{dir: dir}
	stale := time.Now().Add(-48 * time.Hour)
	// Live process (this test), stale heartbeat: conservative keep — the
	// sweep must not remove a record whose process is still alive.
	pid := os.Getpid()
	name := filepath.Base(r.pidPath(pid))
	writeFile(t, dir, name, ProcessInfo{PID: pid,
		HeartbeatAt: stale, StartedAt: stale, UpdatedAt: stale, Status: "running"})
	if _, err := r.ListProcesses(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatalf("record for live process with stale heartbeat must be kept: %v", err)
	}
}

func TestListProcessesLeavesUnrecognizedFiles(t *testing.T) {
	dir := t.TempDir()
	r := &Registry{dir: dir}
	if err := os.WriteFile(filepath.Join(dir, "notes.json"), []byte(`{"hello":"world"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// PID-named file whose content is not a registry record: leave alone.
	if err := os.WriteFile(filepath.Join(dir, "99999.json"), []byte(`not-a-record`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ListProcesses(); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"notes.json", "99999.json"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("unrecognized file %s must be left untouched: %v", keep, err)
		}
	}
}
