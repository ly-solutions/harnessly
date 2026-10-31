package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// fakeProxy is a Proxy over a small Python listener that stands in for claude-code-proxy: it notes its arguments and
// pid in its folder and listens on the port it is given. No account, no network. state is the Proxy's State file.
func fakeProxy(t *testing.T, state string) *Proxy {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3 for a fake proxy")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, ProxyName)
	script := "#!/bin/sh\necho \"$@\" > \"$OUT/argv\"\necho \"$CCP_BIND_ADDRESS\" > \"$OUT/bind\"\necho $$ >> \"$OUT/pids\"\nexec " + py +
		` -c 'import socket,sys,time
s=socket.socket(); s.bind(("127.0.0.1", int(sys.argv[1]))); s.listen(4)
time.sleep(60)' "$4"` + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	// The pids it started end with the test: a started proxy outlives whoever started it.
	t.Cleanup(func() {
		raw, _ := os.ReadFile(filepath.Join(dir, "pids"))
		for _, line := range strings.Fields(string(raw)) {
			if pid, err := strconv.Atoi(line); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	p := &Proxy{
		Bin:   func() (string, error) { return bin, nil },
		Env:   func() []string { return []string{"OUT=" + dir, "PATH=/usr/bin:/bin"} },
		Log:   filepath.Join(dir, "log", "proxy.log"),
		State: state,
	}
	// The script execs Python: the process is the fake's when it is one this test started.
	p.sameProgram = func(pid int, _ string) bool { return p.ownPID(pid) }
	return p
}

func outOf(t *testing.T, p *Proxy, name string) string {
	t.Helper()
	bin, _ := p.Bin()
	raw, _ := os.ReadFile(filepath.Join(filepath.Dir(bin), name))
	return strings.TrimSpace(string(raw))
}

func TestFindProxyLooksOnPathThenInLocalBin(t *testing.T) {
	home, onPath := t.TempDir(), t.TempDir()
	if _, err := FindProxy(onPath, home); !errors.Is(err, ErrProxyMissing) {
		t.Fatalf("absent: %v", err)
	}
	local := filepath.Join(home, ".local", "bin", ProxyName)
	_ = os.MkdirAll(filepath.Dir(local), 0o700)
	if err := os.WriteFile(local, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := FindProxy(onPath, home); err != nil || got != local {
		t.Fatalf("local bin: %s %v", got, err)
	}
	first := filepath.Join(onPath, ProxyName)
	if err := os.WriteFile(first, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := FindProxy(onPath, home); err != nil || got != first {
		t.Fatalf("path first: %s %v", got, err)
	}
}

// The proxy is started on a port it is given, bound to the loopback, noted, and gone on with while it holds the port.
func TestTheProxyIsStartedOnAPortOfItsOwnAndKept(t *testing.T) {
	state := filepath.Join(t.TempDir(), "proxy.json")
	p := fakeProxy(t, state)
	url, err := p.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if url != proxyURL(p.port) || outOf(t, p, "argv") != "serve --no-monitor --port "+strconv.Itoa(p.port) || outOf(t, p, "bind") != "127.0.0.1" {
		t.Fatalf("url %s argv %q bind %q", url, outOf(t, p, "argv"), outOf(t, p, "bind"))
	}
	var noted proxyState
	if raw, err := os.ReadFile(state); err != nil || json.Unmarshal(raw, &noted) != nil || noted.Port != p.port || noted.PID != p.pid {
		t.Fatalf("state %+v %v", noted, err)
	}
	if again, err := p.Ensure(context.Background()); err != nil || again != url {
		t.Fatalf("again: %s %v", again, err)
	}
	// A runtime started later goes on with the proxy it finds noted.
	later := fakeProxy(t, state)
	later.sameProgram = p.sameProgram
	if got, err := later.Ensure(context.Background()); err != nil || got != url {
		t.Fatalf("later: %s %v", got, err)
	}
	if n := len(strings.Fields(outOf(t, p, "pids"))); n != 1 {
		t.Fatalf("%d proxies started", n)
	}
}

// A listener the runtime did not start is never taken for the proxy, though it answers at the noted port and the noted
// pid is a live process of the same user.
func TestAForeignListenerIsNotTakenForTheProxy(t *testing.T) {
	foreign, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	taken := foreign.Addr().(*net.TCPAddr).Port
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Process.Kill(); _ = other.Wait() }()
	state := filepath.Join(t.TempDir(), "proxy.json")
	for name, noted := range map[string]proxyState{
		"another process at the port": {PID: other.Process.Pid, Port: taken},
		// This test's own process holds the port, but is not the program.
		"not the program": {PID: os.Getpid(), Port: taken},
	} {
		raw, _ := json.Marshal(noted)
		if err := os.WriteFile(state, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		p := fakeProxy(t, state)
		if name == "another process at the port" {
			// Even taken for the program, it does not hold the port.
			p.sameProgram = func(pid int, _ string) bool { return pid == other.Process.Pid || p.ownPID(pid) }
		}
		url, err := p.Ensure(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if url == proxyURL(taken) || p.port == taken || len(strings.Fields(outOf(t, p, "pids"))) != 1 {
			t.Fatalf("%s: went to %s", name, url)
		}
	}
	// The default port of claude-code-proxy is no exception: a proxy is never assumed there.
	if p := fakeProxy(t, ""); p.holds(os.Getpid(), 18765) {
		t.Fatal("the default port taken for the proxy")
	}
}

func TestTheProxyMissingOrDeadIsSaid(t *testing.T) {
	missing := &Proxy{Bin: func() (string, error) { return "", ErrProxyMissing }}
	if _, err := missing.Ensure(context.Background()); !errors.Is(err, ErrProxyMissing) || missing.Installed() {
		t.Fatalf("missing: %v", err)
	}
	dir := t.TempDir()
	quits := filepath.Join(dir, "quits")
	_ = os.WriteFile(quits, []byte("#!/bin/sh\nexit 3\n"), 0o700)
	bad := &Proxy{Bin: func() (string, error) { return quits, nil }, Env: func() []string { return nil }}
	if _, err := bad.Ensure(context.Background()); err == nil {
		t.Fatal("a proxy that stopped started")
	}
}

// ownPID says whether the fake started pid.
func (p *Proxy) ownPID(pid int) bool {
	bin, _ := p.Bin()
	raw, _ := os.ReadFile(filepath.Join(filepath.Dir(bin), "pids"))
	return strings.Contains("\n"+string(raw), "\n"+strconv.Itoa(pid)+"\n")
}
