package design

import (
	"encoding/json"
	"testing"
)

// D5: a scene lasts at least 3 s, holds 2.1 s past its last moment, gives the reader 2.5 words a second, rounds to
// 100 ms and stays within 1..60 s.
func TestVideoAutoDur(t *testing.T) {
	for _, c := range []struct {
		last  int64
		words int
		want  int64
	}{
		{0, 0, 3000},
		{1800, 0, 3900},
		{1849, 0, 3900},
		{1851, 0, 4000},
		{0, 10, 4000},
		{0, 11, 4400},
		{90000, 0, 60000},
		{0, 1000, 60000},
	} {
		if got := VideoAutoDur(c.last, c.words); got != c.want {
			t.Errorf("VideoAutoDur(%d, %d) = %d, want %d", c.last, c.words, got, c.want)
		}
	}
}

func TestVideoFormats(t *testing.T) {
	for _, c := range []struct {
		f           VideoFormat
		draft       bool
		w, h        int
		slug, media string
	}{
		{VideoLandscape, false, 1920, 1080, "16x9", "video.mp4"},
		{VideoLandscape, true, 960, 540, "16x9", "video.mp4"},
		{VideoPortrait, false, 1080, 1920, "9x16", "video-9x16.mp4"},
		{VideoPortrait, true, 540, 960, "9x16", "video-9x16.mp4"},
	} {
		w, h := c.f.Size(c.draft)
		if w != c.w || h != c.h || c.f.Slug() != c.slug || VideoPublishedName(c.f) != c.media || !c.f.Valid() {
			t.Errorf("%s draft=%v: %dx%d %s %s", c.f, c.draft, w, h, c.f.Slug(), VideoPublishedName(c.f))
		}
	}
	if VideoFormat("4:3").Valid() {
		t.Error("4:3 is a format")
	}
}

// A new video is a deck folder with meta/video.json: the kind is told by the file, the settings come back as written,
// the formats in their fixed order, and the bounds are kept.
func TestInitVideoMakesAVideoOfADeck(t *testing.T) {
	g := newDeckRig(t)
	if g.l.IsVideo("admin") {
		t.Fatal("a plain project is a video")
	}
	if _, err := g.l.ReadVideoSettings("admin"); err == nil {
		t.Fatal("a plain project has video settings")
	}
	if err := g.l.InitDeck("admin", DeckInit{Minutes: 10, Tone: DeckToneTalk}); err != nil {
		t.Fatal(err)
	}
	thirty := 30
	if err := g.l.InitVideo("admin", VideoInit{Formats: []VideoFormat{VideoPortrait, VideoLandscape}, Seconds: &thirty, Subs: true}); err != nil {
		t.Fatal(err)
	}
	if !g.l.IsVideo("admin") || !g.l.IsDeck("admin") {
		t.Fatal("not a video and a deck")
	}
	s, err := g.l.ReadVideoSettings("admin")
	if err != nil {
		t.Fatal(err)
	}
	if s.V != 1 || len(s.Formats) != 2 || s.Formats[0] != VideoLandscape || s.FPS != 30 || *s.Seconds != 30 || s.Voice != VideoVoiceNone || !s.Subs {
		t.Fatalf("%+v", s)
	}
	if err := g.l.InitVideo("admin", VideoInit{}); err != nil {
		t.Fatal(err)
	}
	if s, _ := g.l.ReadVideoSettings("admin"); len(s.Formats) != 1 || s.Formats[0] != VideoLandscape || s.Seconds != nil || s.Subs {
		t.Fatalf("defaults: %+v", s)
	}
	ten, long := 10, 301
	for name, in := range map[string]VideoInit{
		"unknown format": {Formats: []VideoFormat{"4:3"}},
		"too short":      {Seconds: &ten},
		"too long":       {Seconds: &long},
	} {
		if err := g.l.InitVideo("admin", in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := ValidateVideoSettings(VideoSettings{V: 1, Formats: []VideoFormat{VideoLandscape, VideoLandscape}, FPS: 30, Voice: VideoVoiceNone}); err == nil {
		t.Error("a repeated format is accepted")
	}
	if err := g.l.InitVideo("nope", VideoInit{}); err == nil {
		t.Error("a video in a project that is not there")
	}
}

// A deck's comment keeps its old JSON; a video's carries its time.
func TestDeckCommentVideoFieldsAreAdditive(t *testing.T) {
	old, _ := json.Marshal(DeckComment{ID: "x", Page: "p"})
	var m map[string]any
	_ = json.Unmarshal(old, &m)
	for _, k := range []string{"view", "t0", "t1", "format", "block", "frame"} {
		if _, ok := m[k]; ok {
			t.Errorf("a deck's comment grew %q", k)
		}
	}
	t0 := int64(1200)
	raw, _ := json.Marshal(DeckComment{ID: "x", View: DeckCommentVideo, T0: &t0, Format: VideoPortrait, Block: "Заголовок"})
	var back DeckComment
	if err := json.Unmarshal(raw, &back); err != nil || back.View != "video" || *back.T0 != 1200 || back.T1 != nil || back.Format != VideoPortrait {
		t.Fatalf("%s %+v", raw, back)
	}
}
