package scriptbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/designkit/kitadd"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == kitadd.SandboxHelperArg {
		if err := kitadd.RunSandboxHelper(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	os.Exit(m.Run())
}

func opts() Options { return Options{Helper: []string{os.Args[0], kitadd.SandboxHelperArg}} }

func need(t *testing.T) {
	if !Available() {
		t.Skip("no node or no Landlock here")
	}
}

func TestScriptTransforms(t *testing.T) {
	need(t)
	answer := map[string]any{"rows": []any{map[string]any{"k": "a", "v": 1.0}, map[string]any{"k": "b", "v": 2.0}}}
	script := `(answer, memo) => ({ items: answer.rows.filter(r => r.v > (memo ? memo.min : 0)).map(r => ({ id: r.k, n: r.v * 10 })), memo: { min: 1, seen: (memo && memo.seen || 0) + 1 } })`
	r, err := Run(context.Background(), opts(), script, answer, json.RawMessage(`{"min":1,"seen":4}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 1 || r.Items[0].(map[string]any)["id"] != "b" || !strings.Contains(string(r.Memo), `"seen":5`) {
		t.Fatalf("%+v %s", r.Items, r.Memo)
	}
}

func TestScriptHasNoNetworkNoTimersNoFiles(t *testing.T) {
	need(t)
	// A listener on this machine: a script that reaches it would make a connection the test sees.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan struct{}, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			got <- struct{}{}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	for name, script := range map[string]string{
		"no timers":  `() => { setTimeout(() => {}, 1); return { items: [] } }`,
		"no fetch":   `() => { fetch("http://127.0.0.1:` + fmt.Sprint(port) + `"); return { items: [] } }`,
		"no require": `() => { require("net"); return { items: [] } }`,
		"no strings": `() => { (0, eval)("1"); return { items: [] } }`,
		"no async":   `async () => ({ items: [] })`,
	} {
		if _, err := Run(context.Background(), opts(), script, nil, nil); err == nil {
			t.Errorf("%s: ran", name)
		}
	}
	// Out of the vm by the host's Function: the process wall still holds — no socket, no file of the home.
	escape := `() => { const p = this.constructor.constructor("return process")(); const net = p.mainModule ? p.mainModule.require("net") : p.binding;
		let out = "";
		try { require("fs") } catch (e) {}
		try { const s = new (p.mainModule.require("net").Socket)(); s.connect(` + fmt.Sprint(port) + `, "127.0.0.1"); out += "socket;" } catch (e) { out += "nosocket;" }
		try { p.mainModule.require("fs").readFileSync("` + os.Getenv("HOME") + `/.bashrc"); out += "home;" } catch (e) { out += "nohome;" }
		return { items: [{ id: out }] } }`
	r, err := Run(context.Background(), opts(), escape, nil, nil)
	t.Logf("escape: %+v %v", r.Items, err)
	if err == nil && (len(r.Items) != 1 || strings.Contains(fmt.Sprint(r.Items[0]), "home;") && !strings.Contains(fmt.Sprint(r.Items[0]), "nohome;")) {
		t.Fatalf("the script read the home: %+v", r.Items)
	}
	select {
	case <-got:
		t.Fatal("the script connected to the network")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestScriptLimits(t *testing.T) {
	need(t)
	began := time.Now()
	if _, err := Run(context.Background(), opts(), `() => { for (;;) {} }`, nil, nil); err == nil || !strings.Contains(err.Error(), "longer than") {
		t.Fatalf("loop: %v", err)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("the loop held %s", took)
	}
	if _, err := Run(context.Background(), opts(), `() => { const a = []; for (;;) a.push(new Array(1e6).fill(7)); }`, nil, nil); err == nil {
		t.Fatal("memory: ran")
	}
	if _, err := Run(context.Background(), opts(), `42`, nil, nil); err == nil || !strings.Contains(err.Error(), "not a function") {
		t.Fatalf("not a function: %v", err)
	}
	if _, err := Run(context.Background(), opts(), strings.Repeat(" ", ScriptMax+1), nil, nil); err == nil {
		t.Fatal("too long")
	}
}

// Even a script that got out of the vm (as if it had) meets the process wall: no socket, no file of the home.
func TestProcessWall(t *testing.T) {
	need(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan struct{}, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			got <- struct{}{}
			c.Close()
		}
	}()
	home, _ := os.UserHomeDir()
	js := fmt.Sprintf(`const out = [];
		try { require("fs").readFileSync(%q); out.push("home") } catch (e) { out.push("nohome") }
		try { require("fs").writeFileSync("/tmp/scriptbox-wall-test", "x"); out.push("write") } catch (e) { out.push("nowrite") }
		const s = require("net").connect(%d, "127.0.0.1");
		s.on("connect", () => { out.push("socket"); done() });
		s.on("error", () => { out.push("nosocket"); done() });
		function done() { process.stdout.write(out.join(",")); process.exit(0) }`, home+"/.bashrc", ln.Addr().(*net.TCPAddr).Port)
	bin, _ := exec.LookPath("node")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, _ := walled(ctx, opts().Helper, bin, 32, js).Output()
	if string(out) != "nohome,nowrite,nosocket" {
		t.Fatalf("under the wall: %q", out)
	}
	select {
	case <-got:
		t.Fatal("connected")
	case <-time.After(200 * time.Millisecond):
	}
}
