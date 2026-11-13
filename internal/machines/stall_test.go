package machines

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/wire"
)

func (f *healthHost) put(name, body string) {
	f.t.Helper()
	path := filepath.Join(f.root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *healthHost) stallMonitor(cores int) *HealthMonitor {
	f.write()
	return NewHealthMonitor(HealthDeps{ProcRoot: f.root, Path: "/", Now: func() time.Time { return f.now }, Cores: cores,
		Disk: func(string) (float64, float64, error) { return f.total, f.free, nil }})
}

// hold steps the clock for d with the CPU this busy and answers the last verdict.
func (f *healthHost) hold(h *HealthMonitor, d time.Duration, busyShare float64) wire.MachineHealth {
	var got wire.MachineHealth
	for elapsed := time.Duration(0); elapsed < d; elapsed += HealthEvery {
		got = f.step(h, HealthEvery, busyShare)
	}
	return got
}

const nfsStats = `device /dev/vda1 mounted on / with fstype ext4
device 10.0.0.7:/srv/projects mounted on /home/dev/my\040projects with fstype nfs4 statvers=1.1
	opts:	rw,vers=4.2,hard
	xprt:	tcp 0 0 1 0 24 %d %d 0 3857 0 2 0 0
`

// The case of 13.11: the load is in the hundreds, the processors idle, the network folder's server gone.
func TestStallIsALoadTheProcessorsDoNotExplain(t *testing.T) {
	f := newHealthHost(t)
	h := f.stallMonitor(8)
	f.put("loadavg", "1.20 1.00 0.90 2/900 4242\n")
	f.put("self/mountstats", fmt.Sprintf(nfsStats, 100, 100))
	if got := f.hold(h, time.Minute, 0.05); got.Level != wire.HealthOK || got.Stall != nil {
		t.Fatalf("a quiet machine: %+v", got)
	}

	f.put("loadavg", "312.40 250.00 120.00 1/2900 4242\n")
	f.put("self/mountstats", fmt.Sprintf(nfsStats, 140, 100))
	f.put("101/stat", "101 (git) D 1 101 101 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 100 0 0\n")
	f.put("102/stat", "102 (git) D 1 102 102 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 100 0 0\n")
	f.put("103/stat", "103 (node (x)) D 1 103 103 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 100 0 0\n")
	f.put("104/stat", "104 (bash) S 1 104 104 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 100 0 0\n")
	if got := f.step(h, HealthEvery, 0.05); got.Level != wire.HealthOK {
		t.Fatalf("one look is not a stall yet: %+v", got)
	}
	got := f.hold(h, HealthLimits.riseAfter+HealthLimits.mountSilentAfter, 0.05)
	if levelOf(got, wire.HealthIO) != wire.HealthCritical || levelOf(got, wire.HealthCPU) != wire.HealthOK {
		t.Fatalf("a load of 312 on 8 idle cores is a critical stall, not a busy CPU: %+v", got)
	}
	want := &wire.HealthStall{Mounts: []string{"/home/dev/my projects"}, Waiting: []string{"git", "node (x)"}}
	if !reflect.DeepEqual(got.Stall, want) {
		t.Fatalf("stall = %+v, want %+v", got.Stall, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}

	// The server answers again and the load falls: the level is left only after it held, and the stall goes with it.
	f.put("loadavg", "0.80 90.00 100.00 1/900 4242\n")
	f.put("self/mountstats", fmt.Sprintf(nfsStats, 150, 150))
	if got := f.step(h, HealthEvery, 0.05); levelOf(got, wire.HealthIO) != wire.HealthCritical || got.Stall == nil {
		t.Fatalf("one good look does not take the level off: %+v", got)
	}
	if got := f.hold(h, HealthLimits.fallAfter+HealthEvery, 0.05); got.Level != wire.HealthOK || got.Stall != nil {
		t.Fatalf("after the hold the machine is fine: %+v", got)
	}
}

func TestABusyProcessorIsNotAStall(t *testing.T) {
	f := newHealthHost(t)
	h := f.stallMonitor(8)
	f.put("loadavg", "64.00 60.00 50.00 40/900 4242\n")
	got := f.hold(h, 3*time.Minute, 0.97)
	if levelOf(got, wire.HealthIO) != wire.HealthOK || got.Stall != nil {
		t.Fatalf("a load the processors explain is theirs: %+v", got)
	}
	if levelOf(got, wire.HealthCPU) != wire.HealthWarn {
		t.Fatalf("the CPU says it: %+v", got)
	}
}

func TestStallLevelKeepsAMargin(t *testing.T) {
	cases := []struct {
		perCore, busy float64
		prev, want    wire.HealthLevel
	}{
		{1.9, 10, wire.HealthOK, wire.HealthOK},
		{2, 10, wire.HealthOK, wire.HealthWarn},
		{1.6, 10, wire.HealthWarn, wire.HealthWarn},
		{1.4, 10, wire.HealthWarn, wire.HealthOK},
		{4, 10, wire.HealthWarn, wire.HealthCritical},
		{3.1, 10, wire.HealthCritical, wire.HealthCritical},
		{2.9, 10, wire.HealthCritical, wire.HealthWarn},
		{9, 55, wire.HealthOK, wire.HealthOK},
		{9, 60, wire.HealthCritical, wire.HealthCritical},
		{9, 70, wire.HealthCritical, wire.HealthOK},
	}
	for _, c := range cases {
		if got := stallLevel(c.perCore, c.busy, true, c.prev); got != c.want {
			t.Errorf("stallLevel(%v, %v, %s) = %s, want %s", c.perCore, c.busy, c.prev, got, c.want)
		}
	}
	if got := stallLevel(9, 0, false, wire.HealthWarn); got != wire.HealthWarn {
		t.Errorf("without a measured CPU the level stays: %s", got)
	}
}

func TestAnIdleMountIsNotSilent(t *testing.T) {
	f := newHealthHost(t)
	h := f.stallMonitor(8)
	f.put("self/mountstats", fmt.Sprintf(nfsStats, 100, 99))
	for i := 0; i < 20; i++ {
		f.now = f.now.Add(HealthEvery)
		if got := h.silentMounts(f.now); len(got) != 0 {
			t.Fatalf("nothing was asked since it was first seen: %v", got)
		}
	}
	f.put("self/mountstats", fmt.Sprintf(nfsStats, 101, 99))
	f.now = f.now.Add(HealthEvery)
	if got := h.silentMounts(f.now); len(got) != 1 {
		t.Fatalf("asked and unanswered for minutes: %v", got)
	}
	f.put("self/mountstats", fmt.Sprintf(nfsStats, 101, 100))
	f.now = f.now.Add(HealthEvery)
	if got := h.silentMounts(f.now); len(got) != 0 {
		t.Fatalf("it answered: %v", got)
	}
}
