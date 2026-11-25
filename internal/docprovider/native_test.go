package docprovider

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestSanitizeBoundsAndDrops(t *testing.T) {
	got := Sanitize(Native{Representation: ReprStructured, Blocks: []Block{
		{Type: "heading", Level: 99, Text: "H", Items: []string{"x"}},
		{Type: "iframe", Text: "bad"},
		{Type: "list", Items: []string{"a", strings.Repeat("z", MaxBlockText+10)}},
	}})
	if len(got.Blocks) != 2 || got.Blocks[0].Level != 6 || got.Blocks[0].Items != nil || len(got.Blocks[1].Items[1]) != MaxBlockText {
		t.Fatalf("blocks: %+v", got.Blocks)
	}
	scene := Sanitize(Native{Representation: ReprBoard, Scene: &Scene{Objects: []SceneObject{
		{ID: "a", Type: "shape", X: math.NaN(), W: -5}, {ID: "", Type: "shape"}, {ID: "b", Type: "html"},
	}}})
	if len(scene.Scene.Objects) != 1 || scene.Scene.Objects[0].X != 0 || scene.Scene.Objects[0].W != 0 {
		t.Fatalf("scene: %+v", scene.Scene)
	}
	if u := Sanitize(Native{Representation: "hologram", Markdown: "x", Reason: "r"}); u.Representation != ReprUnsupported || u.Markdown != "" {
		t.Fatalf("unknown: %+v", u)
	}
	if AssetTypeOK("image/svg+xml") || !AssetTypeOK("IMAGE/PNG") {
		t.Fatal("asset types")
	}
}

func TestExtProviderNativeUnsupportedAndDenied(t *testing.T) {
	plain := adapterOf(t, []string{"read"}, 0, nil)
	if n, err := plain.NativeRead(context.Background(), "plan", "", false, "", 10); err != nil || n.Representation != ReprUnsupported {
		t.Fatalf("no capability: %+v %v", n, err)
	}
	if plain.Capabilities().Native {
		t.Fatal("capability claimed")
	}
	p := adapterOf(t, []string{"read", "native"}, 0, map[string]http.HandlerFunc{
		"/v1/native": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) },
	})
	_, err := p.NativeRead(context.Background(), "plan", "", false, "", 10)
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("denial: %v", err)
	}
}
