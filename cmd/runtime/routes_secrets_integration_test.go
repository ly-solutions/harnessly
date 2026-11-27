package main

import (
	"net/http"
	"testing"
)

// The credentials the integrations keep in the safe are used by the runtime itself (reads, and acts after the person's
// yes): no command of an agent gets their values, by any spelling of the name.
func TestSafeKeepsIntegrationCredentialsFromCommands(t *testing.T) {
	rig := newSafeAccessRig(t)
	for _, name := range []string{"conn-c0123456789ab", "connector-cx0123456789ab", "mcp-m0123456789ab", "mcp-client-0123456789abcdef"} {
		rig.put(name, nil, nil)
		for _, ref := range []string{name, "local:" + name, "vault:me/" + name} {
			w := rig.call("POST", "/api/cli/secrets/use", map[string]any{"names": []string{ref}}, "a", nil)
			if w.Code == http.StatusOK {
				t.Errorf("use served %s", ref)
			}
			w = rig.call("POST", "/api/cli/secrets/file", map[string]any{"name": ref}, "a", nil)
			if w.Code == http.StatusOK {
				t.Errorf("file served %s", ref)
			}
		}
	}
	// The person's own secrets that merely start the same way stay theirs to use.
	rig.put("conn-string", nil, nil)
	if w := rig.call("POST", "/api/cli/secrets/use", map[string]any{"names": []string{"conn-string"}}, "a", nil); w.Code != http.StatusOK {
		t.Errorf("a person's secret: %d %s", w.Code, w.Body)
	}
}
