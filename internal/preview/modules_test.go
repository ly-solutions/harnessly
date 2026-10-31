package preview

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/preview/devtools"
)

const modBase = "/__tk/p/5173"

func rewriteModules(t *testing.T, src string, chunk int) string {
	t.Helper()
	var buf bytes.Buffer
	m := newModuleRewriter(&buf, modBase)
	b := []byte(src)
	for len(b) > 0 {
		n := min(chunk, len(b))
		if _, err := m.Write(b[:n]); err != nil {
			t.Fatal(err)
		}
		b = b[n:]
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestModuleRewriter(t *testing.T) {
	b := modBase
	cases := []struct{ name, in, want string }{
		{"from", `import{a}from"/src/a.js";`, `import{a}from"` + b + `/src/a.js";`},
		{"from spaced", "import { a } from '/src/a.js'", "import { a } from '" + b + "/src/a.js'"},
		{"export from", `export*from"/x.js"`, `export*from"` + b + `/x.js"`},
		{"side effect", `import"/bootstrap.js";`, `import"` + b + `/bootstrap.js";`},
		{"side effect spaced", `import '/bootstrap.js'`, `import '` + b + `/bootstrap.js'`},
		{"dynamic", `const m=await import("/lazy.js")`, `const m=await import("` + b + `/lazy.js")`},
		{"dynamic spaced", "import(\n '/lazy.js' )", "import(\n '" + b + "/lazy.js' )"},
		{"new URL", `new URL("/assets/a.png",import.meta.url)`, `new URL("` + b + `/assets/a.png",import.meta.url)`},
		{"new URL spaced", `new URL( '/a.png' , import.meta.url )`, `new URL( '` + b + `/a.png' , import.meta.url )`},
		{"vite client", `import "/@vite/client";import RefreshRuntime from "/@react-refresh";`, `import "` + b + `/@vite/client";import RefreshRuntime from "` + b + `/@react-refresh";`},

		{"protocol relative", `import x from"//cdn.example.org/x.js"`, `import x from"//cdn.example.org/x.js"`},
		{"protocol relative bare", `import"//cdn.example.org/x.js"`, `import"//cdn.example.org/x.js"`},
		{"relative", `import x from"./x.js";import y from"../y.js";import"z"`, `import x from"./x.js";import y from"../y.js";import"z"`},
		{"absolute url", `import x from"https://e.org/x.js"`, `import x from"https://e.org/x.js"`},
		{"already prefixed", `import x from"` + b + `/x.js"`, `import x from"` + b + `/x.js"`},
		{"new URL without meta", `new URL("/x", location.href)`, `new URL("/x", location.href)`},
		{"new URL one arg", `new URL("/x")`, `new URL("/x")`},
		{"inside string", `const s="import '/x'";const t='from "/y"'`, `const s="import '/x'";const t='from "/y"'`},
		{"inside template", "const s=`import \"/x\" ${a} from '/y'`", "const s=`import \"/x\" ${a} from '/y'`"},
		{"template code", "const s=`${import(\"/x.js\")}`", "const s=`${import(\"" + b + "/x.js\")}`"},
		{"inside comments", "// import '/x'\n/* from \"/y\" */ import '/z'", "// import '/x'\n/* from \"/y\" */ import '" + b + "/z'"},
		{"regex", `const r=/import '/;import"/ok"`, `const r=/import '/;import"` + b + `/ok"`},
		{"regex class", `const r=/[/']/;import"/ok"`, `const r=/[/']/;import"` + b + `/ok"`},
		{"escaped quote", `const s="a\" import '/x'";import"/ok"`, `const s="a\" import '/x'";import"` + b + `/ok"`},
		{"property", `o.from("/x");o.import("/y");x.from"/z"`, `o.from("/x");o.import("/y");x.from"/z"`},
		{"longer words", `important("/x");fromage"/y";renew URL("/z",import.meta.url)`, `important("/x");fromage"/y";renew URL("/z",import.meta.url)`},
		{"import meta", `import.meta.hot;import.meta.url`, `import.meta.url`[:0] + `import.meta.hot;import.meta.url`},
		{"division", `a=b/c;import"/ok";d=e/f`, `a=b/c;import"` + b + `/ok";d=e/f`},
		{"multibyte", `const s="привет";import"/ок.js"`, `const s="привет";import"` + b + `/ок.js"`},
	}
	for _, c := range cases {
		for _, chunk := range []int{1, 2, 3, 7, 1 << 20} {
			if got := rewriteModules(t, c.in, chunk); got != c.want {
				t.Errorf("%s (chunk %d):\n got %q\nwant %q", c.name, chunk, got, c.want)
			}
		}
	}
}

func TestModuleRewriterLongCandidate(t *testing.T) {
	long := `new URL("/` + strings.Repeat("a", 3*maxHold) + `", location.href);import"/ok"`
	want := strings.Replace(long, `import"/ok"`, `import"`+modBase+`/ok"`, 1)
	for _, chunk := range []int{1, 500, 1 << 20} {
		if got := rewriteModules(t, long, chunk); got != want {
			t.Errorf("chunk %d: output differs (len %d, want %d)", chunk, len(got), len(want))
		}
	}
}

const viteIndex = `<!doctype html><html><body><script type="module">import "/@vite/client"</script><script type="module" src="/main.js"></script></body></html>`
const viteMain = `import"/bootstrap.js";import{x}from"/src/x.js";const u=new URL("/logo.svg",import.meta.url);fetch("http://localhost:$PORT/api")`

// A Vite-like remote at a neighbour port: its module imports are prefixed on the neighbour route only.
func TestProxyNeighbourModules(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/main.js":
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = io.WriteString(w, viteMain)
		case "/main.js.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, viteMain)
		case "/main.css":
			w.Header().Set("Content-Type", "text/css")
			_, _ = io.WriteString(w, `@import "/theme.css";`)
		case "/main.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"s":"import \"/x\""}`)
		case "/gz.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			_, _ = io.WriteString(zw, viteMain)
			_ = zw.Close()
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, viteIndex)
		default:
			w.WriteHeader(404)
		}
	})
	h := newModulesHarness(t, handler)

	prefix := "/__tk/p/" + h.neighbour
	cases := []struct {
		name, path string
		wantHas    []string
		wantNot    []string
	}{
		{"js", prefix + "/main.js", []string{`import"` + prefix + `/bootstrap.js"`, `from"` + prefix + `/src/x.js"`, `new URL("` + prefix + `/logo.svg",import.meta.url)`}, nil},
		{"gzip js", prefix + "/gz.js", []string{`import"` + prefix + `/bootstrap.js"`}, nil},
		{"plain text", prefix + "/main.js.txt", []string{`import"/bootstrap.js"`}, []string{prefix + `/bootstrap.js`}},
		{"css", prefix + "/main.css", []string{`@import "/theme.css"`}, []string{prefix + "/theme.css"}},
		{"json", prefix + "/main.json", []string{`import \"/x\"`}, []string{prefix + "/x"}},
		{"html", prefix + "/", []string{`import "/@vite/client"`, `src="/main.js"`}, []string{prefix + "/@vite"}},
		{"main port", "/main.js", []string{`import"/bootstrap.js"`, `from"/src/x.js"`, `new URL("/logo.svg",import.meta.url)`}, []string{prefix + "/bootstrap.js"}},
	}
	for _, c := range cases {
		body := h.get(c.path)
		for _, s := range c.wantHas {
			if !strings.Contains(body, s) {
				t.Errorf("%s: %q not in %q", c.name, s, body)
			}
		}
		for _, s := range c.wantNot {
			if strings.Contains(body, s) {
				t.Errorf("%s: %q must not be in %q", c.name, s, body)
			}
		}
	}
}

type modulesHarness struct {
	t         *testing.T
	runtime   *httptest.Server
	p         *Proxy
	secret    []byte
	claim     []byte
	neighbour string
}

// newModulesHarness: the same handler served as the main port and as a neighbour of the claim.
func newModulesHarness(t *testing.T, handler http.Handler) *modulesHarness {
	t.Helper()
	mainSrv := httptest.NewServer(handler)
	t.Cleanup(mainSrv.Close)
	neighbourSrv := httptest.NewServer(handler)
	t.Cleanup(neighbourSrv.Close)
	m, n := strconv.Itoa(portOfURL(mainSrv.URL)), strconv.Itoa(portOfURL(neighbourSrv.URL))
	secret := []byte("modules-secret-0123456789abcdef")
	p := New(Config{
		Path: "/agents/api/platform/preview/proxy", ClaimHeader: "x-acme-preview", Audience: "acme-preview-proxy", Secret: secret,
		PrivateHeaderPrefix: "x-acme-", Forbidden: func() map[int]bool { return map[int]bool{} }, Stands: testStands, Store: devtools.NewStore(),
		Cache:        NewAssetCache(t.TempDir(), DefaultMaxTotalBytes, DefaultMaxFileBytes),
		Bridge:       func() ([]byte, bool) { return []byte("bridge();"), true },
		Resolve:      func(string) Resolved { return Resolved{Address: "127.0.0.1", Family: 4} },
		UpstreamHost: "127.0.0.1",
	})
	claim, _ := json.Marshal(map[string]any{
		"sub": "alice", "target": map[string]any{"kind": "port", "port": json.Number(m)}, "slotOrigin": "https://ai-alice-1.example.org",
		"portSlots": map[string]string{m: "https://ai-alice-1.example.org", n: "https://ai-alice-1.example.org/__tk/p/" + n},
	})
	rt := httptest.NewServer(p)
	t.Cleanup(rt.Close)
	return &modulesHarness{t: t, runtime: rt, p: p, secret: secret, claim: claim, neighbour: n}
}

func (h *modulesHarness) get(path string) string {
	h.t.Helper()
	req, _ := http.NewRequest("GET", h.runtime.URL+h.p.cfg.Path+strings.ReplaceAll(path, "$PORT", ""), nil)
	req.Header.Set("x-acme-preview", signClaim(h.secret, h.claim))
	res, err := (&http.Transport{DisableCompression: true}).RoundTrip(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			h.t.Fatal(err)
		}
		body, _ = io.ReadAll(zr)
	}
	return string(body)
}
