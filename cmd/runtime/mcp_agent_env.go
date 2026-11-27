package main

// The agent's environment as the agent gets it: a pane starts `env -i <a few variables> $SHELL -l`
// (internal/tmux), so what an agent's MCP config expands ${VAR} with is what the person's login files export — a
// per-person port of a local bridge, say — not the runtime's own environment. The sync of the agents' MCP servers expands
// with the same: a login shell run the same way, its `env` read once and again when a login file changes.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	agentEnvTimeout = 10 * time.Second
	agentEnvTTL     = 10 * time.Minute
	agentEnvMark    = "\x00__harnessly_env__\x00"
)

// agentEnvKeys are the variables a pane shell keeps (internal/tmux inheritedKeys, the ones a login file may build on).
var agentEnvKeys = []string{"USER", "LOGNAME", "LANG", "TMPDIR", "TZ"}

// loginFiles are what a login shell reads: a change of one reads the environment again.
var loginFiles = []string{".bash_profile", ".bash_login", ".profile", ".bashrc", ".zprofile", ".zshenv", ".zshrc", ".zlogin"}

type agentEnv struct {
	home  string
	shell string
	run   func(ctx context.Context, shell string, env []string) ([]byte, error)

	mu   sync.Mutex
	vals map[string]string
	ok   bool
	sig  string
	at   time.Time
}

func newAgentEnv(home string) *agentEnv {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return &agentEnv{home: home, shell: shell, run: runLoginShell}
}

func runLoginShell(ctx context.Context, shell string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", `printf '\0__harnessly_env__\0'; env -0`)
	cmd.Env = env
	cmd.Stdin = nil
	return cmd.Output()
}

func (e *agentEnv) signature() string {
	var b strings.Builder
	for _, f := range loginFiles {
		if fi, err := os.Stat(filepath.Join(e.home, f)); err == nil {
			b.WriteString(f + fi.ModTime().UTC().Format(time.RFC3339Nano) + ";")
		}
	}
	return b.String()
}

// base is the environment the pane shell starts with.
func (e *agentEnv) base() []string {
	env := []string{"HOME=" + e.home, "SHELL=" + e.shell, "TERM=xterm-256color",
		"PATH=" + filepath.Join(e.home, ".local", "bin") + ":/usr/local/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"}
	for _, k := range agentEnvKeys {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// read is the login shell's environment; ok is false when the shell did not answer.
func (e *agentEnv) read() (map[string]string, bool) {
	sig := e.signature()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.vals != nil && sig == e.sig && time.Since(e.at) < agentEnvTTL {
		return e.vals, e.ok
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentEnvTimeout)
	defer cancel()
	out, err := e.run(ctx, e.shell, e.base())
	vals, ok := parseEnvDump(out)
	e.vals, e.ok, e.sig, e.at = vals, ok && err == nil, sig, time.Now()
	return e.vals, e.ok
}

// parseEnvDump reads `env -0` after the mark (what a login file printed before it is not the environment).
func parseEnvDump(out []byte) (map[string]string, bool) {
	vals := map[string]string{}
	i := bytes.Index(out, []byte(agentEnvMark))
	if i < 0 {
		return vals, false
	}
	for _, kv := range bytes.Split(out[i+len(agentEnvMark):], []byte{0}) {
		k, v, found := strings.Cut(string(kv), "=")
		if found && k != "" {
			vals[k] = v
		}
	}
	return vals, true
}

// Getenv is a variable as the agent has it. When the login shell could not be read, the runtime's own environment
// stands in (better than a default that may point at someone else's process).
func (e *agentEnv) Getenv(k string) (string, bool) {
	vals, ok := e.read()
	if !ok {
		return os.LookupEnv(k)
	}
	v, has := vals[k]
	return v, has
}
