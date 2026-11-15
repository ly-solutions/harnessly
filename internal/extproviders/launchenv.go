package extproviders

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/ly-solutions/harnessly/internal/agents"
)

// The variables a provider adds to the launch of an agent on it: the base ones of the agent's CLI the person typed by
// hand (ANTHROPIC_BASE_URL, ANTHROPIC_MODEL, ANTHROPIC_SMALL_FAST_MODEL, a token, the paths of certificates) and any
// others. A variable that is not here is left as the launch makes it; one that is here always replaces it. The value
// of a secret variable lives in the machine's safe: the registry keeps the name of the secret, an answer says only
// that it is set.

// Limits of a provider's variables.
const (
	MaxEnv      = 40
	MaxEnvValue = 4096
)

// Reasons of a refusal of a variable.
const (
	// ReasonBadEnv: a name that is no identifier, a value too long or of several lines, a name twice, too many, or a
	// secret variable without a value.
	ReasonBadEnv = "bad-env"
	// ReasonEnvReserved: the variable is the launch's own and is not the person's to set.
	ReasonEnvReserved = "env-reserved"
	// ReasonSubscriptionToken: the key or the value is the token of a Claude subscription's sign-in. A machine has one
	// such sign-in, the CLI's own; a provider takes a key of the API.
	ReasonSubscriptionToken = "subscription-token"
)

// EnvVar is one variable as the registry file keeps it: the value of a plain one, the name of the safe's secret of a
// secret one.
type EnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret,omitempty"`
	Ref    string `json:"ref,omitempty"`
}

// EnvView is a variable as the page sees it: a secret one never has its value, only whether the safe holds it.
type EnvView struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret"`
	Set    bool   `json:"set"`
}

// EnvInput is a variable of a patch. A secret one without a Value keeps the value the safe has.
type EnvInput struct {
	Name   string  `json:"name"`
	Value  *string `json:"value"`
	Secret bool    `json:"secret"`
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// reservedEnv are the variables of the shell, the pane and the agent's own home a launch depends on.
var reservedEnv = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL", "PWD", "OLDPWD", "TERM", "TMPDIR", "IFS", "ENV", "BASH_ENV",
	"PROMPT_COMMAND", "NODE_OPTIONS", "NODE_PATH", "LD_PRELOAD", "LD_LIBRARY_PATH", "LD_AUDIT", "SSH_AUTH_SOCK", "SSH_ASKPASS",
	"EDITOR", "VISUAL", "PAGER", "BROWSER", "LESSOPEN", "LESSCLOSE", "GCONV_PATH", "ZDOTDIR", "CDPATH",
	"CLAUDECODE", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_ENTRYPOINT", "CODEX_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME",
	"XDG_RUNTIME_DIR",
}

// reservedEnvPrefixes are the families harnessly, its panes and its integrations set themselves.
var reservedEnvPrefixes = []string{
	"HARNESSLY_", "RUNTIME_", "TMUX", "DYLD_", "CLAUDE_CODE_SESSION", "ARTIFACTS_", "BOARDS_",
	// The sign-in of a Claude subscription is the CLI's own and one for the machine: a provider brings no second one.
	"CLAUDE_CODE_OAUTH",
	// What makes a program the agent starts run somebody's code: git, the interpreters, the shell's functions, npm.
	"GIT_", "PYTHON", "PERL", "RUBY", "BASH_FUNC_", "NPM_CONFIG_",
}

// EnvReserved says whether a variable is the launch's own.
func EnvReserved(name string) bool {
	up := strings.ToUpper(name)
	if slices.Contains(reservedEnv, up) {
		return true
	}
	own := func(p string) bool { return strings.HasPrefix(up, p) }
	return slices.ContainsFunc(reservedEnvPrefixes, own) || slices.ContainsFunc(deploymentPrefixes(), own)
}

// ReservedPrefixesEnv names more families of a deployment's own variables, comma-separated ("ACME_,WRAPPER_"): what
// the tools around harnessly set for a chat and no provider may override.
const ReservedPrefixesEnv = "RUNTIME_LAUNCH_ENV_RESERVED"

var deploymentPrefixes = sync.OnceValue(func() []string {
	var out []string
	for _, p := range strings.Split(os.Getenv(ReservedPrefixesEnv), ",") {
		if p = strings.ToUpper(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
})

var secretLooking = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIALS?)$`)

// EnvAlwaysSecret says whether a variable is kept in the safe whatever the form says: a credential by its name.
func EnvAlwaysSecret(name string) bool { return secretLooking.MatchString(name) }

// EnvCredentials are the variables Claude Code takes a credential from.
var EnvCredentials = []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"}

func envRef(provider, name string) string {
	return "provider-" + provider + "-env-" + strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// applyEnv turns the variables of a patch into the ones the registry keeps, putting the values of the secret ones into
// the safe. A plain variable with an empty value is left out: an empty field is «as the launch makes it».
func (s *Service) applyEnv(provider string, have []EnvVar, in []EnvInput) ([]EnvVar, error) {
	if len(in) > MaxEnv {
		return nil, refuse(ReasonBadEnv)
	}
	out := make([]EnvVar, 0, len(in))
	type put struct{ ref, value string }
	var puts []put
	seen := map[string]bool{}
	for _, v := range in {
		name := strings.TrimSpace(v.Name)
		if !envName.MatchString(name) || seen[name] {
			return nil, refuse(ReasonBadEnv)
		}
		if EnvReserved(name) {
			return nil, refuse(ReasonEnvReserved)
		}
		seen[name] = true
		value := ""
		if v.Value != nil {
			value = *v.Value
		}
		if len(value) > MaxEnvValue || strings.ContainsAny(value, "\r\n\x00") {
			return nil, refuse(ReasonBadEnv)
		}
		if !v.Secret && !EnvAlwaysSecret(name) {
			if agents.ClaudeSubscriptionToken(value) {
				return nil, refuse(ReasonSubscriptionToken)
			}
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, EnvVar{Name: name, Value: value})
			}
			continue
		}
		ref := envRef(provider, name)
		if agents.ClaudeSubscriptionToken(value) {
			return nil, refuse(ReasonSubscriptionToken)
		}
		if value = strings.TrimSpace(value); value != "" {
			puts = append(puts, put{ref, value})
		} else {
			// Nothing typed: the variable keeps the secret it has, its own or one that was only named.
			k := slices.IndexFunc(have, func(x EnvVar) bool { return x.Name == name && x.Secret })
			if k < 0 || !s.holds(have[k].Ref) {
				// Nothing kept either: there is no value to launch with.
				return nil, refuse(ReasonBadEnv)
			}
			ref = have[k].Ref
		}
		out = append(out, EnvVar{Name: name, Secret: true, Ref: ref})
	}
	// The safe changes only when the whole list is taken.
	for _, p := range puts {
		if err := s.Secrets.Put(p.ref, p.value); err != nil {
			return nil, &Refusal{Reason: ReasonFailed, Err: err}
		}
	}
	for _, old := range have {
		if old.Secret && strings.HasPrefix(old.Ref, "provider-"+provider+"-") && !slices.ContainsFunc(out, func(x EnvVar) bool { return x.Ref == old.Ref }) {
			_ = s.Secrets.Remove(old.Ref)
		}
	}
	return out, nil
}

func (s *Service) envView(env []EnvVar) []EnvView {
	out := make([]EnvView, 0, len(env))
	for _, v := range env {
		if v.Secret {
			out = append(out, EnvView{Name: v.Name, Secret: true, Set: s.holds(v.Ref)})
			continue
		}
		out = append(out, EnvView{Name: v.Name, Value: v.Value, Set: true})
	}
	return out
}
