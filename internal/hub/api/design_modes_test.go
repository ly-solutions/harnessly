package api_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/api"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// the sharing of a Sinopia project on a hub in both of its runtime modes. The same scenarios run with the
// machines on the hub's own origin (path mode, /m/<id>/...) and on an origin of their own (subdomain mode, the ticket
// of /api/machines/<id>/open buys a cookie of <id>.<domain>): a grant, a revoke and a team level reach the person's
// library at once, a guest opens a link on the share host and nothing else, an invitation leads into the workspace.

const modesDomain = "m.example.test"

// runtimeOrigin is a person on one machine's pages: in path mode the hub's origin and the person's own cookie, in
// subdomain mode the machine's host after the ticket exchange, the way the real page does it (the cookie carried by hand:
// the hub's origin and a machine's are both 127.0.0.1 here and one jar would mix them).
type runtimeOrigin struct {
	s      *hubtest.Stack
	p      *hubtest.Person
	raw    *hubtest.Person
	id     string
	sub    bool
	host   string
	cookie string
}

func (o *runtimeOrigin) enter() {
	o.s.T.Helper()
	r := o.p.Do("GET", "/api/machines/"+o.id+"/open", nil)
	var out struct{ URL string }
	r.JSON(&out)
	u, err := url.Parse(out.URL)
	if err != nil || r.Code != 200 {
		o.s.T.Fatalf("open %s for %s: %s", o.id, o.p.Email, r)
	}
	if !o.sub {
		if want := "/m/" + o.id + "/"; out.URL != want {
			o.s.T.Fatalf("open = %q, want %q", out.URL, want)
		}
		return
	}
	if !strings.HasPrefix(u.Host, o.id+"."+modesDomain) {
		o.s.T.Fatalf("open = %q, want a host of the machine on %s", out.URL, modesDomain)
	}
	o.host = u.Host
	r = o.raw.DoAt(o.s.URL, o.host, "GET", u.RequestURI(), nil)
	for _, c := range (&http.Response{Header: r.Header}).Cookies() {
		if c.Name == "hl_run" {
			o.cookie = "hl_run=" + c.Value
		}
	}
	if r.Code != 200 || o.cookie == "" {
		o.s.T.Fatalf("ticket exchange of %s on %s: %s %v", o.p.Email, o.host, r, r.Header)
	}
}

// do is a request to a path of the machine, without its /m/<id> prefix.
func (o *runtimeOrigin) do(method, path string, body any) hubtest.Resp {
	o.s.T.Helper()
	full := "/m/" + o.id + path
	if !o.sub {
		return o.p.Do(method, full, body)
	}
	var h []string
	if o.cookie != "" {
		h = []string{"Cookie", o.cookie}
	}
	return o.raw.DoAt(o.s.URL, o.host, method, full, body, h...)
}

func (f *modesFixture) origin(p *hubtest.Person, machineID string) *runtimeOrigin {
	o := &runtimeOrigin{s: f.s, p: p, id: machineID, sub: f.sub}
	o.raw = &hubtest.Person{S: f.s, Client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	o.enter()
	return o
}

type modesFixture struct {
	s        *hubtest.Stack
	sub      bool
	owner    *hubtest.Person
	member   *hubtest.Person
	space    string
	ownerM   *hubtest.Agent
	memberM  *hubtest.Agent
	ownerAt  *runtimeOrigin // the owner on their own machine
	memberAt *runtimeOrigin // the member on their own machine
	memberOn *runtimeOrigin // the member on the owner's machine
}

func TestDesignSharingInBothModes(t *testing.T) {
	t.Run("path", func(t *testing.T) { runDesignSharing(t, false) })
	t.Run("subdomain", func(t *testing.T) { runDesignSharing(t, true) })
}

func runDesignSharing(t *testing.T, sub bool) {
	opts := []hubtest.Option{
		hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost), hubtest.Env("HUB_DESIGN_DEFAULT_TEAM_LEVEL", "none"),
		// What the invitation's navigation needs (TestDesignInviteHubNavigation), in both modes alike.
		hubtest.Env("HUB_ORGS", "on"), hubtest.Env("HUB_ACCESS_ENFORCE", "on"), hubtest.Env("HUB_FEATURES_FORCE", "runtime.fleet"),
		hubtest.WithOptions(func(o *api.Options) { o.Workspace = fstest.MapFS{"index.html": {Data: []byte("workspace")}} }),
	}
	if sub {
		opts = append(opts, hubtest.Env("HUB_RUNTIME_DOMAIN", modesDomain))
	} else {
		// A machine a member connected opens on the hub's own origin only when the owner of its space trusts it.
		opts = append(opts, hubtest.Env("HUB_MACHINE_TRUST", "on"))
	}
	s := hubtest.New(t, opts...)
	owner := s.SignIn("owner@example.com") // the first person of a hub is its administrator
	member := s.SignIn("member@example.com")
	space, _ := owner.NewSpace("Studio")
	owner.Join(space, member)
	ownerM, _ := designMachine(t, s, owner, space)
	memberM, _ := designMachine(t, s, member, space)
	if !sub {
		if r := owner.Do("POST", "/api/machines/"+memberM.MachineID+"/trust", map[string]any{}); r.Code != 200 {
			t.Fatalf("trust the member's machine: %s", r)
		}
	}
	// hub-design-home opens the long poll of the library; hub-design-owner lists a recorded project from the hub's own
	// rows, which the test needs: no store holds the files of the machines here.
	for _, p := range []*hubtest.Person{owner, member} {
		for _, feature := range []string{"hub-design-home", "hub-design-owner"} {
			if r := owner.Do("PUT", "/api/admin/switches", map[string]any{"feature": feature, "person": p.Email, "enabled": true}); r.Code != 204 {
				t.Fatalf("switch %s for %s: %s", feature, p.Email, r)
			}
		}
	}
	f := &modesFixture{s: s, sub: sub, owner: owner, member: member, space: space, ownerM: ownerM, memberM: memberM}
	f.ownerAt = f.origin(owner, ownerM.MachineID)
	f.memberAt = f.origin(member, memberM.MachineID)
	f.memberOn = f.origin(member, ownerM.MachineID)

	created(ownerM, "shop", "owner@example.com")
	s.Eventually("the project recorded", func() bool {
		role, ok := snapshotFor(ownerM, "owner@example.com", "shop")
		return ok && role == "owner"
	})

	f.scenarioClosedProject(t)
	f.scenarioGrantRevoke(t)
	f.scenarioTeamLevel(t)
	f.scenarioGuestLink(t)
	f.scenarioInvite(t)
	if strings.Contains(s.Logs.String(), "level=ERROR") {
		t.Errorf("errors in the log:\n%s", s.Logs.String())
	}
}

const (
	platform = "/api/platform"
	accessOf = platform + "/access/design/shop"
)

// library is the artifacts of the person's GET store-library on the machine, by id.
func library(t *testing.T, o *runtimeOrigin) map[string]string {
	t.Helper()
	r := o.do("GET", platform+"/design/store-library", nil)
	if r.Code != 200 {
		t.Fatalf("store-library of %s: %s", o.p.Email, r)
	}
	var lib struct{ Artifacts []struct{ ID, Role string } }
	r.JSON(&lib)
	out := map[string]string{}
	for _, a := range lib.Artifacts {
		out[a.ID] = a.Role
	}
	return out
}

func roleIn(t *testing.T, o *runtimeOrigin) (string, bool) {
	t.Helper()
	r := o.do("GET", accessOf, nil)
	if r.Code != 200 {
		t.Fatalf("access view of %s: %s", o.p.Email, r)
	}
	var v accessView
	r.JSON(&v)
	return roleOf(v), v.CanManage
}

// rev is the revision of the person's view of the library now.
func rev(t *testing.T, o *runtimeOrigin, since int64) (int64, time.Duration) {
	t.Helper()
	start := time.Now()
	r := o.do("GET", platform+"/design/store-library/changes?since="+strconv.FormatInt(since, 10), nil)
	var out struct{ Rev int64 }
	r.JSON(&out)
	if r.Code != 200 {
		t.Fatalf("store-library/changes since %d: %s", since, r)
	}
	return out.Rev, time.Since(start)
}

// 1. The owner's project is closed to the space: the member's library and the access view do not have it.
func (f *modesFixture) scenarioClosedProject(t *testing.T) {
	t.Run("closed project is not listed", func(t *testing.T) {
		if r := f.ownerAt.do("PUT", accessOf+"/team", map[string]any{"role": nil}); r.Code != 200 {
			t.Fatalf("close the project: %s", r)
		}
		for _, at := range []*runtimeOrigin{f.memberAt, f.memberOn} {
			if role, manage := roleIn(t, at); role != "" || manage {
				t.Fatalf("the access view of a closed project through %s: %q %v", at.id, role, manage)
			}
			if lib := library(t, at); len(lib) != 0 {
				t.Fatalf("the member's library through %s lists a closed project: %v", at.id, lib)
			}
		}
	})
}

// 2. A grant by email reaches the member, and a long poll started before it returns soon after.
func (f *modesFixture) scenarioGrantRevoke(t *testing.T) {
	t.Run("grant and revoke by email", func(t *testing.T) {
		since, _ := rev(t, f.memberAt, 0)
		type polled struct {
			rev int64
			in  time.Duration
		}
		done := make(chan polled, 1)
		go func() {
			r, d := rev(t, f.memberAt, since)
			done <- polled{r, d}
		}()
		time.Sleep(300 * time.Millisecond)
		if r := f.ownerAt.do("PUT", accessOf+"/grants/user/member@example.com", map[string]any{"role": "view", "canShare": false, "ttl": nil}); r.Code != 200 {
			t.Fatalf("grant by email: %s", r)
		}
		select {
		case p := <-done:
			if p.rev <= since || p.in > 8*time.Second {
				t.Fatalf("the long poll started before the grant: rev %d (was %d) after %s", p.rev, since, p.in)
			}
		case <-time.After(12 * time.Second):
			t.Fatalf("the long poll started before the grant did not return in 12 s (the wait ends at 25 s)")
		}
		for _, at := range []*runtimeOrigin{f.memberAt, f.memberOn} {
			if role, manage := roleIn(t, at); role != "view" || manage {
				t.Fatalf("the member's role through %s after the grant: %q %v", at.id, role, manage)
			}
		}
		if lib := library(t, f.memberAt); lib["shop"] != "view" {
			t.Fatalf("the member's library after the grant: %v", lib)
		}

		granted, _ := rev(t, f.memberAt, 0)
		if r := f.ownerAt.do("DELETE", accessOf+"/grants/user/member@example.com", nil); r.Code != 200 {
			t.Fatalf("revoke: %s", r)
		}
		if role, _ := roleIn(t, f.memberAt); role != "" {
			t.Fatalf("the member's role after the revoke: %q", role)
		}
		if lib := library(t, f.memberAt); len(lib) != 0 {
			t.Fatalf("the member's library after the revoke: %v", lib)
		}
		if r, _ := rev(t, f.memberAt, granted); r <= granted {
			t.Fatalf("the revision after the revoke: %d, before %d", r, granted)
		}
	})
}

// 3. The team level opens the project to the space.
func (f *modesFixture) scenarioTeamLevel(t *testing.T) {
	t.Run("team level", func(t *testing.T) {
		if r := f.ownerAt.do("PUT", accessOf+"/team", map[string]any{"role": "view"}); r.Code != 200 {
			t.Fatalf("team view: %s", r)
		}
		if role, _ := roleIn(t, f.memberAt); role != "view" {
			t.Fatalf("the member's role at team level view: %q", role)
		}
		if lib := library(t, f.memberAt); lib["shop"] != "view" {
			t.Fatalf("the member's library at team level view: %v", lib)
		}
		if r := f.ownerAt.do("PUT", accessOf+"/team", map[string]any{"role": nil}); r.Code != 200 {
			t.Fatalf("close again: %s", r)
		}
		if role, _ := roleIn(t, f.memberAt); role != "" {
			t.Fatalf("the member's role after closing: %q", role)
		}
	})
}

// 4. A guest link opens the project on the share host and nothing of the hub.
func (f *modesFixture) scenarioGuestLink(t *testing.T) {
	t.Run("guest link", func(t *testing.T) {
		// a guest is served from the hub's store, where the machine's sync has put the project.
		putStore(t, f.s, f.space, "shop", shopFiles)
		var made struct{ URL string }
		r := f.ownerAt.do("POST", accessOf+"/links", map[string]any{"forWhom": "Клиент", "rights": "view", "ttl": nil, "options": nil, "version": "latest", "pages": nil})
		if r.Code != http.StatusCreated {
			t.Fatalf("make a link: %s", r)
		}
		r.JSON(&made)
		if !strings.HasPrefix(made.URL, "http://"+shareHost+"/s/") {
			t.Fatalf("link: %s", r)
		}
		path := strings.TrimPrefix(made.URL, "http://"+shareHost)
		guest := f.s.Browser()
		open := func(p string) hubtest.Resp { t.Helper(); return guest.DoAt(f.s.URL, shareHost, "GET", p, nil) }
		if r := open(path + "/index.json"); r.Code != 200 || !strings.Contains(string(r.Body), "Main.dc.html") {
			t.Fatalf("guest index: %s", r)
		}
		if r := open(path + "/f/Main.dc.html"); r.Code != 200 || !strings.Contains(string(r.Body), "shop main") {
			t.Fatalf("guest page: %s", r)
		}
		if r := open("/api/me"); r.Code == 200 {
			t.Fatalf("the hub's API on the share host: %s", r)
		}
		if r := open("/m/" + f.ownerM.MachineID + "/"); r.Code == 200 {
			t.Fatalf("the machine on the share host: %s", r)
		}
		if r := open("/s/not-a-token/index.json"); r.Code != 404 {
			t.Fatalf("an unknown link: %s", r)
		}
		// The machine's own origin does not serve the guest's link either.
		if f.sub {
			if r := f.ownerAt.raw.DoAt(f.s.URL, f.ownerAt.host, "GET", path+"/index.json", nil); r.Code == 200 {
				t.Fatalf("a guest link on a machine's origin: %s", r)
			}
		}
	})
}

// 5. The invitation link of a design leads a member into the workspace.
func (f *modesFixture) scenarioInvite(t *testing.T) {
	t.Run("invite link", func(t *testing.T) {
		if r := f.owner.Do("PATCH", "/api/machines/"+f.ownerM.MachineID, map[string]any{"access": []map[string]string{{"kind": "user", "id": f.owner.ID}}}); r.Code != 200 {
			t.Fatalf("close the owner's machine: %s", r)
		}
		r := f.ownerAt.do("POST", "/api/platform/design/access/shop/invites", map[string]any{"role": "view", "ttl": "1h", "roleTtl": nil})
		if r.Code != 200 {
			t.Fatalf("invite: %s", r)
		}
		var link struct{ ID, URL, Token string }
		r.JSON(&link)
		u, err := url.Parse(link.URL)
		if err != nil || u.Path != "/design-invites/"+link.Token {
			t.Fatalf("expected a hub invitation, got %q", link.URL)
		}
		page := "/designly/invite/" + link.Token
		if r := f.member.Do("GET", u.Path, nil); r.Header.Get("Location") != "/agents"+page {
			t.Fatalf("the invitation for the member: %s %v", r, r.Header)
		}
		// a person of another tenant is led into their own workspace, not refused.
		client := f.s.SignIn("client@other.example")
		client.NewSpace("Client")
		if r := client.Do("GET", u.Path, nil); r.Header.Get("Location") != "/agents"+page {
			t.Fatalf("the invitation for a person of another tenant: %s %v", r, r.Header)
		}
		if r := f.member.Do("GET", "/m/"+f.ownerM.MachineID+page, nil); r.Header.Get("Location") != u.Path {
			t.Fatalf("the old invitation link: %s %v", r, r.Header)
		}
		if r := f.member.Do("POST", "/agents/api/platform/design/access/invites/"+link.Token+"/accept", map[string]any{}); r.Code != 200 {
			t.Fatalf("accept: %s", r)
		}
		if role, _ := roleIn(t, f.memberAt); role != "view" {
			t.Fatalf("the member's role after accepting: %q", role)
		}
	})
}
