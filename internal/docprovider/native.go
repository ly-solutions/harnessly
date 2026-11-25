package docprovider

import (
	"context"
	"math"
	"strings"
)

// Native read representation: what a provider hands over when the pane cannot frame its viewer (the embed is blocked,
// there is no remote browser, no preview port). A document comes as Markdown or as sanitized structured blocks; a
// board as pages of scene objects (positions, text, connectors) with an authorized read of the images they cite.
// Every field is plain data: the runtime clamps it here and the page draws text as text, never as markup.

// Representations a document read may answer with.
const (
	ReprMarkdown    = "markdown"
	ReprStructured  = "structured"
	ReprBoard       = "board"
	ReprUnsupported = "unsupported"
)

// Limits of one native answer; what goes over is cut, not passed on.
const (
	MaxBlocks       = 5000
	MaxBlockText    = 64 << 10
	MaxMarkdown     = 2 << 20
	MaxSceneObjects = 500
	MaxSceneText    = 8 << 10
	MaxPoints       = 64
	MaxAsset        = 8 << 20
)

// Block is one piece of a structured document. Type is paragraph, heading (Level 1-6), quote, code, list (Items) or
// rule. Text is plain.
type Block struct {
	Type  string   `json:"type"`
	Level int      `json:"level,omitempty"`
	Text  string   `json:"text,omitempty"`
	Items []string `json:"items,omitempty"`
	Lang  string   `json:"lang,omitempty"`
}

// Point is a place on a board.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// SceneObject is one thing on a board. Type is shape, text, image, sticky or connector; a connector joins the objects
// From and To (their IDs) and may carry its own Points. Asset names an image to read through NativeAsset.
type SceneObject struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w,omitempty"`
	H      float64 `json:"h,omitempty"`
	Text   string  `json:"text,omitempty"`
	From   string  `json:"from,omitempty"`
	To     string  `json:"to,omitempty"`
	Points []Point `json:"points,omitempty"`
	Asset  string  `json:"asset,omitempty"`
}

// Scene is one page of a board's objects. Next is the cursor of the following page, "" at the end.
type Scene struct {
	Objects []SceneObject `json:"objects"`
	Next    string        `json:"next,omitempty"`
}

// Native is the answer of a native read of one object.
type Native struct {
	// Representation is one of the Repr constants.
	Representation string  `json:"representation"`
	Version        string  `json:"version,omitempty"`
	Markdown       string  `json:"markdown,omitempty"`
	Blocks         []Block `json:"blocks,omitempty"`
	Scene          *Scene  `json:"scene,omitempty"`
	// Assets: the board's images can be read (NativeAsset).
	Assets bool `json:"assets,omitempty"`
	// Reason says, for an unsupported representation, why in the provider's words.
	Reason string `json:"reason,omitempty"`
}

// Asset is an image of a board.
type Asset struct {
	ContentType string `json:"contentType"`
	// Data is the bytes, base64 on the wire.
	Data []byte `json:"data"`
}

// NativeReader is a Provider that can hand an object over without a frame. It is optional: a provider without it (the
// built-in catalog, an adapter that does not list `native`) answers every native read as unsupported and shows as before.
type NativeReader interface {
	// NativeRead reads a document ("" version is the current one) or, for a board, one page of its scene (cursor "").
	NativeRead(ctx context.Context, slug, version string, board bool, cursor string, limit int) (Native, error)
	// NativeAsset reads one image of a board with the person's own authorization at the provider.
	NativeAsset(ctx context.Context, slug, asset string) (Asset, error)
}

// Unsupported is the answer for a provider that has no native representation.
func Unsupported(reason string) Native {
	return Native{Representation: ReprUnsupported, Reason: reason}
}

var (
	blockTypes = map[string]bool{"paragraph": true, "heading": true, "quote": true, "code": true, "list": true, "rule": true}
	sceneTypes = map[string]bool{"shape": true, "text": true, "image": true, "sticky": true, "connector": true}
	assetTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}
)

// AssetTypeOK says whether an image type may be handed to the page (no SVG: it can carry script).
func AssetTypeOK(t string) bool { return assetTypes[strings.ToLower(strings.TrimSpace(t))] }

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 { // do not end inside a rune
		s = s[:len(s)-1]
	}
	return s
}

func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Max(-1e9, math.Min(1e9, v))
}

// Sanitize makes a native answer safe to hand to the page: unknown representations and object types are dropped or
// turned into unsupported, texts and counts are bounded, numbers are finite. It never invents content.
func Sanitize(n Native) Native {
	switch n.Representation {
	case ReprMarkdown:
		n.Markdown, n.Blocks, n.Scene = cut(n.Markdown, MaxMarkdown), nil, nil
	case ReprStructured:
		blocks := make([]Block, 0, len(n.Blocks))
		for _, b := range n.Blocks {
			if !blockTypes[b.Type] || len(blocks) >= MaxBlocks {
				continue
			}
			b.Text, b.Lang = cut(b.Text, MaxBlockText), cut(b.Lang, 32)
			if b.Type == "heading" {
				b.Level = min(max(b.Level, 1), 6)
			} else {
				b.Level = 0
			}
			if b.Type == "list" {
				items := make([]string, 0, len(b.Items))
				for _, it := range b.Items {
					items = append(items, cut(it, MaxBlockText))
				}
				b.Items = items
			} else {
				b.Items = nil
			}
			blocks = append(blocks, b)
		}
		n.Blocks, n.Markdown, n.Scene = blocks, "", nil
	case ReprBoard:
		sc := Scene{Objects: []SceneObject{}}
		if n.Scene != nil {
			sc.Next = cut(n.Scene.Next, 512)
			for _, o := range n.Scene.Objects {
				if !sceneTypes[o.Type] || o.ID == "" || len(sc.Objects) >= MaxSceneObjects {
					continue
				}
				o.ID, o.From, o.To, o.Asset = cut(o.ID, 128), cut(o.From, 128), cut(o.To, 128), cut(o.Asset, 256)
				o.X, o.Y, o.W, o.H = finite(o.X), finite(o.Y), math.Max(0, finite(o.W)), math.Max(0, finite(o.H))
				o.Text = cut(o.Text, MaxSceneText)
				if len(o.Points) > MaxPoints {
					o.Points = o.Points[:MaxPoints]
				}
				for i := range o.Points {
					o.Points[i] = Point{finite(o.Points[i].X), finite(o.Points[i].Y)}
				}
				sc.Objects = append(sc.Objects, o)
			}
		}
		n.Scene, n.Markdown, n.Blocks = &sc, "", nil
	default:
		return Unsupported(cut(n.Reason, 512))
	}
	n.Reason = ""
	return n
}
