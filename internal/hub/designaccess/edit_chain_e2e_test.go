package designaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/filesync"
	"github.com/ly-solutions/harnessly/internal/hub/config"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hub/storehub"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// HRN-1794 / HRN-1793 (GitHub #193), «Готово когда»: two accounts, two machines, the whole chain through the hub's
// store and never through the owner's machine:
//
//	A gives B «edit» → B's machine gets the project → B takes the edit lease, A sees who edits and on which machine →
//	B edits on B's machine, the save goes to the store on its base revision → A's machine has B's edit →
//	A and B edit the same board from the same revision: the later write loses, its edit is put beside, not lost.
func TestEditChainTwoAccountsTwoMachines(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	// The hub reads projects from the store, as it does whenever the store is on: without a view the hub's own routes
	// (the lease among them) answer «store_off».
	r.svc.UseView(&versionsView{fakeLibrary: fakeLibrary{heads: map[string]int64{"plan-chain": 2}}})

	dir := t.TempDir()
	meta, err := filestore.OpenMetaStore(ctx, "sqlite://"+dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { meta.Close() })
	blobs, err := filestore.NewFileBlobs(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { blobs.Close() })
	next := r.owner
	st, err := storehub.Open(ctx, storehub.Options{Config: &config.Config{}, DB: e2eMachines{r.db},
		Key: []byte("0123456789abcdef0123456789abcdef"), Meta: meta, Blobs: blobs, Design: e2eDesign{r.svc},
		Admit: func(*http.Request) (store.Machine, error) { return next, nil }})
	if err != nil {
		t.Fatal(err)
	}
	r.svc.OnAccessChanged(st.AccessChanged)
	mux := http.NewServeMux()
	st.Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	// A machine: its own session with the store and a real filesync agent over its own library folder.
	machine := func(m store.Machine) (string, *filesync.Agent) {
		next = m
		resp, err := http.Post(srv.URL+hubproto.StorePath, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var s hubproto.StoreSession
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			t.Fatal(err)
		}
		c := filesync.NewClient(srv.URL+hubproto.StoreAPIPath, filestore.KindDesign,
			func(context.Context, bool) (string, error) { return s.Token, nil }, nil)
		root := t.TempDir()
		a, err := filesync.New(filesync.Config{Root: root, StateDir: t.TempDir(), Store: c, Machine: m.Name,
			Policy: filestore.PolicyFor(filestore.KindDesign), KeepUnseen: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(a.Close)
		return root, a
	}
	adir, a := machine(r.owner)
	bdir, b := machine(r.mate)
	write := func(root, rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(root, rel string) string {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s on %s: %v", rel, root, err)
		}
		return string(got)
	}
	pull := func(ag *filesync.Agent) {
		if _, err := ag.Pull(ctx, 0); err != nil {
			t.Fatal(err)
		}
	}
	scan := func(ag *filesync.Agent, rel string) {
		if err := ag.Scan(ctx, rel); err != nil {
			t.Fatal(err)
		}
	}

	// 1. A makes a personal project on A's machine; it goes to the store.
	const id = "plan-chain"
	const board = id + "/project/Main.dc.html"
	write(adir, id+"/.owner.json", `{"v":2,"user":"coder","name":"Owner","person":"owner@example.com","private":true}`)
	write(adir, board, "<main>v1</main>")
	scan(a, id)
	if err := r.svc.write(ctx, r.owner, func(t *txn) error { return t.setTeam(id, r.ownerU.Email, levelNone) }); err != nil {
		t.Fatal(err)
	}
	pull(b)
	if _, err := os.Stat(filepath.Join(bdir, id)); err == nil {
		t.Fatal("before the grant B's machine already holds A's personal project")
	}

	// 2. A gives B «edit»; B's machine gets the project from the store (A's machine plays no part from here on).
	if err := r.svc.write(ctx, r.owner, func(t *txn) error {
		return t.grant(id, r.ownerU.Email, r.mateU.Email, roleEdit, ttl{none: true}, false)
	}); err != nil {
		t.Fatal(err)
	}
	pull(b)
	if got := read(bdir, board); got != "<main>v1</main>" {
		t.Fatalf("B's machine has %q, not A's board", got)
	}

	// 3. «Редактировать» on B's page takes the lease; A's header names B and B's machine.
	lease := func(u store.User, m store.Machine, method string) (int, *editLease) {
		req := httptest.NewRequest(method, "/m/"+m.ID+"/api/platform/design/store/"+id+"/lease", nil)
		w := httptest.NewRecorder()
		if !r.svc.ServeMachine(w, req, u, m) {
			t.Fatal("the hub left the lease to the machine")
		}
		var out struct{ Lease *editLease }
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.Lease
	}
	if code, l := lease(r.mateU, r.mate, http.MethodPost); code != 200 || l == nil || !l.Mine {
		t.Fatalf("B did not take the edit lease: %d %+v", code, l)
	}
	if code, l := lease(r.ownerU, r.owner, http.MethodGet); code != 200 || l == nil || l.Mine ||
		l.Person != normEmail(r.mateU.Email) || l.MachineName != r.mate.Name {
		t.Fatalf("A does not see that B edits on B's machine: %d %+v", code, l)
	}

	// 4. B edits on B's machine; the save goes to the store on its base revision; A's machine has it.
	write(bdir, board, "<main>v2 by B</main>")
	scan(b, board)
	pull(a)
	if got := read(adir, board); got != "<main>v2 by B</main>" {
		t.Fatalf("the owner does not see B's edit: A's machine has %q", got)
	}

	// 5. Both edit the board from the same revision. B saves first; A's save is stale: A's machine takes B's version in
	// the board's place, and A's own edit is put beside it as a variant project, not lost.
	pull(b)
	write(bdir, board, "<main>v3, B again</main>") // another size: the scan does not take it for the unchanged v2
	big := "<main>" + strings.Repeat("A's big rewrite. ", 400) + "</main>"
	write(adir, board, big)
	scan(b, board)
	scan(a, board)
	if got := read(adir, board); got != "<main>v3, B again</main>" {
		t.Fatalf("after the stale save A's board is %q, not the store's newest", got)
	}
	entries, err := os.ReadDir(adir)
	if err != nil {
		t.Fatal(err)
	}
	var variant string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), id+"-variant-") {
			variant = e.Name()
		}
	}
	if variant == "" {
		t.Fatalf("A's losing edit was not put beside the board: %v", entries)
	}
	var link filesync.VariantLink
	if err := json.Unmarshal([]byte(read(adir, variant+"/"+filesync.VariantFile)), &link); err != nil || link.Of != id {
		t.Fatalf("variant %s does not name %s: %+v %v", variant, id, link, err)
	}
	if got := read(adir, variant+"/project/Main.dc.html"); !bytes.Equal([]byte(got), []byte(big)) {
		t.Fatalf("the variant does not hold A's edit (%d bytes)", len(got))
	}

	// 6. B leaves the editing mode: the lease is given back, nobody edits.
	if code, l := lease(r.mateU, r.mate, http.MethodDelete); code != 200 || l != nil {
		t.Fatalf("B's lease was not given back: %d %+v", code, l)
	}
	if code, l := lease(r.ownerU, r.owner, http.MethodGet); code != 200 || l != nil {
		t.Fatalf("A still sees an editor after B left: %d %+v", code, l)
	}
}
