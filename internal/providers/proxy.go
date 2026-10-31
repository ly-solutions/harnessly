package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The ChatGPT profile runs Claude Code on the models of a ChatGPT Plus or Pro subscription through claude-code-proxy
// (github.com/raine/claude-code-proxy), an Anthropic-compatible server on the loopback that holds the person's sign-in
// of ChatGPT and renews it. The runtime starts the proxy when a chat or a test needs it and reads whether it is signed
// in; the sign-in itself is the proxy's own (`codex auth device`, driven by loginflow).
//
// The proxy takes no credential from its clients: whatever answers at its address gets the chat (the code, the files
// the agent reads) and may answer for it. So the runtime talks only to a proxy it started itself: on a port it picks,
// bound to the loopback, and checked to be held by that very process of the person's own before the address goes into
// a chat's settings. A listener it did not start is never taken for the proxy, on the proxy's default port or any
// other.

// ProxyName is the proxy's program.
const ProxyName = "claude-code-proxy"

// ErrProxyMissing: claude-code-proxy is not installed on the machine.
var ErrProxyMissing = errors.New("providers: claude-code-proxy is not installed")

// Limits of the proxy (variables so that a test can shorten them).
var (
	// ProxyStartTimeout is how long a started proxy has to listen.
	ProxyStartTimeout = 15 * time.Second
	// proxyDial bounds one look at the port.
	proxyDial = 300 * time.Millisecond
)

// FindProxy is the proxy's program: the first on path (the person's login-shell PATH), else ~/.local/bin's, where its
// installer puts it.
func FindProxy(path, home string) (string, error) {
	dirs := filepath.SplitList(path)
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		bin := filepath.Join(dir, ProxyName)
		if st, err := os.Stat(bin); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return bin, nil
		}
	}
	return "", ErrProxyMissing
}

// Proxy keeps a claude-code-proxy of the runtime's own listening for the Proxied presets. One the runtime starts runs in
// a session of its own and is left running when the runtime stops (the chats on it live in terminals that outlast the
// runtime); its pid and port are noted in State, and a runtime started later goes on with it once it has checked that
// the pid still holds that port and is the program.
type Proxy struct {
	// Bin finds the program; ErrProxyMissing when it is not installed.
	Bin func() (string, error)
	// Env is the environment it starts with, built by the caller from the person's own (HOME, PATH, proxies…), so none
	// of the runtime's tokens reach it.
	Env func() []string
	// Log is the file its output is appended to; "" drops it.
	Log string
	// State is the file the proxy the runtime started is noted in; "" notes nothing, and a later runtime starts its own.
	State string

	// sameProgram says whether the process is bin; nil compares /proc/<pid>/exe with it. A test's fake proxy is a script.
	sameProgram func(pid int, bin string) bool

	mu   sync.Mutex
	pid  int
	port int
}

// proxyState is what State holds.
type proxyState struct {
	PID  int `json:"pid"`
	Port int `json:"port"`
}

// Installed says whether the program is on the machine.
func (p *Proxy) Installed() bool {
	_, err := p.Bin()
	return err == nil
}

func proxyURL(port int) string { return "http://127.0.0.1:" + strconv.Itoa(port) }

// Ensure has the runtime's own proxy listening and gives its address (http://127.0.0.1:<port>): the one it knows if
// that one still holds its port, else the one State notes if it passes the same check, else a new one on a free port.
func (p *Proxy) Ensure(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	bin, err := p.Bin()
	if err != nil {
		return "", ErrProxyMissing
	}
	if p.pid != 0 && p.holds(p.pid, p.port) {
		return proxyURL(p.port), nil
	}
	if st, ok := p.noted(); ok && p.isProgram(st.PID, bin) && p.holds(st.PID, st.Port) {
		p.pid, p.port = st.PID, st.Port
		return proxyURL(p.port), nil
	}
	p.pid, p.port = 0, 0
	return p.start(ctx, bin)
}

func (p *Proxy) start(ctx context.Context, bin string) (string, error) {
	port, err := freePort()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, "serve", "--no-monitor", "--port", strconv.Itoa(port))
	// The loopback whatever the person's config.json says: the proxy takes no credential from its clients.
	cmd.Env = append(p.Env(), "CCP_BIND_ADDRESS=127.0.0.1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if p.Log != "" {
		if err := os.MkdirAll(filepath.Dir(p.Log), 0o700); err != nil {
			return "", err
		}
		f, err := os.OpenFile(p.Log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			return "", err
		}
		defer f.Close()
		cmd.Stdout, cmd.Stderr = f, f
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	exited := make(chan error, 1)
	// The wait reaps it whenever it ends, now or long after the runtime asked.
	go func() { exited <- cmd.Wait() }()
	deadline := time.NewTimer(ProxyStartTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	pid := cmd.Process.Pid
	for {
		select {
		case err := <-exited:
			// Another process took the port first, among others: the proxy does not bind and ends.
			return "", errors.Join(errors.New("providers: claude-code-proxy stopped as it started"), err)
		case <-deadline.C:
			_ = cmd.Process.Kill()
			return "", errors.New("providers: claude-code-proxy did not start listening")
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return "", ctx.Err()
		case <-tick.C:
			// What answers at the port must be this process: a listener that took the port before it is not.
			if !p.holds(pid, port) {
				continue
			}
			p.pid, p.port = pid, port
			p.note(proxyState{PID: pid, Port: port})
			return proxyURL(port), nil
		}
	}
}

// freePort is a port of the loopback nothing listens on now.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (p *Proxy) noted() (proxyState, bool) {
	if p.State == "" {
		return proxyState{}, false
	}
	raw, err := os.ReadFile(p.State)
	if err != nil {
		return proxyState{}, false
	}
	var st proxyState
	if json.Unmarshal(raw, &st) != nil || st.PID <= 0 || st.Port <= 0 {
		return proxyState{}, false
	}
	return st, true
}

func (p *Proxy) note(st proxyState) {
	if p.State == "" {
		return
	}
	raw, _ := json.Marshal(st)
	if os.MkdirAll(filepath.Dir(p.State), 0o700) != nil {
		return
	}
	tmp := p.State + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		_ = os.Rename(tmp, p.State)
	}
}

func (p *Proxy) isProgram(pid int, bin string) bool {
	if p.sameProgram != nil {
		return p.sameProgram(pid, bin)
	}
	exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return false
	}
	want, err := filepath.EvalSymlinks(bin)
	return err == nil && exe == want
}

// holds says whether the process pid, of this user, listens at the loopback port: it answers there, and the
// listening socket of the port (/proc/net/tcp) is one of the process's own descriptors. Where /proc cannot tell, no
// process is taken to hold a port.
func (p *Proxy) holds(pid, port int) bool {
	dir := "/proc/" + strconv.Itoa(pid)
	fi, err := os.Stat(dir)
	if err != nil {
		return false
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
		return false
	}
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), proxyDial)
	if err != nil {
		return false
	}
	_ = c.Close()
	inode, ok := listenInode(port)
	if !ok {
		return false
	}
	fds, err := os.ReadDir(dir + "/fd")
	if err != nil {
		return false
	}
	want := "socket:[" + inode + "]"
	for _, fd := range fds {
		if link, err := os.Readlink(dir + "/fd/" + fd.Name()); err == nil && link == want {
			return true
		}
	}
	return false
}

// listenInode is the inode of the socket of this user that listens on the port over IPv4 (/proc/net/tcp).
func listenInode(port int) (string, bool) {
	f, err := os.Open("/proc/net/tcp")
	if err != nil {
		return "", false
	}
	defer f.Close()
	uid := strconv.Itoa(os.Getuid())
	sc := bufio.NewScanner(f)
	sc.Scan() // the header
	for sc.Scan() {
		// sl local_address rem_address st tx:rx tr:when retrnsmt uid timeout inode
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 || fields[3] != "0A" || fields[7] != uid {
			continue
		}
		_, hexPort, ok := strings.Cut(fields[1], ":")
		if n, err := strconv.ParseUint(hexPort, 16, 16); ok && err == nil && int(n) == port {
			return fields[9], true
		}
	}
	return "", false
}
