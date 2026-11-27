package obsidian

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/connectors/webhook"
)

const secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var clock = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

func instance(t *testing.T) connectors.Instance {
	t.Helper()
	webhook.Base = func() string { return "https://hub.example.com/api/platform/automations/hook/m/mach1" }
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = time.Now })
	return connectors.Instance{ID: "cx0123456789ab", Name: "Obsidian", Dir: t.TempDir(),
		Secret: func(string) (string, error) { return secret, nil }}
}

type part struct {
	Sync  string   `json:"sync"`
	Part  int      `json:"part"`
	Parts int      `json:"parts"`
	Paths []string `json:"paths"`
}

type sent struct {
	V        int      `json:"v"`
	ID       string   `json:"id"`
	At       int64    `json:"at"`
	Vault    string   `json:"vault"`
	Device   string   `json:"device"`
	Plugin   string   `json:"plugin"`
	Upsert   []upsert `json:"upsert,omitempty"`
	Delete   []string `json:"delete,omitempty"`
	Manifest *part    `json:"manifest,omitempty"`
}

func batchOf(id string, up []upsert, del []string, m *part) sent {
	return sent{V: 1, ID: id, At: clock.UnixMilli(), Vault: "Work", Device: "MacBook", Plugin: "1.0.0", Upsert: up, Delete: del, Manifest: m}
}

func push(t *testing.T, in connectors.Instance, b sent) ([]connectors.Event, error) {
	t.Helper()
	raw, _ := json.Marshal(b)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(raw)
	h := http.Header{}
	h.Set("X-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
	return vaultConn{}.Receive(context.Background(), in, connectors.Incoming{Header: h, Body: raw})
}

func read(t *testing.T, in connectors.Instance, tool string, args map[string]string) connectors.Result {
	t.Helper()
	r, err := vaultConn{}.Read(context.Background(), in, tool, args)
	if err != nil {
		t.Fatalf("%s %v: %v", tool, args, err)
	}
	return r
}

func n(p, text string) upsert {
	return upsert{Path: p, Text: text, MTime: clock.UnixMilli(), Title: strings.TrimSuffix(filepath.Base(p), ".md")}
}

// pair is the first full sync: two notes and the whole list; it only learns.
func pair(t *testing.T, in connectors.Instance) {
	t.Helper()
	evs, err := push(t, in, batchOf("b1", []upsert{
		n("Projects/Alpha.md", "# Alpha\nlaunch on friday\n- [ ] call Ann 📅 2026-11-20\n"),
		{Path: "Daily/2026-11-26.md", Text: "met [[Alpha]]", MTime: clock.UnixMilli(), Tags: []string{"#daily"}, Links: []string{"Projects/Alpha.md"}},
	}, nil, &part{Sync: "s1", Part: 0, Parts: 1, Paths: []string{"Projects/Alpha.md", "Daily/2026-11-26.md"}}))
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("the first sync only learns, got %+v", evs)
	}
}

func TestSignatureIsRequired(t *testing.T) {
	in := instance(t)
	raw, _ := json.Marshal(batchOf("b1", []upsert{n("A.md", "a")}, nil, nil))
	for _, sig := range []string{"", "sha256=00", "sha256=" + strings.Repeat("ab", 32)} {
		h := http.Header{}
		h.Set("X-Signature-256", sig)
		_, err := vaultConn{}.Receive(context.Background(), in, connectors.Incoming{Header: h, Body: raw})
		if !errors.Is(err, connectors.ErrUnauthorized) {
			t.Fatalf("signature %q: %v", sig, err)
		}
	}
	// A secret in the address is not how the plugin talks: refused even when the body is signed.
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(raw)
	h := http.Header{}
	h.Set("X-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
	if _, err := (vaultConn{}).Receive(context.Background(), in, connectors.Incoming{Header: h, Body: raw, Path: secret}); !errors.Is(err, connectors.ErrUnauthorized) {
		t.Fatalf("path secret: %v", err)
	}
}

func TestStaleBatchAndRepeat(t *testing.T) {
	in := instance(t)
	old := batchOf("b1", []upsert{n("A.md", "a")}, nil, nil)
	old.At = clock.Add(-time.Hour).UnixMilli()
	if _, err := push(t, in, old); !errors.Is(err, connectors.ErrUnauthorized) {
		t.Fatalf("stale: %v", err)
	}
	b := batchOf("b2", []upsert{n("A.md", "a")}, nil, nil)
	if _, err := push(t, in, b); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := push(t, in, b); !errors.Is(err, connectors.ErrDuplicate) {
		t.Fatalf("repeat: %v", err)
	}
}

func TestBadPathsAreDropped(t *testing.T) {
	in := instance(t)
	bad := []string{"../etc/passwd.md", "/abs.md", "a/../b.md", "a//b.md", "./a.md", "a\\b.md", "x.png", "a\x00.md", "", "dir/"}
	var up []upsert
	for _, p := range bad {
		up = append(up, n(p, "secret"))
	}
	up = append(up, n("ok/Fine.md", "fine"))
	if _, err := push(t, in, batchOf("b1", up, nil, nil)); err != nil {
		t.Fatalf("push: %v", err)
	}
	r := read(t, in, "notes", nil)
	if !strings.Contains(r.Data, "ok/Fine.md") || strings.Count(r.Data, "\n") != 1 {
		t.Fatalf("notes: %q", r.Data)
	}
	// Nothing was written outside the connection's folder.
	if _, err := os.Stat(filepath.Join(filepath.Dir(in.Dir), "etc")); err == nil {
		t.Fatal("wrote outside")
	}
}

func TestFirstSyncLearnsThenEvents(t *testing.T) {
	in := instance(t)
	pair(t, in)

	evs, err := push(t, in, batchOf("b2", []upsert{
		n("Projects/Beta.md", "# Beta"),
		n("Projects/Alpha.md", "# Alpha\nlaunch on monday\n- [ ] call Ann 📅 2026-11-20\n- [ ] send deck 📅 2026-12-01\n"),
	}, nil, nil))
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	types := map[string]int{}
	for _, e := range evs {
		types[e.Type]++
		if strings.Contains(e.Title, "launch") || strings.Contains(e.Title, "deck") {
			t.Fatalf("an event carries the text: %+v", e)
		}
	}
	if types[EventCreated] != 1 || types[EventChanged] != 1 || types[EventTask] != 1 {
		t.Fatalf("events: %+v", evs)
	}
	for _, e := range evs {
		if e.Type == EventTask && (e.Facts["due"] != "2026-12-01" || e.Facts["line"] != "4") {
			t.Fatalf("task: %+v", e)
		}
	}
	// A second save within the quiet time is no new note.changed.
	evs, _ = push(t, in, batchOf("b3", []upsert{n("Projects/Alpha.md", "# Alpha\nlaunch on tuesday\n")}, nil, nil))
	if len(evs) != 0 {
		t.Fatalf("quiet: %+v", evs)
	}
}

func TestManifestRemovesWhatIsNoLongerShared(t *testing.T) {
	in := instance(t)
	pair(t, in)
	// The person stopped sharing Projects/: the next full sync lists only the daily note, in two parts.
	if _, err := push(t, in, batchOf("b2", nil, nil, &part{Sync: "s2", Part: 1, Parts: 2, Paths: nil})); err != nil {
		t.Fatal(err)
	}
	if r := read(t, in, "notes", nil); !strings.Contains(r.Data, "Projects/Alpha.md") {
		t.Fatalf("removed before the list was whole: %q", r.Data)
	}
	evs, err := push(t, in, batchOf("b3", nil, nil, &part{Sync: "s2", Part: 0, Parts: 2, Paths: []string{"Daily/2026-11-26.md"}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Facts["change"] != "removed" || evs[0].Facts["path"] != "Projects/Alpha.md" {
		t.Fatalf("removed event: %+v", evs)
	}
	if r := read(t, in, "notes", nil); strings.Contains(r.Data, "Alpha") {
		t.Fatalf("still there: %q", r.Data)
	}
	files, _ := os.ReadDir(filepath.Join(in.Dir, "notes"))
	if len(files) != 1 {
		t.Fatalf("note files left: %d", len(files))
	}
	if r := read(t, in, "changes", map[string]string{"since": "1h"}); !strings.Contains(r.Data, "removed   Projects/Alpha.md") {
		t.Fatalf("changes: %q", r.Data)
	}
}

func TestDeleteAndReadTools(t *testing.T) {
	in := instance(t)
	pair(t, in)
	r := read(t, in, "note", map[string]string{"path": "Projects/Alpha"})
	if !strings.Contains(r.Data, "launch on friday") || !strings.Contains(r.Data, "linked from: Daily/2026-11-26.md") || r.From != "obsidian: Work" {
		t.Fatalf("note: %+v", r)
	}
	if r := read(t, in, "search", map[string]string{"query": "LAUNCH friday"}); !strings.Contains(r.Data, "Projects/Alpha.md:2:") {
		t.Fatalf("search: %+v", r)
	}
	if r := read(t, in, "tags", nil); !strings.Contains(r.Data, "#daily  1") {
		t.Fatalf("tags: %+v", r)
	}
	if r := read(t, in, "notes", map[string]string{"tag": "daily"}); strings.Contains(r.Data, "Alpha") || !strings.Contains(r.Data, "Daily/") {
		t.Fatalf("notes by tag: %+v", r)
	}
	evs, err := push(t, in, batchOf("b2", nil, []string{"Projects/Alpha.md"}, nil))
	if err != nil || len(evs) != 1 {
		t.Fatalf("delete: %v %+v", err, evs)
	}
	if _, err := (vaultConn{}).Read(context.Background(), in, "note", map[string]string{"path": "Projects/Alpha.md"}); !errors.Is(err, connectors.ErrNotFound) {
		t.Fatalf("read a removed note: %v", err)
	}
}

func TestBulkIsOneEvent(t *testing.T) {
	in := instance(t)
	pair(t, in)
	var up []upsert
	for i := range bulk + 5 {
		up = append(up, n("Bulk/"+strings.Repeat("x", 1+i%7)+string(rune('a'+i%26))+time.Duration(i).String()+".md", "x"))
	}
	evs, err := push(t, in, batchOf("b2", up, nil, nil))
	if err != nil || len(evs) != 1 || evs[0].Facts["change"] != "bulk" {
		t.Fatalf("bulk: %v %+v", err, evs)
	}
}

func TestPairingCode(t *testing.T) {
	in := instance(t)
	shown, err := vaultConn{}.Reveal(in)
	if err != nil || len(shown) != 2 || !strings.HasPrefix(shown[0].Value, "obsidian://harnessly?pair=hly1.") {
		t.Fatalf("reveal: %v %+v", err, shown)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(shown[1].Value, "hly1."))
	if err != nil {
		t.Fatal(err)
	}
	var p pairing
	_ = json.Unmarshal(raw, &p)
	if p.URL != "https://hub.example.com/api/platform/automations/hook/m/mach1/cx0123456789ab" || p.Secret != secret || p.Plugin != "https://hub.example.com/dl/obsidian" {
		t.Fatalf("pairing: %+v", p)
	}
	step, ok := vaultConn{}.Next(context.Background(), in)
	if !ok || !strings.HasPrefix(step.Image, "data:image/png;base64,") || !strings.Contains(step.Text.En, "harnessly-obsidian.zip") {
		t.Fatalf("next: %v %+v", ok, step)
	}
	pair(t, in)
	if _, ok := (vaultConn{}).Next(context.Background(), in); ok {
		t.Fatal("next after pairing")
	}
	acc, err := vaultConn{}.Check(context.Background(), in)
	if err != nil || acc.Label != "Obsidian: Work (MacBook) · 2" {
		t.Fatalf("check: %v %+v", err, acc)
	}
}

// TestPluginEndToEnd runs the Obsidian plugin itself (clients/obsidian/main.js, in node with a stand-in for Obsidian)
// against this connector behind an HTTP server: the pairing code, the signature, the batches, the folder, tag and
// frontmatter choices and the removal of what stops being shared.
func TestPluginEndToEnd(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node")
	}
	in := instance(t)
	// Set before the server starts: its handler reads the instance, and the race detector sees no order through node.
	in.ID = "cx1"
	now = time.Now
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, err := vaultConn{}.Receive(r.Context(), in, connectors.Incoming{Header: r.Header, Body: body})
		switch {
		case errors.Is(err, connectors.ErrUnauthorized):
			w.WriteHeader(http.StatusUnauthorized)
		case err != nil:
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer srv.Close()
	// The plugin takes http only for localhost: the test server is 127.0.0.1.
	webhook.Base = func() string { return srv.URL + "/m" }
	c, err := code(in)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, filepath.Join("..", "..", "..", "clients", "obsidian", "testdata", "run.js"), c)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		`paired []`,
		`projects ["Projects/alpha.md","Projects/beta.md"]`,
		`vault ["Inbox.md","Projects/alpha.md","Projects/beta.md"]`,
		`hide-beta ["Inbox.md","Projects/alpha.md"]`,
		`edit ["Inbox.md","Projects/alpha.md"]`,
		`#private`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plugin output lacks %q:\n%s", want, got)
		}
	}
	st := store{dir: in.Dir}
	ix, err := st.load()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for p := range ix.Notes {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	if strings.Join(paths, ",") != "Inbox.md,Projects/alpha.md" || ix.Vault != "Work" || ix.Device != "MacBook" || !ix.Learned {
		t.Fatalf("machine has %v (%+v)", paths, ix)
	}
	if a := ix.Notes["Projects/alpha.md"]; strings.Join(a.Links, ",") != "" || a.Title != "Alpha" || strings.Join(a.Tags, ",") != "project/alpha" {
		t.Fatalf("alpha: %+v", a)
	}
	if text, _ := st.readNote("Inbox.md"); text != "inbox, changed" {
		t.Fatalf("inbox: %q", text)
	}
}
