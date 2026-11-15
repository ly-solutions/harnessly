package share

import "testing"

// a hub's own address of the links is taken beside the share gateway's; anything else leaves links off.
func TestParseShareBaseTakesAHubsAddress(t *testing.T) {
	for _, ok := range []string{
		"https://share.example.org/share/abcDEF123_-x/",
		"https://hub.example.org/agents/api/acme/chat-shares/",
		"https://hub.example.org:8443/base/agents/api/acme/chat-shares/",
		"http://127.0.0.1:8080/agents/api/acme/chat-shares/",
	} {
		if ParseShareBase(ok) != ok {
			t.Errorf("%q is a link prefix", ok)
		}
	}
	for _, bad := range []string{
		"", "https://hub.example.org/chat-shares", "https://hub.example.org/a b/chat-shares/",
		"https://hub.example.org/agents/../chat-shares/", "javascript://x/chat-shares/", "https://hub.example.org/chat-shares/?x=1",
		"https://user@hub.example.org/chat-shares/", "https://HUB.example.org/chat-shares/",
	} {
		if ParseShareBase(bad) != "" {
			t.Errorf("%q must not be a link prefix", bad)
		}
	}
}
