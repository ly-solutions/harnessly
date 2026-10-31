// Package loginflow signs the machine's agent CLIs, and the proxies of the backend profiles that run through one, in and
// out for the page. The program does the signing in; the service drives it through a terminal of its own and lets the
// page see only what the person needs: the vendor's link, the one-time code of a device flow, and where the flow is.
//
// What it keeps from the CLI's output is the link and that code, taken before anything is typed; after that the output
// is read and thrown away. A code the person pastes back goes to the terminal's input and is dropped; an API key goes
// to a CLI's standard input (never its arguments or environment). Nothing here is logged: not the output, not the code,
// not the key.
package loginflow

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// Limits of the service (variables so that a test can shorten them).
var (
	// FlowTimeout is how long a flow lives: the vendors' own codes last 10 to 15 minutes.
	FlowTimeout = 15 * time.Minute
	// CheckTimeout is how long the CLI has to take a pasted code.
	CheckTimeout = 90 * time.Second
	// StartTimeout is how long the CLI has to print its link.
	StartTimeout = 30 * time.Second
	// KeyTimeout bounds one call that hands a key or signs out.
	KeyTimeout = 30 * time.Second
)

const (
	// StartsPer is how many flows a CLI may start in StartWindow.
	StartsPer   = 5
	StartWindow = 10 * time.Minute
	// grabMax is how much output is looked at for the link before the start gives up.
	grabMax = 16 << 10
)

// Config is what the service needs from the machine.
type Config struct {
	// Bin finds the CLI; an error means it is not installed.
	Bin func(p wire.Provider) (string, error)
	// Env is the environment a CLI starts with, built by the caller from the person's own (HOME, PATH, proxies…): the
	// service adds nothing to it and passes on nothing else, so none of the runtime's tokens reach a CLI.
	Env func() []string
	// OnChange is told when an account changed: a flow ended in a sign-in, a key was saved, the CLI signed out.
	OnChange func(p wire.Provider)
	// ProfileBin finds the proxy of a profile (profileFlows); an error means it is not installed. nil: no profile signs in.
	ProfileBin func(id string) (string, error)
	// OnProfileChange is told when a profile's proxy signed in or out.
	OnProfileChange func(id string)
	// Now is the clock; nil is the system's.
	Now func() time.Time
}

// Service is the wire's LoginSource over the CLIs, and its ProfileLoginSource over the profiles' proxies.
type Service struct {
	cfg Config

	mu    sync.Mutex
	flows map[who]*flow
	log   map[who][]time.Time
}

var (
	_ wire.LoginSource        = (*Service)(nil)
	_ wire.ProfileLoginSource = (*Service)(nil)
)

// New is a service over cfg.
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, flows: map[who]*flow{}, log: map[who][]time.Time{}}
}

// who is whose sign-in a flow is: an agent CLI's, or (profile set) the proxy's of a profile that runs through it.
type who struct {
	provider wire.Provider
	profile  string
}

// profileFlow is how the proxy of a profile signs in: the CLI it runs through, the arguments that sign it in and out
// (always with a device code), and how its link and its code read.
type profileFlow struct {
	via            wire.Provider
	login, logout  []string
	link, codeLine *regexp.Regexp
}

// profileFlows are the profiles whose proxy signs in from the page. Fixed here: nothing of the page goes into them.
var profileFlows = map[string]profileFlow{
	// claude-code-proxy prints «Visit: https://auth.openai.com/codex/device» and «Enter code: XXXX-XXXXX».
	"chatgpt": {via: wire.ProviderClaude, login: []string{"codex", "auth", "device"}, logout: []string{"codex", "auth", "logout"},
		link: codexRE, codeLine: proxyCodeRE},
}

type flow struct {
	who  who
	view wire.LoginFlow
	// in is the terminal's input: set while the flow can take a code.
	term   *os.File
	cancel context.CancelFunc
	// done is closed when the process is reaped.
	done chan struct{}
}

// argv are the arguments of each way of signing in. Fixed here: nothing of the page goes into them.
func argv(p wire.Provider, m wire.LoginMode) ([]string, bool) {
	switch {
	case p == wire.ProviderClaude && m == wire.LoginSubscription:
		return []string{"auth", "login", "--claudeai"}, true
	case p == wire.ProviderClaude && m == wire.LoginConsole:
		return []string{"auth", "login", "--console"}, true
	case p == wire.ProviderCodex && m == wire.LoginDevice:
		return []string{"login", "--device-auth"}, true
	}
	return nil, false
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Service) ms() wire.Ms { return wire.Ms(s.cfg.Now().UnixMilli()) }

func (s *Service) Login(_ context.Context, p wire.Provider) (wire.LoginStatus, error) {
	return s.status(who{provider: p}), nil
}

func (s *Service) status(w who) wire.LoginStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked(w)
}

func (s *Service) statusLocked(w who) wire.LoginStatus {
	f, ok := s.flows[w]
	if !ok {
		return wire.LoginStatus{}
	}
	v := f.view
	return wire.LoginStatus{Flow: &v}
}

func (f *flow) live() bool {
	switch f.view.State {
	case wire.LoginStarting, wire.LoginAwaiting, wire.LoginChecking:
		return true
	}
	return false
}

// StartLogin starts a flow and answers once the CLI has given its link (or failed to), so the page has something to show.
func (s *Service) StartLogin(ctx context.Context, p wire.Provider, mode wire.LoginMode) (wire.LoginStatus, error) {
	args, ok := argv(p, mode)
	if !ok {
		return wire.LoginStatus{}, wire.ErrLoginUnsupported
	}
	return s.start(ctx, who{provider: p}, mode, args)
}

// bin finds the program of a sign-in; ErrLoginNotInstalled when it is not on the machine.
func (s *Service) bin(w who) (string, error) {
	var bin string
	var err error
	switch {
	case w.profile == "":
		bin, err = s.cfg.Bin(w.provider)
	case s.cfg.ProfileBin == nil:
		return "", wire.ErrLoginUnsupported
	default:
		bin, err = s.cfg.ProfileBin(w.profile)
	}
	if err != nil {
		return "", wire.ErrLoginNotInstalled
	}
	return bin, nil
}

func (s *Service) start(ctx context.Context, w who, mode wire.LoginMode, args []string) (wire.LoginStatus, error) {
	bin, err := s.bin(w)
	if err != nil {
		return wire.LoginStatus{}, err
	}
	s.mu.Lock()
	if f, ok := s.flows[w]; ok && f.live() {
		s.mu.Unlock()
		return wire.LoginStatus{}, wire.ErrLoginBusy
	}
	now := s.cfg.Now()
	recent := s.log[w][:0:0]
	for _, t := range s.log[w] {
		if now.Sub(t) < StartWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= StartsPer {
		s.log[w] = recent
		s.mu.Unlock()
		return wire.LoginStatus{}, wire.ErrLoginRate
	}
	s.log[w] = append(recent, now)

	fctx, cancel := context.WithTimeout(context.Background(), FlowTimeout)
	cmd := exec.CommandContext(fctx, bin, args...)
	cmd.Env = s.cfg.Env()
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = 2 * time.Second
	term, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 4000})
	if err != nil {
		cancel()
		s.mu.Unlock()
		return wire.LoginStatus{}, err
	}
	started := s.ms()
	f := &flow{who: w, cancel: cancel, term: term, done: make(chan struct{}),
		view: wire.LoginFlow{ID: newID(), Provider: w.provider, Profile: w.profile, Mode: mode, State: wire.LoginStarting,
			Input: inputOf(mode), StartedAt: started, ExpiresAt: started + wire.Ms(FlowTimeout.Milliseconds())}}
	s.flows[w] = f
	s.mu.Unlock()

	linked := make(chan struct{})
	go s.run(fctx, f, cmd, linked)

	select {
	case <-linked:
	case <-time.After(StartTimeout):
		s.finish(f, wire.LoginFailed, wire.LoginStartFailed)
		f.cancel()
	case <-ctx.Done():
		s.finish(f, wire.LoginCancelled, "")
		f.cancel()
	}
	return s.status(w), nil
}

func inputOf(m wire.LoginMode) wire.LoginInput {
	if m == wire.LoginDevice {
		return wire.LoginInputNone
	}
	return wire.LoginInputCode
}

// run reads the CLI's output until it has the link, then reads and drops the rest while it waits for the process.
func (s *Service) run(ctx context.Context, f *flow, cmd *exec.Cmd, linked chan struct{}) {
	defer close(f.done)
	defer f.cancel()
	var once sync.Once
	signal := func() { once.Do(func() { close(linked) }) }
	defer signal()

	grabbed := make(chan struct{})
	go func() {
		defer close(grabbed)
		s.grab(f, signal)
	}()
	waitErr := cmd.Wait()
	_ = f.term.Close()
	<-grabbed

	s.mu.Lock()
	defer s.mu.Unlock()
	f.term = nil
	if !f.live() {
		return
	}
	switch {
	case waitErr == nil:
		f.view.State = wire.LoginDone
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		f.view.State = wire.LoginExpired
	case f.view.State == wire.LoginChecking:
		f.view.State, f.view.Reason = wire.LoginFailed, ptr(wire.LoginCodeRejected)
	case f.view.State == wire.LoginAwaiting:
		f.view.State, f.view.Reason = wire.LoginFailed, ptr(wire.LoginRefused)
	default:
		f.view.State, f.view.Reason = wire.LoginFailed, ptr(wire.LoginStartFailed)
	}
	if f.view.State == wire.LoginDone {
		go s.changed(f.who)
	}
}

func ptr[T any](v T) *T { return &v }

// grab reads the terminal: until the link (and, for a device flow, its code) is found, what is read is looked through
// for them and kept at most grabMax long; after that it is only read and dropped, so the CLI never blocks on a full
// terminal. The buffer is gone with the link.
func (s *Service) grab(f *flow, linked func()) {
	var buf []byte
	chunk := make([]byte, 4096)
	found := false
	for {
		n, err := f.term.Read(chunk)
		if n > 0 && !found {
			buf = append(buf, chunk[:n]...)
			if len(buf) > grabMax {
				buf = buf[len(buf)-grabMax:]
			}
			if url, code, ok := s.scan(f, buf); ok {
				found = true
				buf = nil
				s.mu.Lock()
				if f.view.State == wire.LoginStarting {
					f.view.State, f.view.URL, f.view.UserCode = wire.LoginAwaiting, &url, code
				}
				s.mu.Unlock()
				linked()
			}
		}
		if err != nil {
			return
		}
	}
}

// The links and the code a CLI prints. Only a link on the vendor's own hosts is taken, so whatever else the CLI (or a
// proxy page) prints can not turn into a link the page shows.
var (
	ansiRE   = regexp.MustCompile(`\x1b\[[0-9;?<>=]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-Za-z0-9]|\x1b[=>78c]`)
	osc8RE   = regexp.MustCompile(`\x1b\]8;[^;\x07\x1b]*;(https://[^\x07\x1b\s]+)(?:\x07|\x1b\\)`)
	claudeRE = regexp.MustCompile(`https://(?:claude\.com|claude\.ai|platform\.claude\.com|console\.anthropic\.com)/[^\s\x07\x1b"'<>]+`)
	codexRE  = regexp.MustCompile(`https://auth\.openai\.com/[^\s\x07\x1b"'<>]+`)
	codeRE   = regexp.MustCompile(`(?m)^\s*([A-Z0-9]{4}-[A-Z0-9]{4,6})\s*$`)
	// proxyCodeRE is the code as claude-code-proxy prints it, after a label on its line.
	proxyCodeRE = regexp.MustCompile(`(?m)^\s*Enter code:\s*([A-Z0-9]{4}-[A-Z0-9]{4,6})\s*$`)
)

// MaxLinkLen bounds a link: the longest the vendors print is under 1 KB.
const MaxLinkLen = 2048

// Scan looks through the terminal output for the link of a flow and, for a device flow, its one-time code; ok is false
// until all the flow needs is in the output.
func Scan(p wire.Provider, mode wire.LoginMode, out []byte) (url string, code *string, ok bool) {
	switch p {
	case wire.ProviderClaude:
		return scanWith(claudeRE, codeRE, mode == wire.LoginDevice, out)
	case wire.ProviderCodex:
		return scanWith(codexRE, codeRE, mode == wire.LoginDevice, out)
	}
	return "", nil, false
}

// ScanProfile is Scan for the device flow of a profile's proxy.
func ScanProfile(id string, out []byte) (url string, code *string, ok bool) {
	pf, found := profileFlows[id]
	if !found {
		return "", nil, false
	}
	return scanWith(pf.link, pf.codeLine, true, out)
}

func (s *Service) scan(f *flow, out []byte) (string, *string, bool) {
	if f.who.profile != "" {
		return ScanProfile(f.who.profile, out)
	}
	return Scan(f.who.provider, f.view.Mode, out)
}

func scanWith(linkRE, codeRE *regexp.Regexp, device bool, out []byte) (url string, code *string, ok bool) {
	for _, m := range osc8RE.FindAllSubmatch(out, -1) {
		if linkRE.Match(m[1]) && linkRE.FindString(string(m[1])) == string(m[1]) {
			url = string(m[1])
			break
		}
	}
	plain := ansiRE.ReplaceAll(bytes.ReplaceAll(out, []byte("\r"), nil), nil)
	if url == "" {
		url = linkRE.FindString(string(plain))
	}
	if url == "" || len(url) > MaxLinkLen {
		return "", nil, false
	}
	if device {
		after := plain[strings.Index(string(plain), url)+len(url):]
		m := codeRE.FindSubmatch(after)
		if m == nil {
			return "", nil, false
		}
		c := string(m[1])
		code = &c
	}
	return url, code, true
}

// SubmitLoginCode types the code into the CLI's terminal. The code is checked again here for what a terminal would take
// as more than text, and is dropped when it has been written.
func (s *Service) SubmitLoginCode(_ context.Context, p wire.Provider, id, code string) (wire.LoginStatus, error) {
	if err := (wire.LoginCode{ID: id, Code: code}).Validate(); err != nil {
		return wire.LoginStatus{}, wire.ErrLoginInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := who{provider: p}
	f, ok := s.flows[w]
	if !ok || f.view.ID != id || f.view.State != wire.LoginAwaiting || f.view.Input != wire.LoginInputCode || f.term == nil {
		return wire.LoginStatus{}, wire.ErrLoginStale
	}
	_ = f.term.SetWriteDeadline(s.cfg.Now().Add(5 * time.Second))
	if _, err := f.term.Write([]byte(code + "\r")); err != nil {
		return wire.LoginStatus{}, wire.ErrLoginStale
	}
	f.view.State = wire.LoginChecking
	go s.watchCheck(f)
	return s.statusLocked(w), nil
}

// watchCheck gives up on a CLI that does not answer a code.
func (s *Service) watchCheck(f *flow) {
	select {
	case <-f.done:
	case <-time.After(CheckTimeout):
		s.mu.Lock()
		if f.view.State == wire.LoginChecking {
			f.view.State, f.view.Reason = wire.LoginFailed, ptr(wire.LoginTimeout)
		}
		s.mu.Unlock()
		f.cancel()
	}
}

// CancelLogin ends the flow of a CLI that is on.
func (s *Service) CancelLogin(_ context.Context, p wire.Provider) (wire.LoginStatus, error) {
	return s.cancel(who{provider: p}), nil
}

func (s *Service) cancel(w who) wire.LoginStatus {
	s.mu.Lock()
	f, ok := s.flows[w]
	if ok && f.live() {
		f.view.State = wire.LoginCancelled
		f.view.URL, f.view.UserCode = nil, nil
	}
	s.mu.Unlock()
	if ok {
		f.cancel()
		select {
		case <-f.done:
		case <-time.After(3 * time.Second):
		}
	}
	return s.status(w)
}

// finish ends a flow that is on with a state.
func (s *Service) finish(f *flow, st wire.LoginState, reason wire.LoginReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !f.live() {
		return
	}
	f.view.State = st
	if reason != "" {
		f.view.Reason = &reason
	}
}

func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// The terminal made the CLI a session leader: the group takes the helpers it started with it.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

// SaveAPIKey hands an API key to the CLI on its standard input: Codex reads it there and writes it into its own file.
// Claude reads none that way.
func (s *Service) SaveAPIKey(ctx context.Context, p wire.Provider, key string) error {
	if p != wire.ProviderCodex {
		return wire.ErrLoginUnsupported
	}
	if err := (wire.ProfileKeyEdit{Key: key}).Validate(); err != nil {
		return wire.ErrLoginInput
	}
	w := who{provider: p}
	s.mu.Lock()
	if f, ok := s.flows[w]; ok && f.live() {
		s.mu.Unlock()
		return wire.ErrLoginBusy
	}
	s.mu.Unlock()
	if err := s.once(ctx, w, []string{"login", "--with-api-key"}, strings.NewReader(key+"\n")); err != nil {
		if errors.Is(err, wire.ErrLoginNotInstalled) {
			return err
		}
		return wire.ErrLoginInput
	}
	s.changed(w)
	return nil
}

// Logout signs the CLI out; a flow that is on ends first.
func (s *Service) Logout(ctx context.Context, p wire.Provider) error {
	var args []string
	switch p {
	case wire.ProviderClaude:
		args = []string{"auth", "logout"}
	case wire.ProviderCodex:
		args = []string{"logout"}
	default:
		return wire.ErrLoginUnsupported
	}
	return s.logout(ctx, who{provider: p}, args)
}

func (s *Service) logout(ctx context.Context, w who, args []string) error {
	s.cancel(w)
	if err := s.once(ctx, w, args, nil); err != nil {
		return err
	}
	s.changed(w)
	return nil
}

func (s *Service) changed(w who) {
	switch {
	case w.profile != "" && s.cfg.OnProfileChange != nil:
		s.cfg.OnProfileChange(w.profile)
	case w.profile == "" && s.cfg.OnChange != nil:
		s.cfg.OnChange(w.provider)
	}
}

// profileOf is the sign-in of a profile's proxy; ErrProfileNoKey for a profile with no proxy that signs in here.
func (s *Service) profileOf(id string) (who, profileFlow, error) {
	pf, ok := profileFlows[id]
	if !ok || s.cfg.ProfileBin == nil {
		return who{}, profileFlow{}, wire.ErrProfileNoKey
	}
	return who{provider: pf.via, profile: id}, pf, nil
}

func (s *Service) ProfileLogin(_ context.Context, id string) (wire.LoginStatus, error) {
	w, _, err := s.profileOf(id)
	if err != nil {
		return wire.LoginStatus{}, err
	}
	return s.status(w), nil
}

// StartProfileLogin starts the device flow of a profile's proxy, as StartLogin does a CLI's.
func (s *Service) StartProfileLogin(ctx context.Context, id string) (wire.LoginStatus, error) {
	w, pf, err := s.profileOf(id)
	if err != nil {
		return wire.LoginStatus{}, err
	}
	return s.start(ctx, w, wire.LoginDevice, pf.login)
}

func (s *Service) CancelProfileLogin(_ context.Context, id string) (wire.LoginStatus, error) {
	w, _, err := s.profileOf(id)
	if err != nil {
		return wire.LoginStatus{}, err
	}
	return s.cancel(w), nil
}

// ProfileLogout signs a profile's proxy out; a flow that is on ends first.
func (s *Service) ProfileLogout(ctx context.Context, id string) error {
	w, pf, err := s.profileOf(id)
	if err != nil {
		return err
	}
	return s.logout(ctx, w, pf.logout)
}

// once runs a CLI to its end with fixed arguments, its input from in and its output thrown away: the exit status is
// all that is read.
func (s *Service) once(ctx context.Context, w who, args []string, in io.Reader) error {
	bin, err := s.bin(w)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, KeyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = s.cfg.Env()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, io.Discard, io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = 2 * time.Second
	return cmd.Run()
}
