package hubproto

import "encoding/json"

// A change of the organization's environment an agent proposes from a machine. The link of a machine proves
// the machine, not a person, so the hub changes nothing by it: it keeps the proposal, and an administrator of the
// organization applies or declines it in the hub under their own session. No value of a secret is carried: a key and a
// secret variable are named.

const (
	// EventEnvPropose carries {"event":"env.propose", …EnvProposal}; the hub answers each with OpEnvProposed.
	EventEnvPropose = "env.propose"
	// OpEnvProposed is the hub's answer: an EnvProposalResult.
	OpEnvProposed = "env.proposed"
	// FeatureEnvPropose is in `hello` when the agent sends proposals and takes their answers.
	FeatureEnvPropose = "env.propose"
)

// EnvProposal is one proposed change. Check asks only whether it would be kept (its shape), keeping nothing: the agent
// is told a refusal in words before the person is asked to confirm anything.
type EnvProposal struct {
	RequestID string `json:"requestId"`
	// Conv and ConvTitle are the chat the proposal comes from: the administrator sees them, and so does the journal.
	Conv      string              `json:"conv,omitempty"`
	ConvTitle string              `json:"convTitle,omitempty"`
	Check     bool                `json:"check,omitempty"`
	Provider  EnvProviderProposal `json:"provider"`
}

// EnvProviderProposal is a model provider of the organization's layer, found by its name: added when the catalog has
// none of that name, changed when it has, taken out of the environment when Remove.
type EnvProviderProposal struct {
	Name   string          `json:"name"`
	Hint   string          `json:"hint,omitempty"`
	Remove bool            `json:"remove,omitempty"`
	Source json.RawMessage `json:"source,omitempty"`
}

// EnvProposalResult is the hub's answer. Status is an HTTP-like code: 202 kept and waits for an administrator, 200 a
// check that found nothing wrong, 400 not valid; Message says it in words for the person and the agent.
type EnvProposalResult struct {
	RequestID string `json:"requestId"`
	Status    int    `json:"status"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	// ProposalID is the proposal that waits.
	ProposalID string `json:"proposalId,omitempty"`
}
