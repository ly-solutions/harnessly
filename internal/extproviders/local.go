package extproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// A local model: a server on the person's own machine or network that serves a model with no key and no
// internet: Ollama, LM Studio, llama.cpp's server, vLLM. It is no provider of a kind of its own: it is an account of
// this registry made from the template PresetLocal, which speaks whichever API the server does. Claude Code reaches
// one that serves the Anthropic Messages API straight; any other goes through the runtime's proxy as an
// OpenAI-compatible provider. What this file adds is finding such servers and the rule that their address is a local
// one: a key-less provider must never point at somebody else's host.

// PresetLocal is the template of a local model server (the page offers its own card for it, not a form).
const PresetLocal = "local"

// ReasonNotLocal: the address is not on this machine or a private network; a local model is never fetched from there.
const ReasonNotLocal = "not-local"

// LocalTimeout bounds the probe of one server: a server on this machine answers at once, so a port that does not is
// not worth waiting for.
const LocalTimeout = 1500 * time.Millisecond

// LocalTarget is a place a local server is looked for: its kind and the root of its address.
type LocalTarget struct {
	Kind string
	Name string
	Root string
}

// LocalTargets are the well-known ports of the servers, on the machine's loopback.
var LocalTargets = []LocalTarget{
	{Kind: "ollama", Name: "Ollama", Root: "http://127.0.0.1:11434"},
	{Kind: "lmstudio", Name: "LM Studio", Root: "http://127.0.0.1:1234"},
	{Kind: "llamacpp", Name: "llama.cpp", Root: "http://127.0.0.1:8080"},
	{Kind: "vllm", Name: "vLLM", Root: "http://127.0.0.1:8000"},
}

// LocalServer is a server that answered.
type LocalServer struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	// BaseURL is the address a provider is made with: the root of a server that speaks the Anthropic API, the /v1 of
	// one that speaks only OpenAI's.
	BaseURL string  `json:"baseUrl"`
	API     API     `json:"api"`
	Models  []Model `json:"models"`
	// Context is the context a loaded model runs with (tokens), when the server says; 0: not known. A server may run a
	// model with far less than the model can take (Ollama's default is 4096 without a big GPU).
	Context int `json:"context,omitempty"`
}

// LocalFound is what a discovery found.
type LocalFound struct {
	Servers []LocalServer `json:"servers"`
	// Custom says how the address the person typed went: nil when none was given.
	Custom *LocalCustom `json:"custom,omitempty"`
}

// LocalCustom is the answer for the address the person typed.
type LocalCustom struct {
	Address string `json:"address"`
	Found   bool   `json:"found"`
}

// privateAddr says whether an address is the machine itself or a private network: loopback, link-local, RFC 1918 and
// unique local ones, and the carrier-grade range VPNs such as Tailscale use.
func privateAddr(a netip.Addr) bool {
	a = a.Unmap()
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() {
		return true
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return cgnat.Contains(a)
}

// LocalHost says whether the host of an address may be a local model's: «localhost», an address of a private range, or
// a name that resolves to such addresses only (checked again when it is connected to, see localClient).
func LocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return privateAddr(a)
	}
	if !strings.Contains(host, ".") {
		// A name with no dot is the network's own: a host of the LAN.
		return true
	}
	// A dotted name is local when every address it resolves to is (and it is checked again when it is dialled).
	ctx, cancel := context.WithTimeout(context.Background(), LocalTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	return err == nil && len(addrs) > 0 && !slices.ContainsFunc(addrs, func(a netip.Addr) bool { return !privateAddr(a) })
}

// CleanLocalURL is CleanURL for an address a local model is at: it must be this machine or a private network.
func CleanLocalURL(raw string) (string, error) {
	base, err := CleanURL(raw)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(base)
	if !LocalHost(u.Hostname()) {
		return "", refuse(ReasonNotLocal)
	}
	return base, nil
}

// errNotLocal is what a connection to a public address is refused with.
var errNotLocal = errors.New("extproviders: not a local address")

// localClient is the client a local server is asked with. It follows no redirect, uses no proxy and, whatever name the
// address has, connects only to a private address: a name that points at the internet is refused where it is dialled,
// so it cannot be turned against a public host after the check of the name.
func localClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: LocalTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errNotLocal
			}
			a, err := netip.ParseAddr(host)
			if err != nil || !privateAddr(a) {
				return errNotLocal
			}
			return nil
		},
	}
	return &http.Client{
		Timeout:       LocalTimeout,
		Transport:     &http.Transport{Proxy: nil, DialContext: dialer.DialContext, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Discoverer looks for local servers.
type Discoverer struct {
	// Targets are the places to look; nil is LocalTargets.
	Targets []LocalTarget
	// Client sends the requests; nil is localClient().
	Client *http.Client
}

func (d *Discoverer) targets() []LocalTarget {
	if d.Targets != nil {
		return d.Targets
	}
	return LocalTargets
}

func (d *Discoverer) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return localClient()
}

// Discover asks every target and, when the person typed an address, that one too (it must be a local one). Servers
// that do not answer are left out; the order is the targets' own, the typed address last.
func (d *Discoverer) Discover(ctx context.Context, custom string) (LocalFound, error) {
	targets := slices.Clone(d.targets())
	var typed string
	if custom = strings.TrimSpace(custom); custom != "" {
		base, err := CleanLocalURL(custom)
		if err != nil {
			return LocalFound{}, err
		}
		typed = localRoot(base)
		if !slices.ContainsFunc(targets, func(t LocalTarget) bool { return sameServer(t.Root, typed) }) {
			u, _ := url.Parse(typed)
			targets = append(targets, LocalTarget{Kind: "custom", Name: u.Host, Root: typed})
		}
	}
	found := make([]*LocalServer, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			found[i] = d.probe(ctx, t)
		}()
	}
	wg.Wait()
	out := LocalFound{Servers: []LocalServer{}}
	for _, s := range found {
		if s != nil {
			out.Servers = append(out.Servers, *s)
		}
	}
	if typed != "" {
		out.Custom = &LocalCustom{Address: typed, Found: slices.ContainsFunc(out.Servers, func(s LocalServer) bool {
			return sameServer(s.BaseURL, typed)
		})}
	}
	return out, nil
}

// localRoot is the address with a trailing /v1 taken off: the root of the server.
func localRoot(base string) string { return strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1") }

// probe asks one server for its models, then whether it speaks the Anthropic API; nil when it does not answer as a
// server of models does.
func (d *Discoverer) probe(ctx context.Context, t LocalTarget) *LocalServer {
	ctx, cancel := context.WithTimeout(ctx, 2*LocalTimeout)
	defer cancel()
	status, raw, err := d.do(ctx, http.MethodGet, t.Root+"/v1/models", nil)
	if err != nil || status < 200 || status >= 300 {
		return nil
	}
	models, ok := parseModels(raw)
	if !ok {
		return nil
	}
	s := &LocalServer{Kind: t.Kind, Name: t.Name, BaseURL: t.Root + "/v1", API: APIOpenAI, Models: models}
	// A message to a model that does not exist is answered with the API's own error by a server that speaks it: no
	// model is loaded by the question.
	body, _ := json.Marshal(map[string]any{"model": "harnessly-probe", "max_tokens": 1, "messages": []map[string]string{{"role": "user", "content": "ping"}}})
	h := http.Header{"anthropic-version": {"2023-06-01"}, "Content-Type": {"application/json"}}
	if status, raw, err = d.do(ctx, http.MethodPost, t.Root+"/v1/messages", bytes.NewReader(body), h); err == nil && status != http.StatusUnauthorized && speaksAnthropic(raw) {
		s.BaseURL, s.API = t.Root, APIAnthropic
	}
	d.windows(ctx, t.Root, s)
	return s
}

// LocalWindow is the window a local model is given when the server does not say: Claude Code would assume a million
// tokens for a model it does not know, and never compact a chat that a local model has long lost the start of.
const LocalWindow = 32 << 10

// maxShown bounds the models /api/show is asked about: a server with a long shelf is not asked a hundred times.
const maxShown = 30

// windows asks an Ollama for what it knows of its models' windows: the model's own limit (/api/show) and the context a
// loaded one runs with (/api/ps). The window a chat is told is the context the model runs with, else its limit up to
// LocalWindow. A server that is no Ollama has no such routes and keeps what /v1/models said.
func (d *Discoverer) windows(ctx context.Context, root string, s *LocalServer) {
	status, raw, err := d.do(ctx, http.MethodGet, root+"/api/ps", nil)
	if err != nil || status != http.StatusOK {
		return
	}
	var ps struct {
		Models []struct {
			Name    string `json:"name"`
			Context int    `json:"context_length"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &ps) != nil {
		return
	}
	loaded := map[string]int{}
	for _, m := range ps.Models {
		if m.Context > 0 {
			loaded[m.Name] = m.Context
			if s.Context == 0 || m.Context < s.Context {
				s.Context = m.Context
			}
		}
	}
	for i := range s.Models {
		m := &s.Models[i]
		if m.Window > 0 || i >= maxShown {
			continue
		}
		if n := loaded[m.ID]; n > 0 {
			m.Window = n
			continue
		}
		body, _ := json.Marshal(map[string]string{"model": m.ID})
		status, raw, err := d.do(ctx, http.MethodPost, root+"/api/show", bytes.NewReader(body), http.Header{"Content-Type": {"application/json"}})
		if err != nil || status != http.StatusOK {
			continue
		}
		var show struct {
			Info map[string]any `json:"model_info"`
		}
		if json.Unmarshal(raw, &show) != nil {
			continue
		}
		for k, v := range show.Info {
			if n, ok := v.(float64); ok && n > 0 && strings.HasSuffix(k, ".context_length") {
				m.Window = min(int(n), LocalWindow)
			}
		}
	}
}

func (d *Discoverer) do(ctx context.Context, method, target string, body *bytes.Reader, extra ...http.Header) (int, []byte, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, target, body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, target, nil)
	}
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	for _, h := range extra {
		for k, v := range h {
			req.Header[k] = v
		}
	}
	res, err := d.client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	return res.StatusCode, raw, nil
}

// LocalName is the name an account of a found server is given: «Локальная модель · Ollama».
func LocalName(server string) string {
	return "Локальная модель · " + strings.TrimSpace(server)
}
