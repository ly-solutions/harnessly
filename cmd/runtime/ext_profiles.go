package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/ly-solutions/harnessly/internal/agents"
	"github.com/ly-solutions/harnessly/internal/extproviders"
	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// A provider the person added by API that speaks the Anthropic API is a profile a Claude Code chat starts on
// its address and models, its key from the safe, and over them the variables the person typed for it. The
// profiles are made again from the registry whenever it changes; a chat that runs already keeps the settings file it
// was started with until it is started again.

// extProfileID is the id of the profile of a provider: «x-» and the provider's id, or a digest of it when that would
// be longer than a profile id may be.
func extProfileID(provider string) string {
	if id := "x-" + provider; wire.ValidProfileID(id) {
		return id
	}
	sum := sha256.Sum256([]byte(provider))
	return "x-" + hex.EncodeToString(sum[:])[:16]
}

// extProfile is the launch profile of a provider; ok is false for one Claude Code cannot run on (another API, or no
// model a launch would take).
func extProfile(p extproviders.Provider) (providers.Preset, bool) {
	if !p.ClaudeCLI() {
		return providers.Preset{}, false
	}
	out := providers.Preset{ID: extProfileID(p.ID), Label: p.Name, Vendor: wire.VendorOwn, Via: wire.ProviderClaude,
		BaseURL: p.BaseURL, Host: p.Host(), KeyRef: p.KeyRef, Custom: true, Env: map[string]string{}, SecretEnv: map[string]string{}}
	for _, m := range p.Models {
		if !agents.IsProfileModelID(m.ID) {
			continue
		}
		label := m.Label
		if label == "" {
			label = m.ID
		}
		out.Models = append(out.Models, providers.Model{ID: m.ID, Label: label})
	}
	if len(out.Models) == 0 {
		return providers.Preset{}, false
	}
	out.Small, out.Test = out.Models[0].ID, out.Models[0].ID
	for _, v := range p.Env {
		if v.Secret {
			out.SecretEnv[v.Name] = v.Ref
		} else {
			out.Env[v.Name] = v.Value
		}
	}
	// The model the person typed is what every chat on the provider runs (providers.ClaudeModelPinned), so it is the
	// one model the picker offers: a list to pick from would name a model the chat is not on.
	if pinned := strings.TrimSpace(out.Env["ANTHROPIC_MODEL"]); agents.IsProfileModelID(pinned) {
		out.Models = []providers.Model{{ID: pinned, Label: modelLabel(out.Models, pinned)}}
	}
	return out, true
}

// modelLabel is the name the provider's list gives the model, or its id when the list does not have it.
func modelLabel(models []providers.Model, id string) string {
	for _, m := range models {
		if m.ID == id {
			return m.Label
		}
	}
	return id
}

// syncExtProfiles makes the launch profiles of a data folder's providers again and has the store read their keys anew.
func syncExtProfiles(svc *extproviders.Service, store *providers.Store) {
	list, err := svc.Store.List()
	if err != nil {
		return
	}
	var out []providers.Preset
	for _, p := range list {
		if pr, ok := extProfile(p); ok {
			out = append(out, pr)
		}
	}
	providers.SetCustom(out)
	for _, pr := range out {
		store.Stale(pr.ID)
	}
}
