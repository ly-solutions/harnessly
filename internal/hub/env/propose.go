package env

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/auth"
	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// A change of the organization's environment an agent proposes from a machine (hubproto.EnvProposal). The
// link of a machine proves the machine and not a person, so a machine changes nothing here: its proposal is kept and
// waits. An administrator of the organization sees it on the environment screen (what changes, without a value of any
// secret; which machine and which chat it came from) and applies or declines it under their own session; applying goes
// through the very handlers the screen calls, with that person's rights, checks and journal. A proposal nobody decided
// stops waiting after ProposalTTL.

// ProposalTTL is how long a proposal waits for an administrator.
const ProposalTTL = 7 * 24 * time.Hour

// maxWaiting is how many proposals of an organization wait at once: a machine cannot fill the screen with them.
const maxWaiting = 20

// maxWaitingOfMachine is how many of them one machine has: one machine does not take the others' places.
const maxWaitingOfMachine = 5

type viaAgentKey struct{}

// viaAgent is where a change an agent proposed comes from, kept in the context for the journal.
type viaAgent struct{ machine, conv, title string }

// agentNote is what the journal adds to the title of a change an agent proposed; "" for a person's own.
func agentNote(ctx context.Context) string {
	v, ok := ctx.Value(viaAgentKey{}).(viaAgent)
	if !ok {
		return ""
	}
	chat := v.title
	if chat == "" {
		chat = v.conv
	}
	if chat == "" {
		return fmt.Sprintf(" — по предложению агента с машины «%s»", v.machine)
	}
	return fmt.Sprintf(" — по предложению агента из чата «%s» на машине «%s»", chat, v.machine)
}

// discard is a response nobody reads: the handlers answer the caller by their error and by what they changed.
type discard struct{ h http.Header }

func (d discard) Header() http.Header         { return d.h }
func (d discard) Write(b []byte) (int, error) { return len(b), nil }
func (d discard) WriteHeader(int)             {}

// as calls a handler of the screen as the person, with the path values and the body it would get.
func (s *Service) as(ctx context.Context, p *auth.Principal, method string, vals map[string]string, body any,
	h func(http.ResponseWriter, *http.Request, *auth.Principal) error) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, method, "/", bytes.NewReader(b))
	if err != nil {
		return err
	}
	for k, v := range vals {
		r.SetPathValue(k, v)
	}
	return h(discard{h: http.Header{}}, r, p)
}

func proposalRefused(rid string, err error) hubproto.EnvProposalResult {
	var he *httpx.Error
	if errors.As(err, &he) {
		return hubproto.EnvProposalResult{RequestID: rid, Status: he.Status, Code: he.Code, Message: he.Message}
	}
	return hubproto.EnvProposalResult{RequestID: rid, Status: http.StatusInternalServerError, Code: "internal", Message: "Хаб не смог принять предложение"}
}

// providerOf is the organization's own provider item of that name, archived ones too.
func providerOf(w *world, name string) (store.CatalogItem, bool) {
	for _, c := range w.items {
		if c.Kind == store.CatalogProvider && c.OrgID == w.org.ID && c.OwnerLayer == "" && strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return store.CatalogItem{}, false
}

const proposalWaits = "Предложение отправлено админу организации: окружение изменится, когда он применит его в хабе (Окружение → Предложения агентов)."

// Proposed takes a proposal of the machine's agent: checks its shape and keeps it for an administrator of the
// organization. It changes nothing of the environment, whoever the machine belongs to.
func (s *Service) Proposed(ctx context.Context, m store.Machine, raw json.RawMessage) hubproto.EnvProposalResult {
	var in hubproto.EnvProposal
	if err := json.Unmarshal(raw, &in); err != nil {
		return hubproto.EnvProposalResult{Status: http.StatusBadRequest, Code: "invalid_request", Message: "Не удалось разобрать запрос"}
	}
	rid := in.RequestID
	now := s.now()
	var kept store.EnvProposal
	err := s.db.Tx(ctx, func(st store.Store) error {
		sp, err := st.SpaceByID(ctx, m.SpaceID)
		if err != nil {
			return err
		}
		if sp.OrgID == "" {
			return httpx.New(http.StatusConflict, "no_org", "Машина не в организации: окружения у неё нет")
		}
		if s.feature(ctx, sp.OrgID, FeatureEnv) != nil || s.feature(ctx, sp.OrgID, FeatureCatalog) != nil {
			return httpx.New(http.StatusConflict, "not_enabled", "Окружение у этой организации не включено")
		}
		o, err := st.OrgByID(ctx, sp.OrgID)
		if err != nil {
			return err
		}
		w, err := loadWorld(ctx, st, o, s.builtIn)
		if err != nil {
			return err
		}
		name, err := CleanText("name", in.Provider.Name, maxNameRunes, true)
		if err != nil {
			return err
		}
		hint, err := CleanText("hint", in.Provider.Hint, maxHintRunes, false)
		if err != nil {
			return err
		}
		src := json.RawMessage("{}")
		if in.Provider.Remove {
			if _, found := providerOf(w, name); !found {
				return httpx.Invalid("name", "Провайдера с таким названием в окружении организации нет")
			}
		} else if src, err = CleanSource(store.CatalogProvider, in.Provider.Source); err != nil {
			return err
		}
		if in.Check {
			return nil
		}
		waiting, err := st.WaitingEnvProposals(ctx, o.ID, now)
		if err != nil {
			return err
		}
		n, own := 0, 0
		for _, p := range waiting {
			if p.MachineID == m.ID && p.Name == name {
				continue // replaced by this one
			}
			n++
			if p.MachineID == m.ID {
				own++
			}
		}
		if own >= maxWaitingOfMachine {
			return httpx.New(http.StatusTooManyRequests, "too_many", "С этой машины уже ждёт несколько предложений; дождитесь решения админа организации")
		}
		if n >= maxWaiting {
			return httpx.New(http.StatusTooManyRequests, "too_many", "У админа организации уже много неразобранных предложений; новое не принято")
		}
		kept, err = st.FileEnvProposal(ctx, store.EnvProposal{OrgID: o.ID, MachineID: m.ID, MachineName: m.Name, OwnerID: m.CreatedBy,
			Conv: clip(in.Conv, 200), ConvTitle: clip(in.ConvTitle, 200), Name: name, Hint: hint, Remove: in.Provider.Remove, Source: string(src),
			CreatedAt: now, ExpiresAt: now.Add(ProposalTTL)}, now)
		return err
	})
	if err != nil {
		return proposalRefused(rid, err)
	}
	if in.Check {
		return hubproto.EnvProposalResult{RequestID: rid, Status: http.StatusOK}
	}
	if s.pub != nil {
		s.pub.Publish(kept.OrgID, "env.proposals", map[string]any{})
	}
	return hubproto.EnvProposalResult{RequestID: rid, Status: http.StatusAccepted, Code: "waits", Message: proposalWaits, ProposalID: kept.ID}
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// The kinds of change a proposal makes.
const (
	proposalAdd    = "add"
	proposalUpdate = "update"
	proposalRemove = "remove"
)

// ProposalView is a waiting proposal as an administrator sees it. Source and Before name secrets and hold no value of
// one. What the agent wrote (the name, the hint, the chat's title) is its text, shown as it is.
type ProposalView struct {
	ID     string `json:"id"`
	Change string `json:"change"`
	Name   string `json:"name"`
	Hint   string `json:"hint"`
	// Source is the provider as proposed; Before is the one of that name the organization has now (a change only).
	Source json.RawMessage `json:"source,omitempty"`
	Before json.RawMessage `json:"before,omitempty"`
	// Machine is the machine's name, Owner the person who connected it ("" when the account is gone).
	Machine   string `json:"machine"`
	Owner     string `json:"owner"`
	Conv      string `json:"conv"`
	ConvTitle string `json:"convTitle"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (q *req) proposalView(p store.EnvProposal) ProposalView {
	v := ProposalView{ID: p.ID, Change: proposalAdd, Name: p.Name, Hint: p.Hint, Machine: p.MachineName, Conv: p.Conv, ConvTitle: p.ConvTitle,
		CreatedAt: store.Ms(p.CreatedAt), ExpiresAt: store.Ms(p.ExpiresAt)}
	if p.OwnerID != "" {
		if u, err := q.st.UserByID(q.ctx, p.OwnerID); err == nil {
			if v.Owner = u.Name; v.Owner == "" {
				v.Owner = u.Email
			}
		}
	}
	have, found := providerOf(q.w, p.Name)
	switch {
	case p.Remove:
		v.Change = proposalRemove
		if found {
			v.Before = sourceOf(have)
		}
	case found:
		v.Change, v.Source, v.Before = proposalUpdate, json.RawMessage(p.Source), sourceOf(have)
	default:
		v.Source = json.RawMessage(p.Source)
	}
	return v
}

func sourceOf(c store.CatalogItem) json.RawMessage {
	if c.Source == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(c.Source)
}

// openProposals opens the organization for an administrator of it: nobody else sees or decides proposals.
func (s *Service) openProposals(r *http.Request, st store.Store, p *auth.Principal) (*req, error) {
	q, err := s.open(r, st, p, FeatureCatalog)
	if err != nil {
		return nil, err
	}
	if err := s.feature(r.Context(), q.w.org.ID, FeatureEnv); err != nil {
		return nil, err
	}
	return q, q.requireOrgAdmin()
}

// listProposals: GET /api/orgs/{org}/env/proposals.
func (s *Service) listProposals(w http.ResponseWriter, r *http.Request, p *auth.Principal) error {
	out := []ProposalView{}
	err := s.db.Tx(r.Context(), func(st store.Store) error {
		q, err := s.openProposals(r, st, p)
		if err != nil {
			return err
		}
		list, err := st.WaitingEnvProposals(r.Context(), q.w.org.ID, s.now())
		if err != nil {
			return err
		}
		for _, one := range list {
			out = append(out, q.proposalView(one))
		}
		return nil
	})
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"proposals": out})
	return nil
}

var errProposalGone = httpx.Conflict("proposal_gone", "Это предложение уже разобрано или его срок вышел")

// decide takes a waiting proposal of the organization out of the waiting ones, as the administrator p.
func (s *Service) decide(r *http.Request, p *auth.Principal, state string) (store.EnvProposal, string, error) {
	var one store.EnvProposal
	var orgID string
	err := s.db.Tx(r.Context(), func(st store.Store) error {
		q, err := s.openProposals(r, st, p)
		if err != nil {
			return err
		}
		orgID = q.w.org.ID
		one, err = st.EnvProposalByID(r.Context(), r.PathValue("proposalId"))
		if errors.Is(err, store.ErrNotFound) || (err == nil && one.OrgID != orgID) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		ok, err := st.DecideEnvProposal(r.Context(), one.ID, state, p.User.ID, s.now())
		if err != nil {
			return err
		}
		if !ok {
			return errProposalGone
		}
		if state != store.EnvProposalDeclined {
			return nil
		}
		ctx := context.WithValue(r.Context(), viaAgentKey{}, viaAgent{machine: one.MachineName, conv: one.Conv, title: one.ConvTitle})
		title := fmt.Sprintf("Окружение: предложение о провайдере «%s» отклонено", one.Name)
		return s.record(ctx, st, q.w, p, store.EnvLayerOrg, orgID, title, "env.proposal.decline", "env_proposal", one.ID, q.proposalView(one), nil, s.now())
	})
	return one, orgID, err
}

// declineProposal: POST /api/orgs/{org}/env/proposals/{proposalId}/decline.
func (s *Service) declineProposal(w http.ResponseWriter, r *http.Request, p *auth.Principal) error {
	_, orgID, err := s.decide(r, p, store.EnvProposalDeclined)
	if err != nil {
		return err
	}
	if s.pub != nil {
		s.pub.Publish(orgID, "env.proposals", map[string]any{})
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// applyProposal: POST /api/orgs/{org}/env/proposals/{proposalId}/apply. The change is made by the administrator who
// pressed the button, through the handlers of the screen; a change that is refused leaves the proposal waiting.
func (s *Service) applyProposal(w http.ResponseWriter, r *http.Request, p *auth.Principal) error {
	one, orgID, err := s.decide(r, p, store.EnvProposalApplied)
	if err != nil {
		return err
	}
	ctx := context.WithValue(r.Context(), viaAgentKey{}, viaAgent{machine: one.MachineName, conv: one.Conv, title: one.ConvTitle})
	id, err := s.makeProposed(ctx, p, orgID, one)
	if err != nil {
		back := s.db.Tx(r.Context(), func(st store.Store) error { return st.ReopenEnvProposal(r.Context(), one.ID) })
		if back != nil {
			return back
		}
		return err
	}
	if s.pub != nil {
		s.pub.Publish(orgID, "env.proposals", map[string]any{})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"itemId": id})
	return nil
}

// makeProposed makes the change of a proposal as p and returns the catalog item it was made to.
func (s *Service) makeProposed(ctx context.Context, p *auth.Principal, orgID string, one store.EnvProposal) (string, error) {
	var have store.CatalogItem
	var found bool
	var entries []store.EnvEntry
	read := func() error {
		return s.db.Tx(ctx, func(st store.Store) error {
			o, err := st.OrgByID(ctx, orgID)
			if err != nil {
				return err
			}
			w, err := loadWorld(ctx, st, o, s.builtIn)
			if err != nil {
				return err
			}
			have, found = providerOf(w, one.Name)
			entries = nil
			for _, e := range w.entries {
				if found && e.ItemID == have.ID && e.Layer == store.EnvLayerOrg {
					entries = append(entries, e)
				}
			}
			return nil
		})
	}
	if err := read(); err != nil {
		return "", err
	}
	org := map[string]string{"org": orgID}
	item := func(id string) map[string]string { return map[string]string{"org": orgID, "itemId": id} }
	if one.Remove {
		if !found {
			return "", httpx.Invalid("name", "Провайдера с таким названием в окружении организации уже нет")
		}
		for _, e := range entries {
			if err := s.as(ctx, p, http.MethodDelete, map[string]string{"org": orgID, "entryId": e.ID}, nil, s.deleteEntry); err != nil {
				return "", err
			}
		}
		if have.ArchivedAt == nil {
			if err := s.as(ctx, p, http.MethodPatch, item(have.ID), map[string]any{"archived": true}, s.patchCatalog); err != nil {
				return "", err
			}
		}
		return have.ID, nil
	}
	src := json.RawMessage(one.Source)
	if found {
		patch := map[string]any{"source": src, "archived": false}
		if one.Hint != "" {
			patch["hint"] = one.Hint
		}
		if err := s.as(ctx, p, http.MethodPatch, item(have.ID), patch, s.patchCatalog); err != nil {
			return "", err
		}
	} else {
		if err := s.as(ctx, p, http.MethodPost, org, map[string]any{"kind": store.CatalogProvider, "name": one.Name, "hint": one.Hint, "source": src},
			s.addCatalog); err != nil {
			return "", err
		}
		if err := read(); err != nil {
			return "", err
		}
		if !found {
			return "", errors.New("the provider item was not kept")
		}
	}
	if len(entries) == 0 {
		// Every member's machine gets it and nobody switches it off: that is what a provider of the organization is.
		if err := s.as(ctx, p, http.MethodPost, org, map[string]any{"layer": store.EnvLayerOrg, "scopeId": orgID, "itemId": have.ID, "pinned": true},
			s.addEntry); err != nil {
			return "", err
		}
	}
	return have.ID, nil
}
