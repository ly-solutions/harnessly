package api_test

import (
	"bufio"
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// a guest link /s/<token> never reaches the owner's machine. A guest is served from the hub's store, and when
// the store cannot serve it the guest is told why: the project is not published yet, the link is outdated, the store
// is off or did not answer. Every guest path is read with the machine ONLINE and ready to answer (its runtime does
// hold the project), and the test fails on any call the hub makes to it.

// machineCalls are the machine's requests that read its runtime: a token for a guest, a runtime session, a tunnel.
func machineCalls(a *hubtest.Agent) int {
	n := 0
	for _, op := range []string{hubproto.OpDesignPublicToken, hubproto.OpTunnelOpen, hubproto.OpRuntimeSess} {
		n += len(a.Calls(op))
	}
	return n
}

// guestPaths are every section of a guest link: the page, the index, a file, the app's files, a media file, the
// version, the events, the comments and the seen marks.
func guestPaths(path string) []struct{ method, path, body string } {
	return []struct{ method, path, body string }{
		{"GET", path + "/", ""},
		{"GET", path + "/index.json", ""},
		{"GET", path + "/f/Main.dc.html", ""},
		{"GET", path + "/app/index.html", ""},
		{"GET", path + "/app/assets/x.js", ""},
		{"GET", path + "/m/deck.pdf", ""},
		{"GET", path + "/version", ""},
		{"GET", path + "/comments", ""},
		{"POST", path + "/comments", `{"name":"G","text":"t","board":"Main.dc.html","guestId":"guest-1"}`},
		{"POST", path + "/seen", `{}`},
	}
}

func TestDesignGuestNeverReachesTheOwnersMachine(t *testing.T) {
	s := hubtest.New(t, hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost))
	if s.Hub.Store == nil {
		t.Skip("the hub runs without a store")
	}
	owner := s.SignIn("owner@example.com")
	space, _ := owner.NewSpace("Studio")
	a, _ := designMachine(t, s, owner, space)
	created(a, "shop", "owner@example.com")
	s.Eventually("the project recorded", func() bool { r, ok := snapshotFor(a, "owner@example.com", "shop"); return ok && r == "owner" })
	base := "/m/" + a.MachineID + "/api/platform/design/access"
	link := func(body map[string]any) (id, path string) {
		t.Helper()
		r := owner.Do("POST", base+"/shop/shares", body)
		if r.Code != 201 {
			t.Fatalf("create link %v: %s", body, r)
		}
		var l struct{ ID, URL string }
		r.JSON(&l)
		return l.ID, strings.TrimPrefix(l.URL, "http://"+shareHost)
	}
	guest := s.Browser()
	do := func(method, p, body string) hubtest.Resp {
		t.Helper()
		if method == "POST" {
			return guest.DoAt(s.URL, shareHost, method, p, body, "Content-Type", "application/json")
		}
		return guest.DoAt(s.URL, shareHost, method, p, nil)
	}
	pinned := ""
	var published int64
	head := func(string) int64 { return published }
	// firstEvent is the revision a live link's events say at once (the stream is closed after it).
	firstEvent := func(path string) int64 {
		t.Helper()
		req, _ := http.NewRequest("GET", s.URL+path+"/events", nil)
		req.Host = shareHost
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		res, err := http.DefaultClient.Do(req.WithContext(ctx))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		lines := bufio.NewScanner(res.Body)
		for lines.Scan() {
			if v, ok := strings.CutPrefix(lines.Text(), "data: "); ok {
				n, _ := strconv.ParseInt(v, 10, 64)
				return n
			}
		}
		return 0
	}
	// readAll opens every path of the link and says what each answered; the machine is asked nothing meanwhile.
	readAll := func(name, path string) map[string]int {
		t.Helper()
		before := machineCalls(a)
		codes := map[string]int{}
		for _, c := range guestPaths(path) {
			r := do(c.method, c.path, c.body)
			if strings.Contains(string(r.Body), "Машина владельца недоступна") || r.Code == http.StatusBadGateway {
				t.Errorf("%s: %s %s answers «the machine is unavailable»: %s", name, c.method, c.path, r)
			}
			codes[c.method+" "+strings.TrimPrefix(c.path, path)] = r.Code
		}
		if path != pinned && head(path) != 0 {
			if got := firstEvent(path); got != head(path) {
				t.Errorf("%s: first event %d, want %d", name, got, head(path))
			}
		}
		if n := machineCalls(a) - before; n != 0 {
			t.Fatalf("%s: a guest reached the owner's machine %d times", name, n)
		}
		return codes
	}

	// 1. The project is not in the store, the machine holds it and is online: «not published», 404, no machine.
	_, live := link(map[string]any{"rights": "comment"})
	if r := owner.Do("POST", base+"/shop/shares", map[string]any{"rights": "view", "version": "pinned"}); r.Code != http.StatusConflict || !strings.Contains(r.String(), "not_published") {
		t.Fatalf("a pinned link to a project the store has nothing of: %s", r)
	}
	for _, p := range []string{"/index.json", "/f/Main.dc.html", "/version", "/"} {
		r := do("GET", live+p, "")
		if r.Code != 404 || !strings.Contains(string(r.Body), "ещё не опубликован") {
			t.Fatalf("an unpublished project, %s: %s", p, r)
		}
	}
	readAll("not published", live)

	// 2. Published: the same paths are served from the store, the machine still asked nothing.
	published = putStore(t, s, space, "shop", shopFiles)
	var pinnedID string
	pinnedID, pinned = link(map[string]any{"rights": "view", "version": "pinned"})
	for _, path := range []string{live, pinned} {
		if r := do("GET", path+"/index.json", ""); r.Code != 200 || !strings.Contains(string(r.Body), "Main.dc.html") {
			t.Fatalf("index from the store: %s", r)
		}
		if r := do("GET", path+"/f/Main.dc.html", ""); r.Code != 200 || !strings.Contains(string(r.Body), "shop main") {
			t.Fatalf("page from the store: %s", r)
		}
		if r := do("GET", path+"/version", ""); r.Code != 200 {
			t.Fatalf("version: %s", r)
		}
		readAll("published "+path, path)
	}
	if r := do("GET", pinned+"/events", ""); r.Code != http.StatusNoContent {
		t.Fatalf("a pinned link's events: %s", r)
	}

	// 3. A link pinned to a version of git, made before the store: outdated, 410, not read from the machine.
	oldRev := strings.Repeat("ab", 20)
	if _, err := s.DB.S().Q().Exec(context.Background(), `UPDATE design_share_links SET rev = ? WHERE id = ?`, oldRev, pinnedID); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/index.json", "/f/Main.dc.html", "/version", "/"} {
		r := do("GET", pinned+p, "")
		if r.Code != http.StatusGone || !strings.Contains(string(r.Body), "устарела") {
			t.Fatalf("a link pinned to git, %s: %s", p, r)
		}
	}
	readAll("pinned to git", pinned)

	// 4. The machine goes off: nothing changes for the guest.
	a.Close()
	s.Eventually("the machine offline", func() bool { return s.Hub.Registry.Get(a.MachineID) == nil })
	if r := do("GET", live+"/f/Main.dc.html", ""); r.Code != 200 || !strings.Contains(string(r.Body), "shop main") {
		t.Fatalf("the live link with the machine off: %s", r)
	}
}

// a hub without a store has nothing to serve a guest from: the guest page says so (503) and does not read
// the machine, which is online and holds the project.
func TestDesignGuestOnAHubWithoutAStoreIsToldSo(t *testing.T) {
	s := hubtest.New(t, hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost), hubtest.Env("HUB_STORE", "off"))
	owner := s.SignIn("owner@example.com")
	space, _ := owner.NewSpace("Studio")
	a, _ := designMachine(t, s, owner, space)
	created(a, "shop", "owner@example.com")
	s.Eventually("the project recorded", func() bool { r, ok := snapshotFor(a, "owner@example.com", "shop"); return ok && r == "owner" })
	r := owner.Do("POST", "/m/"+a.MachineID+"/api/platform/design/access/shop/shares", map[string]any{"rights": "view"})
	if r.Code != 201 {
		t.Fatalf("create link: %s", r)
	}
	var l struct{ URL string }
	r.JSON(&l)
	path := strings.TrimPrefix(l.URL, "http://"+shareHost)
	before := machineCalls(a)
	guest := s.Browser()
	for _, p := range []string{"/", "/index.json", "/f/Main.dc.html", "/version"} {
		r := guest.DoAt(s.URL, shareHost, "GET", path+p, nil)
		if r.Code != http.StatusServiceUnavailable || strings.Contains(string(r.Body), "shop main") {
			t.Fatalf("a guest on a hub without a store, %s: %s", p, r)
		}
	}
	if n := machineCalls(a) - before; n != 0 {
		t.Fatalf("a guest reached the owner's machine %d times", n)
	}
}
