package design

import (
	"strings"
	"testing"
)

// A site board's thread: a pin, a reply under it, a reaction turned on and off, «Решено», the agent's
// words, and the board's shared settings.
func TestSiteCommentThread(t *testing.T) {
	l := testLibrary(t, legacyLibrary(t))
	anna := DeckPerson{Name: "Анна", Email: "anna@example.com"}
	root, err := l.AddSiteComment("admin", anna, SiteCommentSpec{Board: "All.dc.html", X: 10, Y: 20, Block: "Шапка",
		Text: "Кнопку левее @Пётр", Mentions: []DeckPerson{{Name: "Пётр", Email: "Petr@Example.com"}, {Name: "Анна", Email: "anna@example.com"}}, Agent: true})
	if err != nil {
		t.Fatal(err)
	}
	if root.View != DeckCommentSite || len(root.Mentions) != 1 || root.Mentions[0].Email != "petr@example.com" || !root.Agent {
		t.Fatalf("root: %+v", root)
	}
	if _, err := l.AddSiteComment("admin", anna, SiteCommentSpec{Board: "../x", X: 1, Y: 1, Text: "x"}); err == nil {
		t.Error("a board outside the project was taken")
	}
	reply, err := l.AddSiteComment("admin", DeckPerson{Name: "harnessly"}, SiteCommentSpec{Parent: root.ID, Text: "Поправил", ByAgent: true})
	if err != nil || reply.Board != "All.dc.html" || reply.Parent != root.ID {
		t.Fatalf("reply: %+v %v", reply, err)
	}
	if _, err := l.AddSiteComment("admin", anna, SiteCommentSpec{Parent: reply.ID, Text: "nested"}); err == nil {
		t.Error("a reply to a reply was taken")
	}
	if _, err := l.ChangeSiteComment("admin", root.ID, anna, nil, "👍"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ChangeSiteComment("admin", root.ID, anna, nil, "💩"); err == nil {
		t.Error("an unknown reaction was taken")
	}
	got := l.ReadSiteComments("admin", "All.dc.html", anna)
	if len(got) != 2 || len(got[0].Reactions) != 1 || !got[0].Reactions[0].Mine || got[0].Reactions[0].Count != 1 {
		t.Fatalf("read: %+v", got)
	}
	if other := l.ReadSiteComments("admin", "", DeckPerson{Name: "Ксения", Email: "k@example.com"}); other[0].Reactions[0].Mine {
		t.Error("another person's reaction read as mine")
	}
	if _, err := l.ChangeSiteComment("admin", root.ID, anna, nil, "👍"); err != nil {
		t.Fatal(err)
	}
	yes := true
	c, err := l.ChangeSiteComment("admin", root.ID, anna, &yes, "")
	if err != nil || !c.Resolved || len(c.Reactions) != 0 {
		t.Fatalf("resolve: %+v %v", c, err)
	}
	if _, err := l.ChangeSiteComment("admin", reply.ID, anna, &yes, ""); err == nil {
		t.Error("a reply was resolved alone")
	}
	text := siteAgentText("admin", "Admin", root, root)
	if !strings.Contains(text, "harnessly design comments reply admin "+root.ID) || !strings.Contains(text, "Шапка") {
		t.Errorf("agent text: %s", text)
	}

	if _, err := l.SetSiteProps("admin", "All.dc.html", map[string]SitePropValue{"theme": "dark", "count": 3.0, "bad": []any{1}}, false, "Анна"); err != nil {
		t.Fatal(err)
	}
	p := l.ReadSiteProps("admin")
	if v := p.Boards["All.dc.html"]; len(v) != 2 || v["theme"] != "dark" || v["count"] != 3.0 || p.By != "Анна" {
		t.Fatalf("props: %+v", p)
	}
	if _, err := l.SetSiteProps("admin", "All.dc.html", map[string]SitePropValue{}, false, "Анна"); err != nil {
		t.Fatal(err)
	}
	if p := l.ReadSiteProps("admin"); len(p.Boards) != 0 {
		t.Fatalf("cleared: %+v", p)
	}
}
