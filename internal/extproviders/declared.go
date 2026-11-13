package extproviders

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
)

// The person's own accounts as an agent reads and writes them (`harnessly settings`, the key «providers»): the same
// form as a provider of the environment, with a name. An agent never holds a secret: the key and the secret variables
// are named by a secret of the safe (which the person puts there on the agent's ask) or by a vault: reference, and
// a variable that is a credential by its name cannot be given a value.

// ReasonNoSecret: a secret that is named is not in the safe yet.
const ReasonNoSecret = "no-secret"

// Declared is one account.
type Declared struct {
	// ID is the account's; empty for a new one.
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	ManagedSource
	// Default: the account of its provider that is used where none is named.
	Default bool `json:"default,omitempty"`
	// Remove takes the account off (only ID is read then).
	Remove bool `json:"remove,omitempty"`
}

var declaredID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// owns says whether a secret is the registry's own (made for a value typed on the Settings screen) and so goes with
// the account; a secret that is only named stays in the safe.
func (p Provider) owns(ref string) bool {
	return p.Origin == "" && strings.HasPrefix(ref, "provider-"+p.ID+"-")
}

// secretRefs are the secrets an account names.
func (p Provider) secretRefs() []string {
	out := []string{}
	if p.KeyRef != "" {
		out = append(out, p.KeyRef)
	}
	for _, v := range p.Env {
		if v.Secret {
			out = append(out, v.Ref)
		}
	}
	return out
}

func (p Provider) declared() Declared {
	d := Declared{ID: p.ID, Name: p.Name, Default: p.Default, ManagedSource: ManagedSource{API: p.API, BaseURL: p.BaseURL, Preset: p.Preset, Key: p.KeyRef}}
	for _, m := range p.Models {
		d.Models = append(d.Models, m.ID)
	}
	for _, v := range p.Env {
		if v.Secret {
			d.SecretEnv = append(d.SecretEnv, v.Name+"="+v.Ref)
		} else {
			d.Env = append(d.Env, v.Name+"="+v.Value)
		}
	}
	return d
}

// Declared lists the person's own accounts; the environment's ones are not theirs to write and are left out.
func (s *Service) Declared() ([]Declared, error) {
	list, err := s.Store.List()
	if err != nil {
		return nil, err
	}
	out := []Declared{}
	for _, p := range list {
		if p.Origin == "" {
			out = append(out, p.declared())
		}
	}
	return out, nil
}

// ParseDeclared reads what an agent wrote: one account (added, changed or removed; the rest stay) or the whole list
// (whole: the person's accounts become exactly these).
func ParseDeclared(raw string) (list []Declared, whole bool, err error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		if json.Unmarshal([]byte(raw), &list) != nil {
			return nil, false, refuse(ReasonFailed)
		}
		return list, true, nil
	}
	var one Declared
	if json.Unmarshal([]byte(raw), &one) != nil {
		return nil, false, refuse(ReasonFailed)
	}
	return []Declared{one}, false, nil
}

// errDry ends a check before anything is written.
var errDry = errors.New("extproviders: dry")

// Declare writes accounts. All of it is taken or none: a refusal leaves the registry and the safe as they were. dry
// only checks: the answer is what a write would say, and nothing changes.
func (s *Service) Declare(list []Declared, whole, dry bool) error {
	type event struct {
		what          string
		before, after *Provider
	}
	var events []event
	var drop []string
	err := s.Store.change(func(body *fileBody) error {
		events, drop = nil, nil
		next := slices.Clone(body.Providers)
		index := func(id string) int { return slices.IndexFunc(next, func(p Provider) bool { return p.ID == id }) }
		remove := func(i int) {
			gone := next[i]
			for _, ref := range gone.secretRefs() {
				if gone.owns(ref) {
					drop = append(drop, ref)
				}
			}
			next = slices.Delete(next, i, i+1)
			events = append(events, event{"removed", &gone, nil})
		}
		kept := map[string]bool{}
		for _, d := range list {
			i := -1
			if d.ID != "" {
				if !declaredID.MatchString(d.ID) {
					return refuse(ReasonNotFound)
				}
				i = index(d.ID)
			}
			if i >= 0 && next[i].Origin != "" {
				return refuse(ReasonManaged)
			}
			if d.Remove {
				if whole || i < 0 {
					return refuse(ReasonNotFound)
				}
				remove(i)
				continue
			}
			src, err := d.ManagedSource.Clean()
			if err != nil {
				return err
			}
			name, err := cleanName(d.Name)
			if err != nil {
				return err
			}
			// A secret is used for the server it is for and no other. The account's own secret (typed on the Settings
			// screen) stays with the account while the server is the same; any other secret of the safe must have
			// been allowed by the person for this very server; the organization's are not named here at all.
			for _, ref := range src.SecretRefs() {
				if i >= 0 && next[i].owns(ref) && sameServer(next[i].BaseURL, src.BaseURL) && slices.Contains(next[i].secretRefs(), ref) {
					continue
				}
				switch {
				case SharedRef(ref) || strings.HasPrefix(ref, "provider-"):
					return refuse(ReasonSecretUnbound)
				case !s.Secrets.Has(ref):
					return refuse(ReasonNoSecret)
				case !s.allowed(ref, src.BaseURL):
					return refuse(ReasonSecretUnbound)
				}
			}
			want := src.provider("", name)
			want.Origin = ""
			now := s.Store.now()
			if i < 0 {
				if len(next) >= MaxProviders {
					return refuse(ReasonTooMany)
				}
				if slices.ContainsFunc(next, func(p Provider) bool { return sameName(p.Name, name) }) {
					return refuse(ReasonNameTaken)
				}
				want.ID = d.ID
				if want.ID == "" {
					stem := slug(name)
					if stem == "" {
						stem = "provider"
					}
					for want.ID = stem; index(want.ID) >= 0; want.ID = stem + "-" + randomHex(3) {
					}
				}
				// The ids that start so are the environment's.
				if strings.HasPrefix(want.ID, "env-") {
					want.ID = "own-" + want.ID
				}
				want.CreatedAt, want.UpdatedAt, want.Default = now, now, d.Default
				next = append(next, want)
				i = len(next) - 1
				events = append(events, event{"created", nil, &next[i]})
			} else {
				have := next[i]
				if slices.ContainsFunc(next, func(p Provider) bool { return p.ID != have.ID && sameName(p.Name, name) }) {
					return refuse(ReasonNameTaken)
				}
				want.ID, want.CreatedAt, want.UpdatedAt, want.Default = have.ID, have.CreatedAt, now, have.Default || d.Default
				if len(want.Models) == 0 && sameServer(have.BaseURL, want.BaseURL) && have.API == want.API {
					want.Models, want.ModelsAt = have.Models, have.ModelsAt
				}
				for _, ref := range have.secretRefs() {
					if have.owns(ref) && !slices.Contains(want.secretRefs(), ref) {
						drop = append(drop, ref)
					}
				}
				next[i] = want
				events = append(events, event{"updated", &have, &next[i]})
			}
			if d.Default {
				for j := range next {
					if next[j].Group() == next[i].Group() {
						next[j].Default = j == i
					}
				}
			}
			kept[next[i].ID] = true
		}
		if whole {
			for i := len(next) - 1; i >= 0; i-- {
				if next[i].Origin == "" && !kept[next[i].ID] {
					remove(i)
				}
			}
		}
		settleDefaults(next)
		// The events point into next: fix them to the records as they are kept.
		for k := range events {
			if events[k].after != nil {
				if j := index(events[k].after.ID); j >= 0 {
					after := next[j]
					events[k].after = &after
				}
			}
		}
		if dry {
			return errDry
		}
		body.Providers = next
		return nil
	})
	if dry && errors.Is(err, errDry) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, ref := range drop {
		_ = s.Secrets.Remove(ref)
	}
	for _, e := range events {
		s.changed(e.what, e.before, e.after)
	}
	return nil
}
