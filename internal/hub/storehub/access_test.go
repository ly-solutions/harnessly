package storehub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/filesync"
)

// teamMachines are a person's two machines and a colleague's one.
func teamMachines() fakeMachines {
	return fakeMachines{
		"owner":  {ID: "owner", Name: "vm-owner", SpaceID: "sp1", CreatedBy: "u1"},
		"second": {ID: "second", Name: "vm-second", SpaceID: "sp1", CreatedBy: "u1"},
		"mate":   {ID: "mate", Name: "vm-mate", SpaceID: "sp1", CreatedBy: "u2"},
	}
}

// seed writes a revision into a room as the machine named, past every rule of access.
func seed(t *testing.T, st *Store, kind, machine, path, body string) {
	t.Helper()
	ctx := context.Background()
	rs, err := st.Server().Room(ctx, st.Room("sp1", kind))
	if err != nil {
		t.Fatal(err)
	}
	sha := shaOf(body)
	if err := rs.PutBlob(ctx, sha, bytes.NewReader([]byte(body))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rs.Commit(ctx, filestore.CommitRequest{Path: path, Sha: sha, Size: int64(len(body))}, machine); err != nil {
		t.Fatal(err)
	}
}

func putAs(c *filesync.Client, path, body string) error {
	_, err := c.Put(context.Background(), path, []byte(body), "t")
	return err
}

// deleteAs removes a path on top of its latest revision, which the machine may know without being shown it now.
func deleteAs(t *testing.T, st *Store, kind string, c *filesync.Client, path string) error {
	t.Helper()
	ctx := context.Background()
	rs, err := st.Server().Room(ctx, st.Room("sp1", kind))
	if err != nil {
		t.Fatal(err)
	}
	e, err := rs.Current(ctx, path)
	if err != nil || e == nil {
		t.Fatalf("current %s: %+v %v", path, e, err)
	}
	_, err = c.Commit(ctx, filestore.CommitRequest{Path: path, Base: e.Rev, Deleted: true})
	return err
}

// A lease follows the project of its key: it is listed to who sees the project, taken and forced off by who edits it.
func TestLeasesFollowTheProjectsAccess(t *testing.T) {
	machines := teamMachines()
	design := &fakeDesign{view: map[string]map[string]Grant{
		"owner": {"secret": {See: true, Edit: true, Own: true}, "shown": {See: true, Edit: true, Own: true}},
		"mate":  {"secret": {}, "shown": {See: true}},
	}}
	st, _ := designStore(t, machines, design)
	mux := http.NewServeMux()
	st.Mount(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	call := func(machine, method, path string, in, out any) int {
		t.Helper()
		var body bytes.Buffer
		if in != nil {
			if err := json.NewEncoder(&body).Encode(in); err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequest(method, srv.URL+"/store/v1/collections/design"+path, &body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+st.sign(machine, "sp1", time.Now().Add(time.Hour)))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if out != nil && resp.StatusCode == 200 {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				t.Fatal(err)
			}
		}
		return resp.StatusCode
	}
	take := func(machine, key string) int {
		return call(machine, "POST", "/leases", filestore.LeaseRequest{Key: key, Chat: "rt/" + machine}, nil)
	}
	for _, key := range []string{"secret/Main", "shown/Main", "task:pdf/secret"} {
		if code := take("owner", key); code != 200 {
			t.Fatalf("owner takes %s: %d", key, code)
		}
	}
	var got filestore.Leases
	call("mate", "GET", "/leases", nil, &got)
	if len(got.Leases) != 1 || got.Leases[0].Key != "shown/Main" {
		t.Fatalf("the mate is listed %+v", got.Leases)
	}
	for _, key := range []string{"secret/Main", "shown/Main", "shown/Other", "task:pdf/shown", "ask/secret/Main", "nonsense"} {
		if code := take("mate", key); code != http.StatusForbidden {
			t.Errorf("the mate takes %s: %d", key, code)
		}
	}
	// A viewer may ask the holder to let go.
	if code := take("mate", "ask/shown/Main"); code != 200 {
		t.Errorf("a viewer's ask: %d", code)
	}
	for _, key := range []string{"secret/Main", "shown/Main"} {
		if code := call("mate", "POST", "/leases/release", filestore.LeaseRelease{Key: key, Force: true}, nil); code != http.StatusForbidden {
			t.Errorf("the mate forces %s off: %d", key, code)
		}
	}
	got = filestore.Leases{}
	call("owner", "GET", "/leases", nil, &got)
	if len(got.Leases) != 4 {
		t.Fatalf("the owner is listed %+v", got.Leases)
	}
	if code := call("owner", "POST", "/leases/release", filestore.LeaseRelease{Key: "shown/Main", Chat: "rt/owner"}, nil); code != 200 {
		t.Errorf("the holder releases: %d", code)
	}
}

// With closed unrecorded projects a folder nobody recorded, and the record and cover of it, take writes only from the
// person whose machines wrote what is there.
func TestClosedUnrecordedTakesNoWritesOfOthers(t *testing.T) {
	machines := teamMachines()
	design := &fakeDesign{closed: true}
	st, client := designStore(t, machines, design)
	owner, second, mate := client("owner"), client("second"), client("mate")
	if err := putAs(owner, "legacy/project/Main.dc.html", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := putAs(mate, "legacy/project/planted.js", "x"); !errors.Is(err, filesync.ErrRefused) {
		t.Fatalf("a planted file: %v", err)
	}
	if err := putAs(second, "legacy/project/Other.dc.html", "v1"); err != nil {
		t.Fatalf("the person's other machine: %v", err)
	}
	if err := putAs(owner, "legacy/project/Main.dc.html", "v2"); err != nil {
		t.Fatalf("the writer goes on: %v", err)
	}
	// A file a machine wrote itself stays its own to change, though others wrote beside it.
	seed(t, st, filestore.KindDesign, "vm-mate", "mixed/project/mate.js", "m")
	seed(t, st, filestore.KindDesign, "vm-owner", "mixed/project/owner.js", "o")
	if err := deleteAs(t, st, filestore.KindDesign, owner, "mixed/project/owner.js"); err != nil {
		t.Fatalf("removing its own file: %v", err)
	}
	if err := putAs(owner, "mixed/project/mate.js", "mine"); !errors.Is(err, filesync.ErrRefused) {
		t.Fatalf("another's file: %v", err)
	}
	design.mu.Lock()
	design.closed = false
	design.mu.Unlock()
	if err := putAs(mate, "legacy/project/open.js", "x"); err != nil {
		t.Fatalf("open unrecorded projects take writes as before: %v", err)
	}
}

func TestClosedUnrecordedRecordsTakeNoWritesOfOthers(t *testing.T) {
	machines := teamMachines()
	design := &fakeDesign{closed: true}
	_, client := collectionStore(t, machines, design, filestore.KindDecks)
	owner, second, mate := client("owner"), client("second"), client("mate")
	rec := func(title string) string {
		b, err := filestore.Record{ID: "deck", Title: title, Kind: "deck", At: 5}.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if err := putAs(owner, "deck.json", rec("one")); err != nil {
		t.Fatal(err)
	}
	if err := putAs(mate, "deck.json", rec("planted")); !errors.Is(err, filesync.ErrRefused) {
		t.Fatalf("another's record: %v", err)
	}
	if err := putAs(second, "deck.json", rec("two")); err != nil {
		t.Fatalf("the person's other machine: %v", err)
	}
}

// An unrecorded kit that already holds files is claimed only by the person whose machines wrote them.
func TestAFilledKitIsClaimedOnlyByItsWriter(t *testing.T) {
	machines := teamMachines()
	kits := &fakeKits{owner: map[string]string{}}
	st, client := collectionStore(t, machines, &fakeDesign{}, filestore.KindKits)
	st.kits = kits
	owner, mate := client("owner"), client("mate")
	seed(t, st, filestore.KindKits, "vm-owner", "old-ds/1.0.0/tokens.json", "{}")
	if err := putAs(mate, "old-ds/2.0.0/evil.js", "x"); !errors.Is(err, filesync.ErrRefused) {
		t.Fatalf("a claim over another's kit: %v", err)
	}
	if kits.owner["old-ds"] != "" {
		t.Fatalf("claimed by %s", kits.owner["old-ds"])
	}
	if err := putAs(owner, "old-ds/2.0.0/tokens.json", "{}"); err != nil || kits.owner["old-ds"] != "owner" {
		t.Fatalf("the writer's claim: %v %v", err, kits.owner)
	}
}

// Uploads, delivered files and handoffs: a machine changes or removes only what its person's machines wrote.
func TestSharedBlobRoomsKeepOthersPaths(t *testing.T) {
	for _, kind := range []string{filestore.KindUploads, filestore.KindFiles, filestore.KindHandoff} {
		st, client := collectionStore(t, teamMachines(), &fakeDesign{}, kind)
		owner, second, mate := client("owner"), client("second"), client("mate")
		if err := putAs(owner, "chat/shot.png", "mine"); err != nil {
			t.Fatal(kind, err)
		}
		if err := putAs(mate, "chat/shot.png", "theirs"); !errors.Is(err, filesync.ErrRefused) {
			t.Errorf("%s: an overwrite: %v", kind, err)
		}
		if err := deleteAs(t, st, kind, mate, "chat/shot.png"); !errors.Is(err, filesync.ErrRefused) {
			t.Errorf("%s: a delete: %v", kind, err)
		}
		if err := putAs(second, "chat/shot.png", "again"); err != nil {
			t.Errorf("%s: the person's other machine: %v", kind, err)
		}
		if err := putAs(mate, "chat/own.png", "theirs"); err != nil {
			t.Errorf("%s: a new path: %v", kind, err)
		}
	}
}

// Apps: closed unrecorded documents and guest areas are not the space's, and a package is never replaced or removed.
func TestAppsAreasAndBundlesOfOthers(t *testing.T) {
	machines := teamMachines()
	design := &fakeDesign{closed: true}
	st, client := collectionStore(t, machines, design, filestore.KindApps)
	owner, second, mate := client("owner"), client("second"), client("mate")
	hidden := func(path string) {
		t.Helper()
		if e, err := mate.Current(context.Background(), path); err != nil || e != nil {
			t.Errorf("the mate reads %s: %+v %v", path, e, err)
		}
		if err := putAs(mate, path, `{"mate":1}`); !errors.Is(err, filesync.ErrRefused) {
			t.Errorf("the mate writes %s: %v", path, err)
		}
	}
	doc := "storage/acme.board/doc/draft.json"
	if err := putAs(owner, doc, `{"a":1}`); err != nil {
		t.Fatal(err)
	}
	hidden(doc)
	if err := putAs(second, doc, `{"a":2}`); err != nil {
		t.Fatalf("the person's other machine: %v", err)
	}
	// A guest area the hub wrote: the link names it, in the folder of the person who made the link.
	link := `{"link":{"id":"abc","app":"acme.board"},"version":"1","sum":""}`
	const hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	mine := "guests/" + PersonName("u1") + "/" + hash + ".json"
	if err := putAs(owner, mine, link); err != nil {
		t.Fatal(err)
	}
	guest := "storage/acme.board/doc/guest-abc.json"
	seed(t, st, filestore.KindApps, "hub", guest, `{"left":"by a guest"}`)
	hidden(guest)
	if err := putAs(second, guest, `{"seen":1}`); err != nil {
		t.Fatalf("the link maker's machine: %v", err)
	}
	// The link ended: its data stays its maker's.
	if err := deleteAs(t, st, filestore.KindApps, owner, mine); err != nil {
		t.Fatal(err)
	}
	seed(t, st, filestore.KindApps, "hub", guest, `{"left":"later"}`)
	hidden(guest)
	if err := putAs(owner, guest, `{"seen":2}`); err != nil {
		t.Fatalf("after the link ended: %v", err)
	}

	pack := "the package"
	bundle := "bundles/acme.board/" + shaOf(pack) + ".tgz"
	if err := putAs(mate, bundle, "something else"); !errors.Is(err, filesync.ErrRefused) {
		t.Errorf("a package that is not its name: %v", err)
	}
	if err := putAs(owner, bundle, pack); err != nil {
		t.Fatal(err)
	}
	if err := putAs(mate, bundle, "replaced"); !errors.Is(err, filesync.ErrRefused) {
		t.Errorf("a package replaced: %v", err)
	}
	if err := deleteAs(t, st, filestore.KindApps, mate, bundle); !errors.Is(err, filesync.ErrRefused) {
		t.Errorf("a package removed: %v", err)
	}
}
