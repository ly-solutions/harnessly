package browser

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeProcess struct {
	done chan struct{}
	once sync.Once
}

func newFakeProcess() *fakeProcess { return &fakeProcess{done: make(chan struct{})} }

func (p *fakeProcess) Done() <-chan struct{} { return p.done }
func (p *fakeProcess) Close() error {
	p.once.Do(func() { close(p.done) })
	return nil
}

type fakeStart struct {
	name string
	args []string
	env  []string
	proc *fakeProcess
}

func TestServiceStartsOneStackWithPersistentProfile(t *testing.T) {
	var starts []fakeStart
	dataDir := t.TempDir()
	s := New(Config{Home: "/home/person", DataDir: dataDir})
	s.lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	ports := []int{5900, 6080}
	s.freePort = func() (int, error) {
		port := ports[0]
		ports = ports[1:]
		return port, nil
	}
	var readyPorts []int
	displayReady := ""
	s.ready = func(_ context.Context, port int) error {
		readyPorts = append(readyPorts, port)
		return nil
	}
	s.displayReady = func(_ context.Context, display string) error {
		displayReady = display
		return nil
	}
	s.start = func(name string, args, env []string) (process, error) {
		p := newFakeProcess()
		starts = append(starts, fakeStart{name: name, args: slices.Clone(args), env: slices.Clone(env), proc: p})
		return p, nil
	}

	port, err := s.Endpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if port != 6080 {
		t.Fatalf("port = %d", port)
	}
	if len(starts) != 4 {
		t.Fatalf("started %d processes", len(starts))
	}
	if displayReady != starts[0].args[0] {
		t.Fatalf("display readiness = %q, Xvfb display = %q", displayReady, starts[0].args[0])
	}
	if !slices.Equal(readyPorts, []int{5900, 6080}) {
		t.Fatalf("readiness checks = %v, want VNC then WebSocket", readyPorts)
	}
	if starts[0].name != "Xvfb" || starts[1].name != "x11vnc" || filepath.Base(starts[2].name) != "google-chrome" || starts[3].name != "websockify" {
		t.Fatalf("wrong processes: %#v", starts)
	}
	profile := filepath.Join(dataDir, "browser", "chrome-profile")
	if !slices.Contains(starts[2].args, "--user-data-dir="+profile) {
		t.Fatalf("Chrome does not use the persistent profile: %v", starts[2].args)
	}
	if !slices.Contains(starts[2].args, "--restore-last-session") {
		t.Fatalf("Chrome does not restore the previous tabs: %v", starts[2].args)
	}
	if !slices.Contains(starts[2].env, "DISPLAY="+starts[0].args[0]) {
		t.Fatalf("Chrome has no display: %v", starts[2].env)
	}

	if _, err := s.Endpoint(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(starts) != 4 {
		t.Fatalf("second connection started more processes: %d", len(starts))
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, start := range starts {
		select {
		case <-start.proc.done:
		default:
			t.Errorf("%s was not stopped", start.name)
		}
	}
}

func TestServiceRestartsAfterProcessExitWithoutReplacingProfile(t *testing.T) {
	var starts []fakeStart
	var first *fakeProcess
	dataDir := t.TempDir()
	s := New(Config{Home: "/home/person", DataDir: dataDir})
	s.lookPath = func(name string) (string, error) { return name, nil }
	ports := []int{5901, 6081, 5902, 6082}
	s.freePort = func() (int, error) {
		port := ports[0]
		ports = ports[1:]
		return port, nil
	}
	s.ready = func(context.Context, int) error { return nil }
	s.displayReady = func(context.Context, string) error { return nil }
	s.start = func(name string, args, env []string) (process, error) {
		p := newFakeProcess()
		if first == nil {
			first = p
		}
		starts = append(starts, fakeStart{name: name, args: args, proc: p})
		return p, nil
	}
	if _, err := s.Endpoint(context.Background()); err != nil {
		t.Fatal(err)
	}
	first.once.Do(func() { close(first.done) })
	deadline := time.Now().Add(time.Second)
	for len(starts) < 8 && time.Now().Before(deadline) {
		if _, err := s.Endpoint(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if len(starts) != 8 {
		t.Fatalf("browser was not restarted: %d processes", len(starts))
	}
	profile := "--user-data-dir=" + filepath.Join(dataDir, "browser", "chrome-profile")
	if !slices.Contains(starts[2].args, profile) || !slices.Contains(starts[6].args, profile) {
		t.Fatalf("profile changed: %v / %v", starts[2].args, starts[6].args)
	}
}

func TestServiceAcceptsChromeStable(t *testing.T) {
	var starts []fakeStart
	s := New(Config{Home: "/home/person", DataDir: t.TempDir()})
	s.lookPath = func(name string) (string, error) {
		if name == "google-chrome" || name == "chromium" || name == "chromium-browser" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}
	ports := []int{5900, 6080}
	s.freePort = func() (int, error) {
		port := ports[0]
		ports = ports[1:]
		return port, nil
	}
	s.ready = func(context.Context, int) error { return nil }
	s.displayReady = func(context.Context, string) error { return nil }
	s.start = func(name string, args, env []string) (process, error) {
		p := newFakeProcess()
		starts = append(starts, fakeStart{name: name, proc: p})
		return p, nil
	}
	defer s.Close()
	if _, err := s.Endpoint(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(starts) != 4 || starts[2].name != "/usr/bin/google-chrome-stable" {
		t.Fatalf("Chrome Stable was not used: %#v", starts)
	}
}

func TestServiceReportsMissingDependencies(t *testing.T) {
	s := New(Config{Home: "/home/person", DataDir: t.TempDir()})
	s.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	_, err := s.Endpoint(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Xvfb") {
		t.Fatalf("error = %v", err)
	}
}
