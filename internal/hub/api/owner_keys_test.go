package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/api"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
	"github.com/ly-solutions/harnessly/internal/hubproto"
	"github.com/ly-solutions/harnessly/internal/secrets/secretstest"
)

// ownerStack is a hub with every personal service of a machine switched on, over kv.
func ownerStack(t *testing.T, kv *secretstest.MemKV, env ...hubtest.Option) *hubtest.Stack {
	t.Helper()
	return hubtest.New(t, append([]hubtest.Option{
		hubtest.Env("HUB_FEATURES_FORCE", "secrets.vault"), hubtest.Env("HUB_MEMORY_SYNC", "on"), hubtest.Env("HUB_ORGS", "on"), hubtest.Env("HUB_STORE", "on"),
		hubtest.WithOptions(func(o *api.Options) { o.SafeKV = kv }),
	}, env...)...)
}

func safeCall(t *testing.T, s *hubtest.Stack, m *hubtest.Agent, req hubproto.SecretsRequest) hubtest.Resp {
	t.Helper()
	host, _ := hubproto.DialHost(s.Cfg.PublicURL)
	body, _ := json.Marshal(req)
	auth, _ := hubproto.SignSecrets(m.Priv, m.MachineID, s.Clock.Now(), host, body)
	return s.Browser().Do("POST", hubproto.SecretsPath, body, "Authorization", auth)
}

// personal asks every service that answers a machine in its owner's name, and says what each answered.
func personal(t *testing.T, s *hubtest.Stack, m *hubtest.Agent) map[string]hubtest.Resp {
	t.Helper()
	host, _ := hubproto.DialHost(s.Cfg.PublicURL)
	history, _ := hubproto.SignHistory(m.Priv, m.MachineID, s.Clock.Now(), host)
	routines, _ := hubproto.SignRoutines(m.Priv, m.MachineID, s.Clock.Now(), host)
	store, _ := hubproto.SignStore(m.Priv, m.MachineID, s.Clock.Now(), host)
	var session hubproto.StoreSession
	if r := s.Browser().Do("POST", hubproto.StorePath, nil, "Authorization", store); r.Code != http.StatusOK {
		t.Fatalf("store session: %s", r)
	} else {
		r.JSON(&session)
	}
	return map[string]hubtest.Resp{
		"profile":  s.Browser().Do("GET", hubproto.StoreAPIPath+"/v1/collections/profile/changes?since=0", nil, "Authorization", "Bearer "+session.Token),
		"safe":     safeCall(t, s, m, hubproto.SecretsRequest{Op: hubproto.SecretsList}),
		"memory":   memoryPost(t, s, m, hubproto.MemoryPullPath, hubproto.MemoryPull{Projects: []string{"git:example.com/acme/app"}}),
		"history":  s.Browser().Do("POST", hubproto.HistoryReadPath, hubproto.HistoryRead{Conversation: "tab-x", Limit: 10}, "Authorization", history),
		"routines": s.Browser().Do("POST", hubproto.RoutinesPath, hubproto.RoutinesCall{Op: "list"}, "Authorization", routines),
	}
}

func refused(t *testing.T, s *hubtest.Stack, m *hubtest.Agent, when, code string) {
	t.Helper()
	for name, r := range personal(t, s, m) {
		if r.Code != http.StatusForbidden || name != "profile" && r.Err() != code {
			t.Errorf("%s: %s answered %s, want 403 %s", when, name, r, code)
		}
	}
}

// relink registers the machine again over its registration, with a code of by's, and returns the agent with its new keys.
func relink(t *testing.T, s *hubtest.Stack, old *hubtest.Agent, by *hubtest.Person, space string) *hubtest.Agent {
	t.Helper()
	a := s.NewAgent(nil)
	code := by.NewCode(space, "dir")
	req := a.RegisterRequest(code, hubproto.ProfileDir, hubproto.ViaCommand)
	req.Previous = hubproto.SignReregister(old.Priv, old.MachineID, code, req.PublicKey)
	r := s.Browser().Do("POST", "/agent/register", req)
	if r.Code != http.StatusOK {
		t.Fatalf("register again: %s", r)
	}
	a.MachineID = old.MachineID
	return a
}

// a machine handed to another person keeps its keys, and those keys reach nothing of the new owner's until
// the owner registers the machine again with a code of their own.
func TestTransferredMachineKeysDoNotReachTheNewOwner(t *testing.T) {
	kv := secretstest.New()
	s := ownerStack(t, kv)
	admin, alice, bob := s.SignIn("admin@example.com"), s.SignIn("alice@example.com"), s.SignIn("bob@example.com")
	space, _ := admin.NewSpace("Acme")
	admin.Join(space, alice)
	admin.Join(space, bob)
	m := admin.Machine(space, nil)
	kv.Data["harnessly/users/"+alice.ID+"/gh-token"] = map[string]string{"value": "alice's"}
	if r := safeCall(t, s, m, hubproto.SecretsRequest{Op: hubproto.SecretsList}); r.Code != http.StatusOK {
		t.Fatalf("before the transfer: %s", r)
	}
	if r := admin.Do("POST", "/api/machines/"+m.MachineID+"/owner", map[string]string{"userId": alice.ID}); r.Code != http.StatusOK {
		t.Fatalf("transfer: %s", r)
	}
	refused(t, s, m, "the old keys after the transfer", "machine_relink_required")
	for _, req := range []hubproto.SecretsRequest{
		{Op: hubproto.SecretsRead, Name: "gh-token"},
		{Op: hubproto.SecretsWrite, Name: "gh-token", Data: map[string]string{"value": "mine now"}},
		{Op: hubproto.SecretsDelete, Name: "gh-token"},
	} {
		if r := safeCall(t, s, m, req); r.Code != http.StatusForbidden {
			t.Errorf("%s with the old keys: %s", req.Op, r)
		}
	}
	if kv.Data["harnessly/users/"+alice.ID+"/gh-token"]["value"] != "alice's" {
		t.Error("the old keys changed the new owner's secret")
	}

	// Keys registered again with somebody else's code are not the owner's either: the administrator's, a colleague's.
	by := relink(t, s, m, admin, space)
	refused(t, s, by, "registered again by the administrator", "machine_relink_required")
	by = relink(t, s, by, bob, space)
	refused(t, s, by, "registered again by a colleague", "machine_relink_required")

	own := relink(t, s, by, alice, space)
	for name, r := range personal(t, s, own) {
		if r.Code == http.StatusForbidden {
			t.Errorf("registered again by the owner: %s answered %s", name, r)
		}
	}
	if r := safeCall(t, s, own, hubproto.SecretsRequest{Op: hubproto.SecretsRead, Name: "gh-token"}); r.Code != http.StatusOK {
		t.Errorf("the owner's keys read the owner's secret: %s", r)
	}
}

// The new owner accepts the machine in the web instead of registering it again: only they can, signed in as a person on
// a page of the hub's own origin.
func TestOnlyTheOwnerAcceptsAMachine(t *testing.T) {
	kv := secretstest.New()
	s := ownerStack(t, kv)
	admin, alice, bob := s.SignIn("admin@example.com"), s.SignIn("alice@example.com"), s.SignIn("bob@example.com")
	space, _ := admin.NewSpace("Acme")
	admin.Join(space, alice)
	admin.Join(space, bob)
	m := admin.Machine(space, nil)
	if r := admin.Do("POST", "/api/machines/"+m.MachineID+"/owner", map[string]string{"userId": alice.ID}); r.Code != http.StatusOK {
		t.Fatalf("transfer: %s", r)
	}
	refused(t, s, m, "before the owner's yes", "machine_relink_required")
	accept := "/api/machines/" + m.MachineID + "/accept"
	for who, p := range map[string]*hubtest.Person{"the space's owner who connected it": admin, "a colleague": bob} {
		if r := p.Do("POST", accept, map[string]string{}); r.Code != http.StatusForbidden {
			t.Errorf("%s accepted the machine: %s", who, r)
		}
	}
	for what, h := range map[string][]string{
		"a page of another site":     {"Sec-Fetch-Site", "cross-site"},
		"a page of a machine's host": {"Sec-Fetch-Site", "same-site"},
		"another origin":             {"Origin", "https://evil.example"},
		"a form":                     {"Content-Type", "text/plain"},
	} {
		if r := alice.Do("POST", accept, []byte(`{}`), h...); r.Code != http.StatusForbidden {
			t.Errorf("%s accepted the machine: %s", what, r)
		}
	}
	if r := s.Browser().Do("POST", accept, map[string]string{}); r.Code != http.StatusUnauthorized {
		t.Errorf("nobody accepted the machine: %s", r)
	}
	signed, _ := hubproto.SignMemory(m.Priv, m.MachineID, s.Clock.Now(), "hub")
	if r := s.Browser().Do("POST", accept, map[string]string{}, "Authorization", signed); r.Code != http.StatusUnauthorized {
		t.Errorf("the machine accepted itself: %s", r)
	}
	refused(t, s, m, "after every refused yes", "machine_relink_required")

	if r := alice.Do("POST", accept, map[string]string{}); r.Code != http.StatusOK {
		t.Fatalf("the owner accepts: %s", r)
	}
	for name, r := range personal(t, s, m) {
		if r.Code == http.StatusForbidden {
			t.Errorf("accepted: %s answered %s", name, r)
		}
	}
	// The one who connected it leaves: the machine its owner accepted stays.
	if err := s.DB.S().SetRole(t.Context(), space, alice.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if r := alice.Do("DELETE", "/api/spaces/"+space+"/members/"+admin.ID, nil); r.Code != http.StatusNoContent {
		t.Fatalf("removing the connector: %s", r)
	}
	if got, err := s.DB.S().MachineByID(t.Context(), m.MachineID); err != nil || got.RevokedAt != nil {
		t.Errorf("the accepted machine was revoked: %v", err)
	}
}

// Where members do not connect machines, a member may still register their own machine again, and nothing else.
func TestMemberRelinksTheirOwnMachineWhereMembersConnectNone(t *testing.T) {
	s := ownerStack(t, secretstest.New())
	admin, alice := s.SignIn("admin@example.com"), s.SignIn("alice@example.com")
	space, _ := admin.NewSpace("Acme")
	admin.Join(space, alice)
	var org struct{ Org struct{ ID string } }
	if r := admin.Do("POST", "/api/orgs", map[string]any{"name": "Acme", "spaceIds": []string{space}}); r.Code != http.StatusCreated {
		t.Fatalf("org: %s", r)
	} else {
		r.JSON(&org)
	}
	m := admin.Machine(space, nil)
	if _, err := s.DB.Exec(t.Context(), `UPDATE orgs SET member_machines = 'off' WHERE id = ?`, org.Org.ID); err != nil {
		t.Fatal(err)
	}
	codes := "/api/spaces/" + space + "/machines/codes"
	if r := alice.Do("POST", codes, map[string]string{"profile": "dir"}); r.Code != http.StatusForbidden {
		t.Fatalf("a code with nothing to register again: %s", r)
	}
	if r := admin.Do("POST", "/api/machines/"+m.MachineID+"/owner", map[string]string{"userId": alice.ID}); r.Code != http.StatusOK {
		t.Fatalf("transfer: %s", r)
	}
	if r := s.NewAgent(nil).Register(alice.NewCode(space, "dir"), hubproto.ProfileDir, hubproto.ViaCommand); r.Code != http.StatusForbidden {
		t.Errorf("the code registered a new machine: %s", r)
	}
	own := relink(t, s, m, alice, space)
	if r := safeCall(t, s, own, hubproto.SecretsRequest{Op: hubproto.SecretsList}); r.Code != http.StatusOK {
		t.Errorf("after registering again: %s", r)
	}
}

// the machines of an account that is switched off reach none of its data.
func TestSwitchedOffAccountsMachinesReachNothing(t *testing.T) {
	s := ownerStack(t, secretstest.New())
	admin, person := s.SignIn("admin@example.com"), s.SignIn("person@example.com")
	space, _ := admin.NewSpace("Acme")
	admin.Join(space, person)
	if err := s.DB.S().SetRole(t.Context(), space, person.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	m := person.Machine(space, nil)
	if r := admin.Do("PUT", "/api/admin/users/"+person.ID+"/disabled", map[string]bool{"disabled": true}); r.Code != http.StatusOK {
		t.Fatalf("switching off: %s", r)
	}
	refused(t, s, m, "the account switched off", "owner_disabled")
}

// the folders of references are a tenant's own: a second organization does not read what the first may.
func TestReferenceFoldersAreTheOrganizations(t *testing.T) {
	kv := secretstest.New()
	for _, p := range []string{"shared/ci/deploy", "teams/beta/deploy", "shared/all/x"} {
		kv.Data[p] = map[string]string{"key": "v"}
	}
	s := ownerStack(t, kv, hubtest.Env("HUB_SECRETS_VAULT_ADDR", "http://127.0.0.1:1"), hubtest.Env("HUB_SECRETS_VAULT_TOKEN_FILE", "/dev/null"),
		hubtest.Env("HUB_SECRETS_REF_PREFIXES", "shared/ci"))
	a, b := s.SignIn("a@example.com"), s.SignIn("b@example.com")
	first, _ := a.NewSpace("Acme")
	second, _ := b.NewSpace("Beta")
	newOrg := func(p *hubtest.Person, name, space string) string {
		var out struct{ Org struct{ ID string } }
		r := p.Do("POST", "/api/orgs", map[string]any{"name": name, "spaceIds": []string{space}})
		if r.Code != http.StatusCreated {
			t.Fatalf("org: %s", r)
		}
		r.JSON(&out)
		return out.Org.ID
	}
	newOrg(a, "Acme", first)
	ma := a.Machine(first, nil)
	ref := func(m *hubtest.Agent, path string) int {
		return safeCall(t, s, m, hubproto.SecretsRequest{Op: hubproto.SecretsRef, Path: path}).Code
	}
	// One organization: the list works as it did.
	if c := ref(ma, "shared/ci/deploy"); c != http.StatusOK {
		t.Fatalf("the only organization: %d", c)
	}
	s.Clock.Advance(time.Second)
	beta := newOrg(b, "Beta", second)
	mb := b.Machine(second, nil)
	if c := ref(mb, "shared/ci/deploy"); c != http.StatusForbidden {
		t.Errorf("the second organization read the first's folder: %d", c)
	}
	if c := ref(ma, "shared/ci/deploy"); c != http.StatusOK {
		t.Errorf("the first organization keeps its folder: %d", c)
	}
	s.Cfg.SafeOrgRefPrefixes = map[string][]string{"*": {"shared/all"}}
	if c := ref(mb, "shared/all/x"); c != http.StatusOK {
		t.Errorf("a folder of every organization: %d", c)
	}
	s.Cfg.SafeOrgRefPrefixes = map[string][]string{beta: {"teams/beta"}}
	if own, other := ref(mb, "teams/beta/deploy"), ref(ma, "teams/beta/deploy"); own != http.StatusOK || other != http.StatusForbidden {
		t.Errorf("an organization's own folder: its machine %d, another's %d", own, other)
	}
}

// An installation that had its organizations before the folders became theirs goes on reading them in every one it had;
// one made afterwards needs an entry of its own, the first organization of such an installation included.
func TestTenantsFromBeforeKeepTheReferenceFolders(t *testing.T) {
	kv := secretstest.New()
	kv.Data["shared/ci/deploy"] = map[string]string{"key": "v"}
	s := ownerStack(t, kv, hubtest.Env("HUB_SECRETS_VAULT_ADDR", "http://127.0.0.1:1"), hubtest.Env("HUB_SECRETS_VAULT_TOKEN_FILE", "/dev/null"),
		hubtest.Env("HUB_SECRETS_REF_PREFIXES", "shared/ci"))
	people := []*hubtest.Person{s.SignIn("a@example.com"), s.SignIn("b@example.com"), s.SignIn("c@example.com")}
	var spaces []string
	for i, p := range people {
		id, _ := p.NewSpace([]string{"Acme", "Beta", "Gamma"}[i])
		spaces = append(spaces, id)
	}
	newOrg := func(i int) string {
		var out struct{ Org struct{ ID string } }
		r := people[i].Do("POST", "/api/orgs", map[string]any{"name": []string{"Acme", "Beta", "Gamma"}[i], "spaceIds": []string{spaces[i]}})
		if r.Code != http.StatusCreated {
			t.Fatalf("org: %s", r)
		}
		r.JSON(&out)
		s.Clock.Advance(time.Second)
		return out.Org.ID
	}
	first, second := newOrg(0), newOrg(1)
	// As the migration writes them down: two organizations and a space outside any.
	for _, tenant := range []string{first, second, "space:" + spaces[2]} {
		if _, err := s.DB.Exec(t.Context(), `INSERT INTO safe_ref_tenants (tenant) VALUES (?)`, tenant); err != nil {
			t.Fatal(err)
		}
	}
	read := func(i int) int {
		return safeCall(t, s, people[i].Machine(spaces[i], nil), hubproto.SecretsRequest{Op: hubproto.SecretsRef, Path: "shared/ci/deploy"}).Code
	}
	for i := range people {
		if c := read(i); c != http.StatusOK {
			t.Errorf("a tenant from before (%d): %d", i, c)
		}
	}
	late := s.SignIn("d@example.com")
	space, _ := late.NewSpace("Delta")
	if r := late.Do("POST", "/api/orgs", map[string]any{"name": "Delta", "spaceIds": []string{space}}); r.Code != http.StatusCreated {
		t.Fatalf("org: %s", r)
	}
	if r := safeCall(t, s, late.Machine(space, nil), hubproto.SecretsRequest{Op: hubproto.SecretsRef, Path: "shared/ci/deploy"}); r.Code != http.StatusForbidden {
		t.Errorf("an organization made afterwards read the folder: %s", r)
	}
}
