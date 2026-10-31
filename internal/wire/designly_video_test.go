package wire

import (
	"encoding/json"
	"strings"
	"testing"
)

// A new video's create body: its formats, length and subtitles are checked by Go and by the schema alike, and the field
// stays absent for a page that does not know videos.
func TestDesignlyCreateCarriesAVideo(t *testing.T) {
	sc := mustSchema(t)
	def := sc.defs["DesignlyCreate"].(map[string]any)
	thirty := 30
	good := DesignlyCreate{Title: "Acme teaser", Brief: "Thirty seconds on checkout", Kind: DesignVideo, Provider: ProviderClaude,
		Video: &DesignlyVideo{Formats: []DesignVideoFormat{"16:9", "9:16"}, Seconds: &thirty, Subs: true}}
	free := good
	free.Video = &DesignlyVideo{Formats: []DesignVideoFormat{"9:16"}}
	bare := DesignlyCreate{Title: "Acme teaser", Brief: "Teaser", Kind: DesignVideo, Provider: ProviderClaude}
	for name, c := range map[string]DesignlyCreate{"video": good, "free length": free, "no settings": bare} {
		if err := c.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		raw, _ := json.Marshal(c)
		v, err := decodeNumbers(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := sc.check(def, v, "$"); err != nil {
			t.Fatalf("%s: schema: %v", name, err)
		}
		var back DesignlyCreate
		if err := decodeStrict(raw, &back); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if name == "no settings" && strings.Contains(string(raw), `"video":`) {
			t.Fatalf("a body without settings grew them: %s", raw)
		}
	}
	ten, long := 10, 301
	bad := map[string]func(c *DesignlyCreate){
		"video on a deck": func(c *DesignlyCreate) { c.Kind = DesignSlides },
		"no format":       func(c *DesignlyCreate) { c.Video.Formats = nil },
		"unknown format":  func(c *DesignlyCreate) { c.Video.Formats = []DesignVideoFormat{"4:3"} },
		"repeated format": func(c *DesignlyCreate) { c.Video.Formats = []DesignVideoFormat{"16:9", "16:9"} },
		"three formats":   func(c *DesignlyCreate) { c.Video.Formats = []DesignVideoFormat{"16:9", "9:16", "16:9"} },
		"too short":       func(c *DesignlyCreate) { c.Video.Seconds = &ten },
		"too long":        func(c *DesignlyCreate) { c.Video.Seconds = &long },
		"deck on a video": func(c *DesignlyCreate) { c.Deck = &DesignlyDeck{Tone: "talk"} },
	}
	for name, spoil := range bad {
		c := good
		video := *good.Video
		video.Formats = append([]DesignVideoFormat{}, good.Video.Formats...)
		c.Video = &video
		spoil(&c)
		if c.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// The schema refuses what Go refuses where it can say so.
	for name, f := range map[string][]DesignVideoFormat{"none": {}, "unknown": {"4:3"}} {
		c := good
		c.Video = &DesignlyVideo{Formats: f}
		raw, _ := json.Marshal(c)
		v, _ := decodeNumbers(raw)
		if sc.check(def, v, "$") == nil {
			t.Errorf("schema accepts formats %s", name)
		}
	}
}
