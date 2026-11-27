package extproviders

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeServer answers like a model server: its models on /v1/models and, when messages is set, the Anthropic API's
// error for a model it does not have on /v1/messages.
func fakeServer(t *testing.T, models string, messages bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(models))
	})
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if !messages {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"not_found_error","message":"model not found"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDiscoverFindsServersAndSkipsTheSilentOnes(t *testing.T) {
	ollama := fakeServer(t, `{"object":"list","data":[{"id":"qwen3-coder:30b"},{"id":"llama3.2:3b"}]}`, true)
	vllm := fakeServer(t, `{"data":[{"id":"Qwen/Qwen3-Coder-30B","max_model_len":65536}]}`, false)
	closed := httptest.NewServer(http.NotFoundHandler())
	gone := closed.URL
	closed.Close()
	d := &Discoverer{Targets: []LocalTarget{
		{Kind: "ollama", Name: "Ollama", Root: ollama.URL},
		{Kind: "lmstudio", Name: "LM Studio", Root: gone},
		{Kind: "vllm", Name: "vLLM", Root: vllm.URL},
	}}
	got, err := d.Discover(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 2 || got.Custom != nil {
		t.Fatalf("found: %+v", got)
	}
	o, v := got.Servers[0], got.Servers[1]
	// A server that speaks the Anthropic API is reached at its root; any other through the proxy, at its /v1.
	if o.Kind != "ollama" || o.API != APIAnthropic || o.BaseURL != ollama.URL || len(o.Models) != 2 || o.Models[0].ID != "llama3.2:3b" {
		t.Fatalf("ollama: %+v", o)
	}
	if v.Kind != "vllm" || v.API != APIOpenAI || v.BaseURL != vllm.URL+"/v1" || len(v.Models) != 1 || v.Models[0].Window != 65536 {
		t.Fatalf("vllm: %+v", v)
	}
}

func TestDiscoverTakesAnAddressOfThePersonOnlyWhenItIsLocal(t *testing.T) {
	srv := fakeServer(t, `{"data":[{"id":"m"}]}`, false)
	d := &Discoverer{Targets: []LocalTarget{}}
	got, err := d.Discover(context.Background(), srv.URL+"/v1/")
	if err != nil || len(got.Servers) != 1 || got.Servers[0].Kind != "custom" || got.Custom == nil || !got.Custom.Found || got.Custom.Address != srv.URL {
		t.Fatalf("a typed address: %+v, %v", got, err)
	}
	for _, bad := range []string{"https://api.openai.com/v1", "http://8.8.8.8:11434", "http://[2606:4700::1111]/", "ftp://127.0.0.1", "http://user:pw@127.0.0.1"} {
		if _, err := d.Discover(context.Background(), bad); err == nil {
			t.Errorf("%s: a public or malformed address is no local model", bad)
		}
	}
	if _, err := d.Discover(context.Background(), "http://8.8.8.8"); ReasonOf(err) != ReasonNotLocal {
		t.Errorf("reason: %v", err)
	}
	for _, ok := range []string{"http://localhost:11434", "http://192.168.1.20:8080", "http://10.0.0.5:8000/v1", "http://100.100.10.7:1234", "http://gpu-box:11434", "http://[::1]:8080"} {
		if _, err := CleanLocalURL(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
}

// The default client connects only to a private address, whatever the name says.
func TestLocalClientRefusesAPublicAddress(t *testing.T) {
	_, err := localClient().Get("http://8.8.8.8:80/")
	if err == nil || !strings.Contains(err.Error(), errNotLocal.Error()) {
		t.Fatalf("a public address was dialled: %v", err)
	}
}

func TestLocalAccountTakesNoKeyAndOnlyALocalAddress(t *testing.T) {
	svc, sec := newService(t)
	view, err := svc.Create(Input{Name: LocalName("Ollama"), BaseURL: "http://127.0.0.1:11434", API: APIAnthropic, Preset: PresetLocal, Key: "ignored",
		Models: []Model{{ID: "qwen3-coder:30b"}}})
	if err != nil {
		t.Fatal(err)
	}
	if view.HasKey || len(sec) != 0 || view.Preset != PresetLocal || !view.ClaudeCLI || view.Proxied {
		t.Fatalf("view: %+v, safe: %v", view, sec)
	}
	if _, err := svc.Create(Input{Name: "Not local", BaseURL: "https://api.openai.com/v1", API: APIOpenAI, Preset: PresetLocal}); ReasonOf(err) != ReasonNotLocal {
		t.Fatalf("a local model at a public address: %v", err)
	}
	away := "https://api.openai.com/v1"
	if _, err := svc.Update(view.ID, Patch{BaseURL: &away}); ReasonOf(err) != ReasonNotLocal {
		t.Fatalf("a local model moved to a public address: %v", err)
	}
}

// An Ollama says what its models run with: the window a chat is told is the loaded context, else the model's own limit
// up to LocalWindow, and a loaded context is told to the person.
func TestDiscoverTakesTheWindowsOfAnOllama(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"loaded:1b"},{"id":"big:30b"},{"id":"small:1b"}]}`))
	})
	mux.HandleFunc("GET /api/ps", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"loaded:1b","context_length":4096}]}`))
	})
	mux.HandleFunc("POST /api/show", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		limit := map[string]int{"big:30b": 262144, "small:1b": 8192}[in.Model]
		_, _ = fmt.Fprintf(w, `{"model_info":{"general.architecture":"x","x.context_length":%d}}`, limit)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	got, err := (&Discoverer{Targets: []LocalTarget{{Kind: "ollama", Name: "Ollama", Root: srv.URL}}}).Discover(context.Background(), "")
	if err != nil || len(got.Servers) != 1 {
		t.Fatalf("found: %+v, %v", got, err)
	}
	s := got.Servers[0]
	windows := map[string]int{}
	for _, m := range s.Models {
		windows[m.ID] = m.Window
	}
	if windows["loaded:1b"] != 4096 || windows["big:30b"] != LocalWindow || windows["small:1b"] != 8192 || s.Context != 4096 {
		t.Fatalf("windows %v, context %d", windows, s.Context)
	}
}
