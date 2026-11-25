package capabilities

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/config"
	"github.com/ly-solutions/harnessly/internal/hub/features"
	"github.com/ly-solutions/harnessly/internal/hub/switches"
)

type orgGate map[string]bool

func (g orgGate) On(_ context.Context, _, name string) bool { return g[name] }
func (g orgGate) Forced(name string) bool                   { return name == "env" }

var cyrillic = regexp.MustCompile(`[а-яА-ЯёЁ]`)

// Every row is written for a person: Russian name, description and group, a known group, no repeated id.
func TestCatalogIsCompleteAndInRussian(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range catalog {
		if seen[d.id] {
			t.Errorf("%s twice", d.id)
		}
		seen[d.id] = true
		if !slices.Contains(Groups, d.group) {
			t.Errorf("%s: unknown group %q", d.id, d.group)
		}
		for what, text := range map[string]string{"name": d.name, "description": d.desc} {
			if !cyrillic.MatchString(text) {
				t.Errorf("%s: %s %q is not Russian", d.id, what, text)
			}
		}
		if d.source == Env && d.on == nil && !d.defaultOn {
			t.Errorf("%s: an env row with no reading", d.id)
		}
		if !d.defaultOn && d.why == "" && d.needs == "" {
			t.Errorf("%s: off by default and no reason said", d.id)
		}
		for _, w := range []string{"rollout", "флаг", "feature", "переключател"} {
			if strings.Contains(strings.ToLower(d.name+d.desc+d.why+d.needs), w) {
				t.Errorf("%s: inner word %q in the text", d.id, w)
			}
		}
	}
	// Every flag of the organizations is there, so none is left out of the screen.
	for _, f := range features.Known {
		if !seen[f] {
			t.Errorf("flag %s is not in the list", f)
		}
	}
	// Every setting named is one the hub reads, and every default-on switch and flag is marked.
	known := map[string]bool{}
	for _, f := range config.Flags {
		known[f.Name] = true
	}
	for _, d := range catalog {
		if d.source == Env && !known[d.id] {
			t.Errorf("%s is not a variable of the hub", d.id)
		}
		if d.viaEnv != "" && !known[d.viaEnv] {
			t.Errorf("%s: %s is not a variable of the hub", d.id, d.viaEnv)
		}
		if d.source == Rollout && switches.DefaultOn[d.id] != d.defaultOn {
			t.Errorf("%s: DefaultOn disagrees with the list", d.id)
		}
		if d.source == Org && slices.Contains(config.DefaultFeatures, d.id) != d.defaultOn {
			t.Errorf("%s: the built-in flags disagree with the list", d.id)
		}
	}
	for id := range switches.DefaultOn {
		if !seen[id] {
			t.Errorf("rollout switch %s is on by default and not in the list", id)
		}
	}
}

func TestListStates(t *testing.T) {
	c := &config.Config{Automations: true, LimitsEnforce: "warn", EnvFeatures: []string{"catalog"}}
	items := List(context.Background(), c, Gates{Org: orgGate{"machine.seats": true}, Rollout: switches.Static{"automation-triggers": true}}, "o1", true)
	by := map[string]Item{}
	for _, it := range items {
		by[it.ID] = it
	}
	if it := by["HUB_LIMITS_ENFORCE"]; !it.Enabled || it.Detail == "" || it.CanToggle {
		t.Errorf("limits = %+v", it)
	}
	if it := by["HUB_DOCS"]; it.Enabled {
		t.Errorf("docs = %+v", it)
	}
	// A flag the instance forces cannot be toggled; one the settings put on for all names that setting.
	if it := by["env"]; it.Enabled || !it.Forced || it.CanToggle || it.SetBy != "HUB_FEATURES_FORCE" {
		t.Errorf("env = %+v", it)
	}
	if it := by["catalog"]; !it.Enabled || !it.Forced || it.CanToggle || it.SetBy != "HUB_ENV_FEATURES" {
		t.Errorf("catalog = %+v", it)
	}
	if it := by["machine.seats"]; !it.Enabled || !it.CanToggle {
		t.Errorf("seats = %+v", it)
	}
	if it := by["automation-triggers"]; !it.Enabled || !it.CanToggle {
		t.Errorf("triggers = %+v", it)
	}
	// Without the right to change, nothing is toggled.
	for _, it := range List(context.Background(), c, Gates{}, "o1", false) {
		if it.CanToggle {
			t.Errorf("%s toggles for a viewer who may not", it.ID)
		}
	}
}
