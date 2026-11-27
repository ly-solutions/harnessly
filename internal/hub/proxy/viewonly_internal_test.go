package proxy

import "testing"

func TestRuntimePath(t *testing.T) {
	for in, want := range map[string]string{
		"/m/abc/api/timeline":        "/api/timeline",
		"/m/abc/agents/api/timeline": "/api/timeline",
		"/m/abc/agentsx":             "/agentsx",
		"/api/status":                "/api/status",
	} {
		if got := runtimePath(in, "abc"); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
