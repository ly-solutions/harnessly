package design

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type deckIndex struct {
	Kind *string `json:"kind"`
	Deck *struct {
		Type  string `json:"type"`
		Video *struct {
			Chapters []DeckChapter `json:"chapters"`
			Formats  []VideoFormat `json:"formats"`
			DurMs    *int64        `json:"durMs"`
		} `json:"video"`
		PDF bool `json:"pdf"`
	} `json:"deck"`
}

func readDeckIndex(t *testing.T, a *API, token string) deckIndex {
	t.Helper()
	w := publicGet(a, "GET", "p1", token)
	if w.Code != 200 {
		t.Fatalf("index: %d", w.Code)
	}
	var got deckIndex
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPublicIndexOfAnOrdinaryDesignHasNoDeck(t *testing.T) {
	a, _ := scopeAPI(t)
	got := readDeckIndex(t, a, scopeToken(t, nil))
	if got.Kind != nil || got.Deck != nil {
		t.Fatalf("kind %v deck %v", got.Kind, got.Deck)
	}
	if w := publicGet(a, "GET", "p1/__media/video.mp4", scopeToken(t, nil)); w.Code != 404 {
		t.Fatalf("media of a design: %d", w.Code)
	}
}

func TestPublicDeckIndexAndMedia(t *testing.T) {
	a, root := scopeAPI(t)
	writeFile(t, filepath.Join(root, "p1", DeckSettingsFile), `{"v":1}`)
	token := scopeToken(t, nil)

	got := readDeckIndex(t, a, token)
	if got.Kind == nil || *got.Kind != "slides" || got.Deck == nil || got.Deck.Video != nil || got.Deck.PDF {
		t.Fatalf("nothing published yet: %+v", got)
	}
	if w := publicGet(a, "GET", "p1/__media/video.mp4", token); w.Code != 404 {
		t.Fatalf("not published: %d", w.Code)
	}

	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "video.mp4"), "0123456789")
	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "deck.pdf"), "%PDF-1")
	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "chapters.json"), `[{"page":"A","title":"One","t":0},{"page":"","title":"x","t":5},{"page":"B","title":"Two","t":4000}]`)
	writeFile(t, filepath.Join(root, "p1", DeckMediaDir, "rec", "r1", "av.webm"), "secret")

	got = readDeckIndex(t, a, token)
	if got.Deck == nil || got.Deck.Video == nil || !got.Deck.PDF || len(got.Deck.Video.Chapters) != 2 || got.Deck.Video.Chapters[1].T != 4000 {
		t.Fatalf("published: %+v", got.Deck)
	}

	w := publicRequest(a, "GET", "p1/__media/video.mp4", token, http.Header{"Range": {"bytes=2-5"}})
	if w.Code != 206 || w.Body.String() != "2345" || w.Header().Get("Content-Range") != "bytes 2-5/10" || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("range: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	w = publicRequest(a, "GET", "p1/__media/video.mp4", token, http.Header{"Range": {"bytes=50-60"}})
	if w.Code != 416 {
		t.Fatalf("out of range: %d", w.Code)
	}
	if w := publicGet(a, "HEAD", "p1/__media/deck.pdf", token); w.Code != 200 || w.Header().Get("Content-Length") != "6" {
		t.Fatalf("pdf head: %d %v", w.Code, w.Header())
	}
	for _, name := range []string{"av.webm", "chapters.json", "../rec/r1/av.webm", "video.mp4/x"} {
		if w := publicGet(a, "GET", "p1/__media/"+name, token); w.Code != 404 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if w := publicGet(a, "GET", "p1/.media/published/video.mp4", token); w.Code != 404 {
		t.Errorf("hidden path: %d", w.Code)
	}
}

func TestPublicDeckMediaHonoursTheScope(t *testing.T) {
	a, root := scopeAPI(t)
	writeFile(t, filepath.Join(root, "p1", DeckSettingsFile), `{"v":1}`)
	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "video.mp4"), "0123456789")
	for name, claims := range map[string]map[string]any{
		"some pages": {"pages": []any{"One.dc.html"}},
		"no pages":   {"pages": []any{}},
	} {
		token := scopeToken(t, claims)
		got := readDeckIndex(t, a, token)
		if got.Kind == nil || got.Deck != nil {
			t.Errorf("%s: kind %v deck %v", name, got.Kind, got.Deck)
		}
		if w := publicGet(a, "GET", "p1/__media/video.mp4", token); w.Code != 404 {
			t.Errorf("%s: media %d", name, w.Code)
		}
	}
	if w := publicGet(a, "GET", "p1/__media/video.mp4", scopeToken(t, nil)); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "0123") {
		t.Errorf("whole link: %d", w.Code)
	}
	if w := publicGet(a, "GET", "p1/__media/video.mp4", "forged.token.x"); w.Code != 401 {
		t.Errorf("forged: %d", w.Code)
	}
}

func videoProject(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "p1", DeckSettingsFile), `{"v":1}`)
	writeFile(t, filepath.Join(root, "p1", VideoSettingsFile), `{"v":1,"formats":["16:9","9:16"],"fps":30,"seconds":null,"voice":"none","subs":true}`)
}

func TestPublicVideoIndexAndMedia(t *testing.T) {
	a, root := scopeAPI(t)
	videoProject(t, root)
	token := scopeToken(t, nil)

	got := readDeckIndex(t, a, token)
	if got.Kind == nil || *got.Kind != "slides" || got.Deck == nil || got.Deck.Type != "video" || got.Deck.Video != nil {
		t.Fatalf("nothing published yet: %+v", got.Deck)
	}

	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "video-9x16.mp4"), "0123456789")
	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "chapters.json"), `[{"page":"A","title":"One","t":0}]`)
	writeFile(t, filepath.Join(root, "p1", VideoPublishedFile), `{"formats":{"16:9":{"key":"k","durMs":9000,"at":"x"},"9:16":{"key":"k","durMs":8000,"at":"x"}}}`)
	got = readDeckIndex(t, a, token)
	v := got.Deck.Video
	if v == nil || len(v.Formats) != 1 || v.Formats[0] != VideoPortrait || v.DurMs == nil || *v.DurMs != 8000 || len(v.Chapters) != 1 {
		t.Fatalf("only 9:16 published: %+v", v)
	}

	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "video.mp4"), "abcdefghij")
	v = readDeckIndex(t, a, token).Deck.Video
	if len(v.Formats) != 2 || v.Formats[0] != VideoLandscape || v.Formats[1] != VideoPortrait || *v.DurMs != 9000 {
		t.Fatalf("both published: %+v", v)
	}

	w := publicRequest(a, "GET", "p1/__media/video-9x16.mp4", token, http.Header{"Range": {"bytes=2-5"}})
	if w.Code != 206 || w.Body.String() != "2345" || w.Header().Get("Content-Range") != "bytes 2-5/10" || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("range: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if w := publicGet(a, "GET", "p1/__media/video-16x9.mp4", token); w.Code != 404 {
		t.Errorf("unknown name: %d", w.Code)
	}
}

func TestPublicVideoBrokenMetaIsANullLength(t *testing.T) {
	a, root := scopeAPI(t)
	videoProject(t, root)
	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "video.mp4"), "0123456789")
	writeFile(t, filepath.Join(root, "p1", VideoPublishedFile), `{nope`)
	v := readDeckIndex(t, a, scopeToken(t, nil)).Deck.Video
	if v == nil || v.DurMs != nil || len(v.Formats) != 1 || v.Chapters == nil {
		t.Fatalf("%+v", v)
	}
}

func TestPublicVideoHonoursTheScope(t *testing.T) {
	a, root := scopeAPI(t)
	videoProject(t, root)
	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "video-9x16.mp4"), "0123456789")
	for name, claims := range map[string]map[string]any{
		"some pages": {"pages": []any{"One.dc.html"}},
		"no pages":   {"pages": []any{}},
	} {
		token := scopeToken(t, claims)
		if got := readDeckIndex(t, a, token); got.Deck != nil {
			t.Errorf("%s: deck %+v", name, got.Deck)
		}
		if w := publicGet(a, "GET", "p1/__media/video-9x16.mp4", token); w.Code != 404 {
			t.Errorf("%s: media %d", name, w.Code)
		}
	}
}

func TestPublicDeckIndexSaysItsType(t *testing.T) {
	a, root := scopeAPI(t)
	writeFile(t, filepath.Join(root, "p1", DeckSettingsFile), `{"v":1}`)
	writeFile(t, filepath.Join(root, "p1", DeckPublishedDir, "video.mp4"), "0123456789")
	got := readDeckIndex(t, a, scopeToken(t, nil))
	if got.Deck.Type != "slides" || got.Deck.Video == nil || got.Deck.Video.Formats == nil || len(got.Deck.Video.Formats) != 0 || got.Deck.Video.DurMs != nil {
		t.Fatalf("%+v", got.Deck)
	}
}
