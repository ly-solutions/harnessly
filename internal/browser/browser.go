// Package browser owns the machine's persistent, interactive Chrome and its private VNC endpoint.
package browser

import (
	"context"
	"errors"
	"fmt"
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

type Config struct {
	Home    string
	DataDir string
}

type process interface {
	Done() <-chan struct{}
	Close() error
}

type Service struct {
	cfg Config

	mu           sync.Mutex
	procs        []process
	dead         chan struct{}
	wsPort       int
	start        func(name string, args, env []string) (process, error)
	lookPath     func(string) (string, error)
	freePort     func() (int, error)
	ready        func(context.Context, int) error
	displayReady func(context.Context, string) error
}

func New(cfg Config) *Service {
	s := &Service{cfg: cfg, lookPath: exec.LookPath, freePort: freePort, ready: ready, displayReady: displayReady}
	s.start = startProcess
	return s
}

// Endpoint starts the shared stack on first use and returns its loopback websockify port.
func (s *Service) Endpoint(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead != nil {
		select {
		case <-s.dead:
			_ = s.closeLocked()
		default:
		}
	}
	if s.wsPort != 0 {
		return s.wsPort, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.startLocked(ctx); err != nil {
		_ = s.closeLocked()
		return 0, err
	}
	return s.wsPort, nil
}

func (s *Service) startLocked(ctx context.Context) error {
	profile := filepath.Join(s.cfg.DataDir, "browser", "chrome-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"Xvfb", "x11vnc", "websockify"} {
		if _, err := s.lookPath(name); err != nil {
			return fmt.Errorf("remote browser needs %s: %w", name, err)
		}
	}
	chrome := ""
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if path, err := s.lookPath(name); err == nil {
			chrome = path
			break
		}
	}
	if chrome == "" {
		return errors.New("remote browser needs Chrome or Chromium")
	}
	vncPort, err := s.freePort()
	if err != nil {
		return err
	}
	wsPort, err := s.freePort()
	if err != nil {
		return err
	}
	display, err := freeDisplay()
	if err != nil {
		return err
	}
	s.dead = make(chan struct{})
	dead := s.dead
	var once sync.Once
	watch := func(p process) {
		go func() {
			<-p.Done()
			once.Do(func() { close(dead) })
		}()
	}
	startupCtx, cancelStartup := context.WithCancel(ctx)
	defer cancelStartup()
	go func() {
		select {
		case <-dead:
			cancelStartup()
		case <-startupCtx.Done():
		}
	}()
	ctx = startupCtx
	launch := func(name string, args, env []string) error {
		p, err := s.start(name, args, env)
		if err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}
		s.procs = append(s.procs, p)
		watch(p)
		return nil
	}
	if err := launch("Xvfb", []string{display, "-screen", "0", "1440x900x24", "-nolisten", "tcp"}, nil); err != nil {
		return err
	}
	if err := s.displayReady(ctx, display); err != nil {
		return fmt.Errorf("remote browser display did not start: %w", err)
	}
	if err := launch("x11vnc", []string{"-display", display, "-rfbport", strconv.Itoa(vncPort), "-localhost", "-forever", "-shared", "-nopw"}, nil); err != nil {
		return err
	}
	if err := s.ready(ctx, vncPort); err != nil {
		return fmt.Errorf("remote browser VNC did not start: %w", err)
	}
	chromeArgs := []string{
		"--no-first-run", "--no-default-browser-check", "--disable-session-crashed-bubble",
		"--disable-dev-shm-usage", "--window-size=1440,900", "--window-position=0,0",
		"--user-data-dir=" + profile, "--restore-last-session",
	}
	if extra := strings.Fields(os.Getenv("RUNTIME_BROWSER_CHROME_ARGS")); len(extra) > 0 {
		chromeArgs = append(chromeArgs, extra...)
	}
	if err := launch(chrome, chromeArgs, []string{"HOME=" + s.cfg.Home, "DISPLAY=" + display}); err != nil {
		return err
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(wsPort))
	target := net.JoinHostPort("127.0.0.1", strconv.Itoa(vncPort))
	if err := launch("websockify", []string{address, target}, nil); err != nil {
		return err
	}
	if err := s.ready(ctx, wsPort); err != nil {
		return fmt.Errorf("remote browser did not start: %w", err)
	}
	select {
	case <-dead:
		return errors.New("a remote browser process exited during startup")
	default:
	}
	s.wsPort = wsPort
	return nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func freeDisplay() (string, error) {
	for n := 99; n < 199; n++ {
		if _, err := os.Stat(filepath.Join("/tmp/.X11-unix", "X"+strconv.Itoa(n))); errors.Is(err, os.ErrNotExist) {
			return ":" + strconv.Itoa(n), nil
		}
	}
	return "", errors.New("no free X display for remote browser")
}

func displayReady(ctx context.Context, display string) error {
	socket := filepath.Join("/tmp/.X11-unix", "X"+strings.TrimPrefix(display, ":"))
	for {
		if _, err := os.Stat(socket); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func ready(ctx context.Context, port int) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type execProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func startProcess(name string, args, env []string) (process, error) {
	// #nosec G204 -- only the fixed browser stack is launched; no request data reaches the command or its arguments.
	cmd := exec.Command(name, args...)
	cmd.Env = withEnv(os.Environ(), env...)
	p := &execProcess{cmd: cmd, done: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func withEnv(base []string, extra ...string) []string {
	out := append([]string{}, base...)
	for _, kv := range extra {
		key, _, _ := strings.Cut(kv, "=")
		filtered := out[:0]
		for _, old := range out {
			if !strings.HasPrefix(old, key+"=") {
				filtered = append(filtered, old)
			}
		}
		out = append(filtered, kv)
	}
	return out
}

func (p *execProcess) Done() <-chan struct{} { return p.done }

func (p *execProcess) Close() error {
	if p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
		return nil
	case <-time.After(2 * time.Second):
	}
	return p.cmd.Process.Kill()
}

func (s *Service) closeLocked() error {
	var errs []error
	for i := len(s.procs) - 1; i >= 0; i-- {
		if err := s.procs[i].Close(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			errs = append(errs, err)
		}
	}
	s.procs = nil
	s.dead = nil
	s.wsPort = 0
	return errors.Join(errs...)
}

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}
