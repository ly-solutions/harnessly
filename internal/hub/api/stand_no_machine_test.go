package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// The stand of HRN-1318: a hub whose store keeps its contents in an S3-compatible bucket (HUB_TEST_STORE_S3_BUCKET with
// HUB_TEST_S3_*; without them it skips), a machine, a person, a design project with guest links. The machine is
// stopped, and what a guest reads is asked over HTTP by the same script the deployment is checked with
// (HUB_STAND_SCRIPT, optional) and by the test itself: the pages, a file outside the link, a revoked link.
func TestStandDesignIsReadFromTheBucketWithTheMachineOff(t *testing.T) {
	if os.Getenv("HUB_TEST_STORE_S3_BUCKET") == "" || os.Getenv("HUB_TEST_S3_ENDPOINT") == "" {
		t.Skip("HUB_TEST_STORE_S3_BUCKET and HUB_TEST_S3_ENDPOINT are not set: no bucket to stand on")
	}
	s := hubtest.New(t, hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost), hubtest.Env("HUB_DESIGN_DERIVED", "on"))
	if s.Hub.Store == nil {
		t.Fatal("the hub runs without a store")
	}
	owner := s.SignIn("owner@example.com")
	space, _ := owner.NewSpace("Studio")
	a, _ := designMachine(t, s, owner, space)
	created(a, "shop", "owner@example.com")
	s.Eventually("the project recorded", func() bool { r, ok := snapshotFor(a, "owner@example.com", "shop"); return ok && r == "owner" })
	ctx := context.Background()
	room, err := s.Hub.Store.DesignRoom(ctx, space)
	if err != nil {
		t.Fatal(err)
	}
	put := func(path, body string) {
		t.Helper()
		sum := sha256.Sum256([]byte(body))
		sha := hex.EncodeToString(sum[:])
		if err := room.PutBlob(ctx, sha, bytes.NewReader([]byte(body))); err != nil {
			t.Fatal(err)
		}
		if _, conflict, err := room.Commit(ctx, filestore.CommitRequest{Path: path, Sha: sha, Size: int64(len(body)), Author: "owner"}, "vm-owner"); err != nil || conflict {
			t.Fatalf("commit %s: %v %v", path, err, conflict)
		}
	}
	put("shop/artifact.json", `{"title":"Shop"}`)
	put("shop/project/Secret.dc.html", `<html><body>store secret</body></html>`)
	put("shop/project/Main.dc.html", `<html><body>store v1</body></html>`)

	base := "/m/" + a.MachineID + "/api/platform/design/access"
	link := func(body map[string]any) (id, address string) {
		t.Helper()
		r := owner.Do("POST", base+"/shop/shares", body)
		if r.Code != 201 {
			t.Fatalf("create link: %s", r)
		}
		var l struct{ ID, URL string }
		r.JSON(&l)
		return l.ID, l.URL
	}
	_, live := link(map[string]any{"rights": "view", "pages": []string{"Main.dc.html"}})
	goneID, gone := link(map[string]any{"rights": "view"})
	if r := owner.Do("DELETE", base+"/shop/shares/"+goneID, nil); r.Code >= 300 {
		t.Fatalf("revoke: %s", r)
	}

	a.Close()
	s.Eventually("the machine offline", func() bool { return s.Hub.Registry.Get(a.MachineID) == nil })

	guest := s.Browser()
	open := func(address, rest string) hubtest.Resp {
		t.Helper()
		return guest.DoAt(s.URL, shareHost, "GET", strings.TrimPrefix(address, "http://"+shareHost)+rest, nil)
	}
	rows := []struct {
		name string
		ok   bool
	}{
		{"the list of boards by the link", func() bool {
			r := open(live, "/index.json")
			return r.Code == 200 && strings.Contains(string(r.Body), "Main.dc.html")
		}()},
		{"a board's page", func() bool {
			r := open(live, "/f/Main.dc.html")
			return r.Code == 200 && strings.Contains(string(r.Body), "store v1")
		}()},
		{"the same again, from the kept answer", open(live, "/index.json").Code == 200},
		{"a page outside the link stays closed", open(live, "/f/Secret.dc.html").Code != 200},
		{"a revoked link is gone", open(gone, "/index.json").Code == 410},
	}
	for _, r := range rows {
		t.Logf("stand: %-40s %v", r.name, r.ok)
		if !r.ok {
			t.Errorf("stand: %s", r.name)
		}
	}
	var kept int
	if err := s.DB.S().Q().QueryRow(ctx, `SELECT COUNT(*) FROM design_derived`).Scan(&kept); err != nil || kept == 0 {
		t.Errorf("stand: the answers were not kept in the store: %d %v", kept, err)
	}

	if script := os.Getenv("HUB_STAND_SCRIPT"); script != "" {
		u, _ := url.Parse(s.URL)
		cmd := exec.Command("bash", script)
		cmd.Env = append(os.Environ(), "DESIGN="+live, "BOARD_FILE=Main.dc.html", "REVOKED="+gone,
			"CURL_OPTS=--connect-to "+shareHost+":80:"+u.Host)
		out, err := cmd.CombinedOutput()
		t.Logf("stand, the deployment's script:\n%s", out)
		if err != nil {
			t.Errorf("stand: the script: %v", err)
		}
	}
}
