package main

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The profiles are gzip'd pprof, one set per signal, and the folder keeps the newest few.
func TestWriteProfilesKeepsTheNewestSets(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 8, 22, 0, 0, 0, time.UTC)
	for i := range profileSets + 2 {
		if _, err := writeProfiles(dir, at.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != profileSets*len(profileNames) {
		t.Fatalf("%d files, want %d", len(entries), profileSets*len(profileNames))
	}
	if _, err := os.Stat(filepath.Join(dir, "heap-20261108-220000.pprof")); !os.IsNotExist(err) {
		t.Fatal("the oldest set was kept")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "heap-20261108-220004.pprof"))
	if err != nil || !bytes.HasPrefix(raw, []byte{0x1f, 0x8b}) {
		t.Fatalf("not a gzip'd profile: %v", err)
	}
}

// SIGUSR1 writes a set.
func TestProfileOnSignal(t *testing.T) {
	dir := t.TempDir()
	profileOnSignal(dir, 20*time.Millisecond)
	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	for range 250 {
		g, _ := filepath.Glob(filepath.Join(dir, "goroutine-*.pprof"))
		c, _ := filepath.Glob(filepath.Join(dir, "cpu-*.pprof"))
		if len(g) == 1 && len(c) == 1 && !cpuProfiling.Load() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no profiles after SIGUSR1")
}

// waitCPUProfilerFree waits out a CPU profile another test's signal started: one runs at a time.
func waitCPUProfilerFree(t *testing.T) {
	t.Helper()
	for range 250 {
		if !cpuProfiling.Load() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the CPU profiler stayed busy")
}

// The CPU profile of a signal lands beside its set and goes with it when the set is pruned.
func TestCPUProfileIsWrittenBesideTheSet(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	stamp, err := writeProfiles(dir, at)
	if err != nil {
		t.Fatal(err)
	}
	waitCPUProfilerFree(t)
	if err := writeCPUProfile(dir, stamp, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dir, "cpu-"+stamp+".pprof")); err != nil || st.Size() == 0 {
		t.Fatalf("cpu profile: %v", err)
	}
	for i := 1; i <= profileSets; i++ {
		if _, err := writeProfiles(dir, at.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "cpu-"+stamp+".pprof")); !os.IsNotExist(err) {
		t.Fatalf("the pruned set left its cpu profile: %v", err)
	}
}
