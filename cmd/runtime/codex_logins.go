package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ly-solutions/harnessly/internal/agents/codex"
	"github.com/ly-solutions/harnessly/internal/codexaccounts"
	"github.com/ly-solutions/harnessly/internal/limits"
	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// One more sign-in of the Codex CLI is a profile a Codex chat starts on: the CLI itself, with the home of
// that sign-in (internal/codexaccounts) in place of the main one. It has no provider and no key of its own; its models
// are the ones of a ChatGPT plan. The profiles are made again from the registry whenever it changes.

// codexLoginPrefix starts the id of the profile of a sign-in: «cx-» and the account's id.
const codexLoginPrefix = limits.CodexLoginPrefix

// codexLoginStore is the registry of this runtime's data folder, set once the runtime knows its folders.
var codexLoginStore atomic.Pointer[codexaccounts.Store]

// codexLoginFlows are the sign-in services of the accounts, one for each, made when first asked for: a sign-in of an
// account runs the CLI in that account's home.
type codexLoginFlows struct {
	store *codexaccounts.Store
	// make builds the service of a home; changed is told after a sign-in or a sign-out of any account.
	make func(codexHome string, changed func(wire.Provider)) wire.LoginSource
	// inUse says whether a chat that still has its CLI is on the profile; nil when the runtime runs no chats.
	inUse func(profile string) bool
	// changed is told after an account was added, renamed, removed, signed in or out: the profiles are made again by
	// then.
	changed func()

	mu    sync.Mutex
	flows map[string]wire.LoginSource
}

var codexLoginHub atomic.Pointer[codexLoginFlows]

// of is the sign-in service of an account of Codex; false for another CLI or an account the registry does not have.
func (h *codexLoginFlows) of(p wire.Provider, account string) (wire.LoginSource, bool) {
	if h == nil || h.make == nil || p != wire.ProviderCodex {
		return nil, false
	}
	list, err := h.store.List()
	if err != nil {
		return nil, false
	}
	known := false
	for _, a := range list {
		known = known || a.ID == account
	}
	if !known {
		return nil, false
	}
	// The home is there, with the settings of the main one, before the CLI is started in it.
	if err := h.store.Sync(account); err != nil {
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.flows == nil {
		h.flows = map[string]wire.LoginSource{}
	}
	if f := h.flows[account]; f != nil {
		return codexAccountLogin{f}, true
	}
	f := h.make(h.store.Home(account), func(wire.Provider) { h.touched() })
	h.flows[account] = f
	return codexAccountLogin{f}, true
}

// liveOn says whether the list has a conversation on the profile whose CLI may still be there: every state but the
// ones that say the process is gone.
func liveOn(profile string, convs []wire.Conversation) bool {
	for _, c := range convs {
		if c.Model == nil || c.Model.Profile != profile {
			continue
		}
		switch c.State.K {
		case wire.StateStopped, wire.StateLaunchFailed, wire.StateGone:
		default:
			return true
		}
	}
	return false
}

// forget drops the service of a removed account.
func (h *codexLoginFlows) forget(account string) {
	h.mu.Lock()
	delete(h.flows, account)
	h.mu.Unlock()
}

// touched makes the profiles again and tells whoever shows them.
func (h *codexLoginFlows) touched() {
	syncCodexLoginProfiles(h.store)
	if h.changed != nil {
		h.changed()
	}
}

// codexAccountLogin is the sign-in of one more account: by the plan only. An API key is the main home's matter (it is
// no subscription, and a second one changes nothing).
type codexAccountLogin struct{ wire.LoginSource }

func (codexAccountLogin) SaveAPIKey(context.Context, wire.Provider, string) error {
	return wire.ErrLoginUnsupported
}

func codexLoginProfileID(account string) string { return codexLoginPrefix + account }

// codexLoginProfile is the launch profile of a sign-in; ok is false when the plan's models are not known here.
func codexLoginProfile(st *codexaccounts.Store, a codexaccounts.Account) (providers.Preset, bool) {
	plan, ok := providers.Find("gpt")
	if !ok || len(plan.Models) == 0 {
		return providers.Preset{}, false
	}
	return providers.Preset{ID: codexLoginProfileID(a.ID), Label: a.Name, Vendor: wire.VendorGPT, Via: wire.ProviderCodex,
		BaseURL: plan.BaseURL, Host: plan.Host, Hint: "Через Codex: ещё один вход по подписке ChatGPT",
		Models: plan.Models, Small: plan.Small, Test: plan.Test, Home: st.Home(a.ID)}, true
}

// syncCodexLoginProfiles makes the launch profiles of the registry's sign-ins again.
func syncCodexLoginProfiles(st *codexaccounts.Store) {
	list, err := st.List()
	if err != nil {
		return
	}
	var out []providers.Preset
	for _, a := range list {
		// The sessions of a sign-in are in its own home: the transcript readers are told where.
		codex.RememberHome(st.Home(a.ID))
		if p, ok := codexLoginProfile(st, a); ok {
			out = append(out, p)
		}
	}
	providers.SetCodexLogins(out)
}

// codexLoginSessionRoots are the sessions folders of the sign-ins' homes, for the readers of the spend.
func codexLoginSessionRoots() []string {
	st := codexLoginStore.Load()
	if st == nil {
		return nil
	}
	list, err := st.List()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, filepath.Join(st.Home(a.ID), "sessions"))
	}
	return out
}

// syncCodexLoginHome brings the home of a sign-in's profile up to date with the main home before a chat starts in it
// (providers.Launcher.SyncHome). A profile that is no sign-in, or a runtime without the registry, has nothing to do.
func syncCodexLoginHome(profile string) error {
	st := codexLoginStore.Load()
	id, ok := strings.CutPrefix(profile, codexLoginPrefix)
	if st == nil || !ok {
		return nil
	}
	return st.Sync(id)
}
