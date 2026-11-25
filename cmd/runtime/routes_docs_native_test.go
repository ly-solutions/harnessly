package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/docprovider"
	"github.com/ly-solutions/harnessly/internal/hub/ext"
	"github.com/ly-solutions/harnessly/internal/integrations"
)

// pinNativeAdapter declares an adapter that hands objects over natively: a Markdown document, a locked one, a board of
// two scene pages with an image, and a document in a representation the runtime does not know.
func pinNativeAdapter(t *testing.T, caps []string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var in struct {
			Slug   string `json:"slug"`
			Cursor string `json:"cursor"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/v1/describe":
			_ = json.NewEncoder(w).Encode(ext.Description{Protocol: docprovider.Protocol, Name: "example-docs", Version: "1", Capabilities: caps})
		case "/v1/get":
			kind := "artifact"
			if in.Slug == "Board1" {
				kind = "board"
			}
			_, _ = io.WriteString(w, `{"slug":"`+in.Slug+`","kind":"`+kind+`","title":"T","url":"https://docs.example.com/d/`+in.Slug+`"}`)
		case "/v1/native":
			switch in.Slug {
			case "Locked":
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"error":"denied"}`)
			case "Weird":
				_, _ = io.WriteString(w, `{"representation":"hologram","markdown":"x"}`)
			default:
				_, _ = io.WriteString(w, `{"representation":"markdown","markdown":"# Plan\n\nBody text <script>x</script>","version":"v2"}`)
			}
		case "/v1/native/board":
			if in.Cursor == "" {
				_, _ = io.WriteString(w, `{"representation":"board","assets":true,"scene":{"objects":[{"id":"a","type":"sticky","x":10,"y":20,"w":100,"h":50,"text":"Hello"},{"id":"bad","type":"script","x":1,"y":1},{"id":"i","type":"image","x":0,"y":0,"w":10,"h":10,"asset":"img1"}],"next":"p2"}}`)
			} else {
				_, _ = io.WriteString(w, `{"representation":"board","scene":{"objects":[{"id":"c","type":"connector","x":0,"y":0,"from":"a","to":"i","points":[{"x":1,"y":2},{"x":3,"y":4}]}]}}`)
			}
		case "/v1/native/asset":
			_, _ = io.WriteString(w, `{"contentType":"image/png","data":"iVBORw0KGgo="}`)
		default:
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = io.WriteString(w, `{"error":"not_supported"}`)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := ext.NewClient(ext.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	p, err := docprovider.Register(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(docprovider.Pin(p))
}

func TestNativeReadThroughAnAdapter(t *testing.T) {
	pinNativeAdapter(t, []string{"read", "native", "native-assets"})
	d := integrationDeps(false)
	d.DataDir = t.TempDir()
	routes := docsRoutes(d, integrations.Config{})
	page := http.Header{"Cookie": {"session=ok"}}
	get := func(path string) (int, string) {
		return serveRoutes(routes, "GET", platformAPI+"/docs/"+docprovider.Alias(path), "", page)
	}
	native := func(id, q string) (int, string) {
		return serveRoutes(routes, "GET", platformAPI+"/docs/"+docprovider.Alias(id)+"/native"+q, "", page)
	}

	if code, body := get("Plan1"); code != 200 || !strings.Contains(body, `"native":true`) {
		t.Fatalf("summary tells the page it can read natively: %d %s", code, body)
	}
	if code, body := native("Plan1", ""); code != 200 || !strings.Contains(body, `"representation":"markdown"`) || !strings.Contains(body, "Body text") {
		t.Fatalf("document: %d %s", code, body)
	}
	if code, body := native("Weird", ""); code != 200 || !strings.Contains(body, `"representation":"unsupported"`) || strings.Contains(body, "markdown") {
		t.Fatalf("unknown representation must be explicit: %d %s", code, body)
	}
	code, body := native("Locked", "")
	if code != 403 || !strings.Contains(body, `"code":"denied"`) {
		t.Fatalf("denial is about the object: %d %s", code, body)
	}

	code, body = native("Board1", "")
	if code != 200 || !strings.Contains(body, `"next":"p2"`) || !strings.Contains(body, `"text":"Hello"`) || strings.Contains(body, "script") || !strings.Contains(body, `"assets":true`) {
		t.Fatalf("board page 1 (unknown object type dropped): %d %s", code, body)
	}
	if code, body = native("Board1", "?cursor=p2"); code != 200 || !strings.Contains(body, `"from":"a"`) || strings.Contains(body, `"next"`) {
		t.Fatalf("board page 2: %d %s", code, body)
	}
	if code, body = serveRoutes(routes, "GET", platformAPI+"/docs/"+docprovider.Alias("Board1")+"/native/asset?id=img1", "", page); code != 200 || !strings.HasPrefix(body, "\x89PNG") {
		t.Fatalf("asset: %d %q", code, body)
	}
	if code, _ := serveRoutes(routes, "GET", platformAPI+"/docs/"+docprovider.Alias("Board1")+"/native", "", http.Header{}); code != 401 {
		t.Fatalf("unauthenticated: %d", code)
	}
}

func TestNativeReadUnsupportedWithoutCapability(t *testing.T) {
	pinNativeAdapter(t, []string{"read"})
	d := integrationDeps(false)
	d.DataDir = t.TempDir()
	routes := docsRoutes(d, integrations.Config{})
	page := http.Header{"Cookie": {"session=ok"}}
	slug := docprovider.Alias("Plan1")
	if code, body := serveRoutes(routes, "GET", platformAPI+"/docs/"+slug, "", page); code != 200 || strings.Contains(body, `"native"`) {
		t.Fatalf("a read-only adapter shows as before: %d %s", code, body)
	}
	if code, body := serveRoutes(routes, "GET", platformAPI+"/docs/"+slug+"/native", "", page); code != 200 || !strings.Contains(body, `"representation":"unsupported"`) {
		t.Fatalf("unsupported must be explicit: %d %s", code, body)
	}
}
