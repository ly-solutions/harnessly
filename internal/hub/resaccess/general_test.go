package resaccess

import (
	"context"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/linkseal"
)

// «Общий доступ → Все, у кого есть ссылка» for a document or a board, through the route the share dialog
// calls: on, the address opens for anyone; off, it stops; on again, the same address opens again. A hub whose key
// changed gives a new address and the old one stops.
func TestGeneralAccess(t *testing.T) {
	s := newService(t)
	s.Seal = linkseal.New([]byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	general := func(v map[string]any) (string, string) {
		g, _ := v["general"].(map[string]any)
		level, _ := g["level"].(string)
		url, _ := g["url"].(string)
		return level, url
	}

	if level, _ := general(call(t, s, owner, "GET", "", "", 200)); level != levelNone {
		t.Fatalf("a new resource is open to anyone: %q", level)
	}
	v := call(t, s, owner, "PUT", "general", `{"level":"view"}`, 200)
	level, url := general(v)
	if level != roleView || !strings.HasPrefix(url, "https://hub.example.com/d/") {
		t.Fatalf("on: %v", v)
	}
	if shares, _ := v["shares"].([]any); len(shares) != 0 {
		t.Fatalf("the general link is listed among the links: %v", shares)
	}
	token := strings.TrimPrefix(url, "https://hub.example.com/d/")
	if g, err := s.Resolve(ctx, token); err != nil || g.Rights != roleView || g.Ref != ref {
		t.Fatalf("a guest opens it: %+v %v", g, err)
	}

	call(t, s, proved("stranger@example.com"), "PUT", "general", `{"level":"view"}`, 403)

	if level, _ := general(call(t, s, owner, "PUT", "general", `{"level":"none"}`, 200)); level != levelNone {
		t.Fatalf("off: %q", level)
	}
	if _, err := s.Resolve(ctx, token); err == nil {
		t.Fatal("the general link opens after it was switched off")
	}

	level, again := general(call(t, s, owner, "PUT", "general", `{"level":"comment"}`, 200))
	if level != roleComment || again != url {
		t.Fatalf("on again: %q %q, want the same address %q", level, again, url)
	}
	if g, err := s.Resolve(ctx, token); err != nil || g.Rights != roleComment {
		t.Fatalf("the same address opens again: %+v %v", g, err)
	}

	s.Seal = linkseal.New([]byte("another hub key, 32 bytes long.."))
	if _, url := general(call(t, s, owner, "GET", "", "", 200)); url != "" {
		t.Fatalf("an address the hub cannot open is shown: %q", url)
	}
	_, fresh := general(call(t, s, owner, "PUT", "general", `{"level":"view"}`, 200))
	if fresh == "" || fresh == url {
		t.Fatalf("a changed key gives a new address: %q", fresh)
	}
	if _, err := s.Resolve(ctx, token); err == nil {
		t.Fatal("the old address opens after it was replaced")
	}

	call(t, s, owner, "PUT", "general", `{"level":"edit"}`, 400)
}

// HRN-1791, the table of can.go: an editor the owner lets share invites people, but «anyone with the link» is a
// manager's; a manager sets it. The view tells the page the same through «can».
func TestGeneralAccessIsAManagers(t *testing.T) {
	s := newService(t)
	s.Seal = linkseal.New([]byte("0123456789abcdef0123456789abcdef"))
	ed := proved("ed@example.com")
	call(t, s, owner, "PUT", "people/ed@example.com", `{"role":"edit"}`, 200)
	call(t, s, owner, "PUT", "settings", `{"editorsCanShare":true}`, 200)
	v := call(t, s, ed, "GET", "", "", 200)
	can, _ := v["can"].(map[string]any)
	if v["canManage"] != true || can["share"] != true || can["edit"] != true || can["general"] != false {
		t.Fatalf("an editor who may share: canManage %v, can %v", v["canManage"], can)
	}
	call(t, s, ed, "PUT", "general", `{"level":"view"}`, 403)
	call(t, s, owner, "PUT", "people/ed@example.com", `{"role":"manage"}`, 200)
	call(t, s, ed, "PUT", "general", `{"level":"view"}`, 200)
}
