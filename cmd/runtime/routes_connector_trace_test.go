package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ly-solutions/harnessly/internal/connectors"
)

// The trace is the person's: a page session reads it, nothing else; it is written only while the settings say so.
func TestConnectorTraceRouteIsThePagesAndFollowsTheSetting(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfg, []byte(`{"developerMode":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	session := false
	d := deps{DataDir: dir, ConfigFile: cfg, SessionOK: func(*http.Request) bool { return session }}
	routes := connectorTraceRoutes(d)
	defer connectors.Trace.SetEnabled(nil)
	get := func(path string) (int, map[string]json.RawMessage) {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		for _, rt := range routes {
			if rt.Match(r) {
				w := httptest.NewRecorder()
				rt.Handler.ServeHTTP(w, r)
				var out map[string]json.RawMessage
				_ = json.Unmarshal(w.Body.Bytes(), &out)
				return w.Code, out
			}
		}
		t.Fatalf("no route for %s", path)
		return 0, nil
	}
	if code, _ := get(platformAPI + "/connectors/trace"); code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d", code)
	}
	session = true
	connectors.Trace.Add("cx-test", "poll", connectors.TraceInfo, "poll: 1 new", map[string]int{"events": 1})
	code, out := get(platformAPI + "/connectors/trace?conn=cx-test")
	if code != http.StatusOK || string(out["on"]) != "true" {
		t.Fatalf("with a session: %d %s", code, out["on"])
	}
	var entries []connectors.TraceEntry
	if err := json.Unmarshal(out["entries"], &entries); err != nil || len(entries) == 0 || entries[len(entries)-1].Message != "poll: 1 new" {
		t.Fatalf("entries: %s %v", out["entries"], err)
	}
}
