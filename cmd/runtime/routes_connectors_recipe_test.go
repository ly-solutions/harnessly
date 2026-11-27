package main

import (
	"context"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/mcpconn"
)

// A poll recipe calls a server signed in with a key through the runtime's MCP service, credentials and all: a service
// without the safe panicked on the first call.
func TestRecipeMCPCallsASignedInServer(t *testing.T) {
	ts, calls := fakeLinear(t, map[string]string{"Bearer tok": "sergey"})
	data := t.TempDir()
	d := deps{DataDir: data, Home: t.TempDir(), CLI: &gate.CLIToken{DataDir: data, Headers: []string{"x-cli-token"}}}
	sf := safeFor(d)
	t.Cleanup(sf.files.Close)
	srv, _, err := mcpFor(d).Add(context.Background(), mcpconn.AddInput{URL: ts.URL + "/mcp", CatalogID: "linear",
		Header: &mcpconn.HeaderInput{Name: "Authorization", Value: "Bearer tok"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := recipeMCP(d).CallTool(context.Background(), srv.ID, "list_issues", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "hello sergey") || len(*calls) != 1 {
		t.Fatalf("answer %s, calls %v", raw, *calls)
	}
}
