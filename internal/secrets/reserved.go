package secrets

import (
	"regexp"
	"strings"
)

// ReasonIntegration refuses a command the credentials of an integration.
const ReasonIntegration = "integration"

// integrationCredential are the names the runtime itself gives the credentials it keeps for the person's integrations:
// conn-<id> (connections), connector-<id> (connectors), mcp-<id> and mcp-client-<hash> (MCP servers). The ids are the
// runtime's random ones, so a secret the person named, such as conn-string, is not one of them.
var integrationCredential = regexp.MustCompile(`^(conn-c[0-9a-f]{12}|connector-cx[0-9a-f]{12}|mcp-m[0-9a-f]{12}|mcp-client-[0-9a-f]{16})$`)

// IntegrationCredential says whether a name or reference points at an integration's credentials: the runtime uses them
// itself, behind its own confirmations, and gives their values to no command. A vault: reference counts by the last
// segment of its path, which is how such a record is reached on a server.
func IntegrationCredential(ref string) bool {
	r, ok := ParseRef(ref)
	if !ok {
		return false
	}
	name := r.Name
	if r.Kind == WhereVault {
		name = r.Path[strings.LastIndex(r.Path, "/")+1:]
	}
	return integrationCredential.MatchString(name)
}
