// Package scriptbox runs a trigger's transform script: a JavaScript function `(answer, memo) => ({items,
// memo})` written by an agent or the person, which only reshapes what the runtime fetched. It never fetches anything
// itself. Two walls stand around it:
//
//   - the process: node under Landlock and seccomp (internal/confine, the helper of internal/designkit/kitadd): no
//     sockets at all, nothing writable, only the system's and node's own folders readable, an empty environment, its
//     heap capped (--max-old-space-size), killed after a few seconds whatever it does;
//   - the script: a fresh vm context with no require, no process, no timers, no fetch, no code from strings, and a
//     timeout on its run (200 ms by default).
//
// The answer and the memo go in as JSON on stdin, the result comes back as JSON on stdout.
package scriptbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/confine"
	"github.com/ly-solutions/harnessly/internal/designkit/kitadd"
)

// Limits of one run.
const (
	DefaultTimeout  = 200 * time.Millisecond
	DefaultMemoryMB = 32
	// processLimit is how long the whole process may live: node's start, the run, the answer.
	processLimit = 5 * time.Second
	// inputMax and outputMax cap what goes in and comes out.
	inputMax  = 8 << 20
	outputMax = 4 << 20
	// ScriptMax is the longest script taken.
	ScriptMax = 16 << 10
)

// Options of a run; zero values take the defaults.
type Options struct {
	Timeout  time.Duration
	MemoryMB int
	// Helper is the sandbox helper's command line up to its flags (this binary and kitadd.SandboxHelperArg).
	Helper []string
}

// ErrUnavailable: this machine cannot run a script safely (no node, or no Landlock).
var ErrUnavailable = errors.New("scriptbox: scripts need node and Landlock on this machine")

// Result is what the script gave back.
type Result struct {
	Items []any           `json:"items"`
	Memo  json.RawMessage `json:"memo,omitempty"`
}

// runner is the program node runs: the script in a fresh context, bounded in time, its result as JSON.
const runner = `
'use strict';
const vm = require('vm');
const chunks = [];
process.stdin.on('data', (c) => chunks.push(c));
process.stdin.on('end', () => {
  let out;
  try {
    const inp = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    const ctx = vm.createContext(Object.create(null), { codeGeneration: { strings: false, wasm: false } });
    ctx.__in = JSON.stringify({ answer: inp.answer, memo: inp.memo });
    const code = '(() => { const f = (' + inp.script + '); if (typeof f !== "function") throw new Error("the script is not a function");' +
      ' const i = JSON.parse(__in); const r = f(i.answer, i.memo);' +
      ' if (r && typeof r.then === "function") throw new Error("the script must not be async");' +
      ' return JSON.stringify(r === undefined ? null : r); })()';
    out = { ok: true, result: vm.runInContext(code, ctx, { timeout: inp.timeout, breakOnSigint: false }) };
  } catch (e) {
    const msg = e && e.code === 'ERR_SCRIPT_EXECUTION_TIMEOUT' ? 'timeout' : String(e && e.message || e).slice(0, 300);
    out = { ok: false, error: msg };
  }
  process.stdout.write(JSON.stringify(out), () => process.exit(0));
});
`

// Available says whether a script can run here.
func Available() bool {
	if confine.Available() <= 0 {
		return false
	}
	_, err := exec.LookPath("node")
	return err == nil
}

// systemRO is what node may read: programs, libraries, the few settings it reads. No home, no /proc but its own.
var systemRO = []string{"/usr", "/bin", "/lib", "/lib64", "/lib32", "/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/ld.so.conf.d",
	"/etc/localtime", "/proc/self", "/proc/meminfo", "/proc/cpuinfo", "/sys/devices/system/cpu", "/sys/fs/cgroup", "/dev/urandom"}

func nodeDirs(bin string) []string {
	var out []string
	for _, p := range []string{bin, realPath(bin)} {
		dir := filepath.Dir(p)
		out = append(out, dir)
		if up := filepath.Dir(dir); up != "/" && up != "." {
			out = append(out, up)
		}
	}
	return out
}

func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// Run runs the script once on the answer and the memo.
func Run(ctx context.Context, o Options, script string, answer any, memo json.RawMessage) (Result, error) {
	if len(script) > ScriptMax {
		return Result{}, fmt.Errorf("the script is longer than %d bytes", ScriptMax)
	}
	if !Available() {
		return Result{}, ErrUnavailable
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MemoryMB <= 0 {
		o.MemoryMB = DefaultMemoryMB
	}
	bin, err := exec.LookPath("node")
	if err != nil {
		return Result{}, ErrUnavailable
	}
	helper := o.Helper
	if len(helper) == 0 {
		self, err := os.Executable()
		if err != nil {
			return Result{}, err
		}
		helper = []string{self, kitadd.SandboxHelperArg}
	}
	if len(memo) == 0 {
		memo = json.RawMessage("null")
	}
	in, err := json.Marshal(map[string]any{"script": script, "answer": answer, "memo": memo, "timeout": o.Timeout.Milliseconds()})
	if err != nil {
		return Result{}, err
	}
	if len(in) > inputMax {
		return Result{}, errors.New("the answer is too big for a script")
	}
	ctx, cancel := context.WithTimeout(ctx, processLimit)
	defer cancel()
	cmd := walled(ctx, helper, bin, o.MemoryMB, runner)
	cmd.Stdin = bytes.NewReader(in)
	var out, errb limited
	out.max, errb.max = outputMax, 4<<10
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return Result{}, errors.New("the script ran out of time")
	}
	var res struct {
		OK     bool   `json:"ok"`
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if out.over || json.Unmarshal(out.Bytes(), &res) != nil {
		tail := strings.TrimSpace(errb.String())
		if strings.Contains(tail, "heap") || strings.Contains(tail, "out of memory") {
			return Result{}, fmt.Errorf("the script went over %d MB", o.MemoryMB)
		}
		if runErr != nil {
			return Result{}, fmt.Errorf("the script's process failed: %v: %s", runErr, tail)
		}
		return Result{}, errors.New("the script gave no answer")
	}
	if !res.OK {
		if res.Error == "timeout" {
			return Result{}, fmt.Errorf("the script took longer than %s", o.Timeout)
		}
		return Result{}, fmt.Errorf("the script failed: %s", res.Error)
	}
	var r Result
	if err := json.Unmarshal([]byte(res.Result), &r); err != nil {
		return Result{}, errors.New("the script must return {items, memo}")
	}
	if r.Items == nil {
		r.Items = []any{}
	}
	return r, nil
}

// walled is node running js under the process wall: no sockets, nothing writable, only system and node folders
// readable, an empty environment, a capped heap, no code from strings.
func walled(ctx context.Context, helper []string, bin string, memoryMB int, js string) *exec.Cmd {
	args := append([]string{}, helper[1:]...)
	args = append(args, "--net", "none", "--rw", "/dev/null")
	for _, p := range append(append([]string{}, systemRO...), nodeDirs(bin)...) {
		args = append(args, "--ro", p)
	}
	args = append(args, "--", bin, "--max-old-space-size="+strconv.Itoa(memoryMB), "--max-semi-space-size=1",
		"--disallow-code-generation-from-strings", "-e", js)
	cmd := exec.CommandContext(ctx, helper[0], args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "OPENSSL_CONF=/dev/null"}
	cmd.Dir = "/"
	return cmd
}

// limited keeps at most max bytes and says whether there was more.
type limited struct {
	bytes.Buffer
	max  int
	over bool
}

func (l *limited) Write(p []byte) (int, error) {
	if l.Len()+len(p) > l.max {
		l.over = true
		if room := l.max - l.Len(); room > 0 {
			l.Buffer.Write(p[:room])
		}
		return len(p), nil
	}
	return l.Buffer.Write(p)
}
