package design

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
)

func parseJSONObject(t *testing.T, s string) *jsvalue.Object {
	t.Helper()
	v, err := jsvalue.Parse([]byte(s))
	o, ok := v.(*jsvalue.Object)
	if err != nil || !ok {
		t.Fatalf("not an object: %s", s)
	}
	return o
}

// A drawing kept with a comment: good shapes are kept rounded, bad ones dropped, a flood refused, and the
// comment may have no words.
func TestSiteDrawing(t *testing.T) {
	body := parseJSONObject(t, `{"drawing":{"shapes":[
		{"kind":"pen","color":"#E5484D","w":3,"pts":[[10.04,20],[30,40.06]]},
		{"kind":"arrow","color":"#e5484d","w":3,"pts":[[1,1]]},
		{"kind":"label","color":"#e5484d","w":3,"pts":[[5,5]],"text":"  тут  "},
		{"kind":"bomb","color":"#e5484d","w":3,"pts":[[5,5]]},
		{"kind":"rect","color":"red","w":3,"pts":[[5,5],[6,6]]}
	]}}`)
	d, ok := siteDrawingOf(Field(body, "drawing"))
	if !ok || d == nil || len(d.Shapes) != 2 || d.Shapes[0].Color != "#e5484d" || d.Shapes[0].Pts[0][0] != 10 || d.Shapes[1].Text != "тут" {
		t.Fatalf("drawing: %+v %v", d, ok)
	}
	if d, ok := siteDrawingOf(nil); !ok || d != nil {
		t.Error("no drawing is not a refusal")
	}
	pts := strings.Repeat("[1,1],", siteShapePoints-1) + "[1,1]"
	flood := `{"shapes":[` + strings.Repeat(`{"kind":"pen","color":"#000000","w":2,"pts":[`+pts+`]},`, 5) + `{"kind":"pen","color":"#000000","w":2,"pts":[` + pts + `]}]}`
	if _, ok := siteDrawingOf(parseJSONObject(t, flood)); ok {
		t.Error("a flood of points was taken")
	}

	l := testLibrary(t, legacyLibrary(t))
	anna := DeckPerson{Name: "Анна", Email: "anna@example.com"}
	c, err := l.AddSiteComment("admin", anna, SiteCommentSpec{Board: "All.dc.html", X: 10, Y: 20, Drawing: d})
	if err != nil || c.Drawing == nil || len(c.Drawing.Shapes) != 2 {
		t.Fatalf("a drawing without words: %+v %v", c, err)
	}
	if _, err := l.AddSiteComment("admin", anna, SiteCommentSpec{Board: "All.dc.html", X: 10, Y: 20}); err == nil {
		t.Error("an empty comment without a drawing was taken")
	}
}

func TestSiteShotAsks(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root, "p1", "Plan", otherV2)
	a := pagesAPI(accessLibrary(t, root))
	for _, body := range []string{`{"artifact":"p1","board":"","w":800,"h":600,"y":0}`, `{"artifact":"p1","board":"Main.dc.html","w":8,"h":600,"y":0}`, `{"artifact":"p1","board":"Main.dc.html","w":800,"h":600,"y":-1}`} {
		if status, _ := postJSON(a.SiteShotRoute, http.MethodPost, body); status != http.StatusBadRequest {
			t.Errorf("%s: %d", body, status)
		}
	}
}
