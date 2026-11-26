package design

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
)

// A drawing over a site board kept with a comment: the shapes the person drew, in the page's CSS pixels from
// its top-left, so it stands where it was drawn whatever the scroll, the zoom or the device.

// SiteShape is one shape: a stroke of the pen or the marker (the points it went through), an arrow or a frame (its two
// ends), or a label (its point and words).
type SiteShape struct {
	Kind  string       `json:"kind"`
	Color string       `json:"color"`
	W     float64      `json:"w"`
	Pts   [][2]float64 `json:"pts"`
	Text  string       `json:"text,omitempty"`
}

// SiteDrawing is the shapes of one drawing, in the order drawn.
type SiteDrawing struct {
	Shapes []SiteShape `json:"shapes"`
}

const (
	siteDrawingShapes = 200
	siteDrawingPoints = 20_000
	siteShapePoints   = 4_000
	siteLabelRunes    = 200
	siteDrawCoordMax  = 1_000_000
)

var (
	siteShapeKinds = map[string]int{"pen": 1, "marker": 1, "arrow": 2, "rect": 2, "label": 1}
	siteColorRe    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// siteDrawingOf reads a drawing sent by the page; nil when there is none or it is not one (a bad shape is dropped, too
// much of it is refused whole).
func siteDrawingOf(v any) (*SiteDrawing, bool) {
	if v == nil {
		return nil, true
	}
	o, ok := v.(*jsvalue.Object)
	if !ok {
		return nil, false
	}
	list, ok := Field(o, "shapes").([]any)
	if !ok || len(list) > siteDrawingShapes {
		return nil, false
	}
	out := &SiteDrawing{Shapes: []SiteShape{}}
	total := 0
	for _, raw := range list {
		m, isObj := raw.(*jsvalue.Object)
		if !isObj {
			continue
		}
		kind, _ := Field(m, "kind").(string)
		least, known := siteShapeKinds[kind]
		color, _ := Field(m, "color").(string)
		w, _ := Field(m, "w").(float64)
		pts, _ := Field(m, "pts").([]any)
		if !known || !siteColorRe.MatchString(color) || w <= 0 || w > 64 || len(pts) < least || len(pts) > siteShapePoints {
			continue
		}
		s := SiteShape{Kind: kind, Color: strings.ToLower(color), W: math.Round(w*10) / 10, Pts: make([][2]float64, 0, len(pts))}
		for _, p := range pts {
			xy, isList := p.([]any)
			if !isList || len(xy) != 2 {
				continue
			}
			x, okX := xy[0].(float64)
			y, okY := xy[1].(float64)
			if !okX || !okY || math.IsNaN(x) || math.IsNaN(y) || math.Abs(x) > siteDrawCoordMax || math.Abs(y) > siteDrawCoordMax {
				continue
			}
			s.Pts = append(s.Pts, [2]float64{math.Round(x*10) / 10, math.Round(y*10) / 10})
		}
		if len(s.Pts) < least {
			continue
		}
		if kind == "label" {
			text, _ := Field(m, "text").(string)
			text = strings.TrimSpace(cleanCommentText(text))
			if text == "" || utf8.RuneCountInString(text) > siteLabelRunes {
				continue
			}
			s.Text = text
		}
		total += len(s.Pts)
		if total > siteDrawingPoints {
			return nil, false
		}
		out.Shapes = append(out.Shapes, s)
	}
	if len(out.Shapes) == 0 {
		return nil, true
	}
	return out, true
}
