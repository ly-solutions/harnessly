package agents

import "strings"

// subscriptionTokens start the tokens of a Claude subscription's sign-in: the access token the CLI's own login and
// `claude setup-token` give, and the token that renews it. A key of the API starts otherwise and is not one of them.
var subscriptionTokens = []string{"sk-ant-oat", "sk-ant-ort"}

// ClaudeSubscriptionToken says whether a value is the token of a Claude subscription's sign-in and not a key of the
// API. A machine has one sign-in by a subscription, the CLI's own: such a token is never taken as the key or a
// variable of a provider, which would be a second one. It is told by its shape, whatever address the provider has:
// an address may be a proxy's.
func ClaudeSubscriptionToken(v string) bool {
	v = strings.TrimSpace(v)
	for _, prefix := range subscriptionTokens {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	return false
}
