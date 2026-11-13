package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/codexaccounts"
	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/locale"
	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/sameorigin"
)

// The sign-ins of the Codex CLI beside the main one (internal/codexaccounts), for the Settings screen. All of
// it is the person's page: a page session, same origin. No answer holds a token: a sign-in is told by whose it is.
//
//	GET    <platform>/codex-logins        {logins: [{id, name, profile, account?}], max}
//	POST   <platform>/codex-logins        {name} -> 201 {login}
//	PATCH  <platform>/codex-logins/<id>   {name} -> {login}
//	DELETE <platform>/codex-logins/<id>   {ok: true}
//
// The sign-in itself is the agents' route with `?account=<id>` (wire.Server.AccountLogins). A refusal is
// {error: <reason>, message: <words for the person>}.

const codexLoginBodyMax = 4 << 10

// codexLoginView is a sign-in as the page shows it. Account is whose plan it is signed in with; "" when it is not.
type codexLoginView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Profile string `json:"profile"`
	Account string `json:"account,omitempty"`
	// Main: a new Codex chat is offered this sign-in first.
	Main bool `json:"main,omitempty"`
}

func codexLoginViewOf(st *codexaccounts.Store, a codexaccounts.Account) codexLoginView {
	v := codexLoginView{ID: a.ID, Name: a.Name, Profile: codexLoginProfileID(a.ID), Main: a.Main}
	if account, ok := (&providers.CodexLogin{Path: st.AuthFile(a.ID)}).Account(); ok {
		v.Account = account
	}
	return v
}

// errCodexLoginInUse: chats still run on the sign-in.
var errCodexLoginInUse = errors.New("codex-logins: chats run on the sign-in")

func codexLoginRefusal(w http.ResponseWriter, err error) {
	status, reason, said := http.StatusInternalServerError, "failed", locale.Pick("It did not work. Try again.", "Не получилось. Попробуйте ещё раз.")
	switch {
	case errors.Is(err, codexaccounts.ErrNotFound):
		status, reason, said = http.StatusNotFound, "not-found", locale.Pick("There is no such sign-in any more.", "Такого входа уже нет.")
	case errors.Is(err, codexaccounts.ErrTooMany):
		status, reason, said = http.StatusConflict, "too-many", locale.Pick("There are too many sign-ins. Remove one you do not use.", "Входов слишком много. Удалите ненужный.")
	case errors.Is(err, errCodexLoginInUse):
		status, reason, said = http.StatusConflict, "in-use", locale.Pick("Chats run on this sign-in. Stop them, then remove it.", "На этом входе идут чаты. Остановите их, потом удаляйте вход.")
	case errors.Is(err, codexaccounts.ErrBadName):
		status, reason, said = http.StatusBadRequest, "bad-name", locale.Pick("Give it a name no other sign-in has.", "Дайте название, которого нет у других входов.")
	default:
		agent.Log("warn", "codex-logins: failed", agent.F{K: "error", V: err.Error()})
	}
	secretJSON(w, status, map[string]string{"error": reason, "message": said})
}

func codexLoginRoutes(d deps) []gate.Route {
	st := &codexaccounts.Store{Dir: d.DataDir, Main: providers.CodexHome(d.Home)}
	if d.Home == "" {
		st.Main = filepath.Join(d.DataDir, "no-codex-home")
	}
	base := platformAPI + "/codex-logins"
	under := func(r *http.Request) (string, bool) {
		for _, prefix := range []string{base + "/", "/agents" + base + "/"} {
			if rest, ok := strings.CutPrefix(r.URL.Path, prefix); ok && rest != "" && !strings.Contains(rest, "/") {
				return rest, true
			}
		}
		return "", false
	}
	pageOK := func(w http.ResponseWriter, r *http.Request) bool {
		if sameorigin.Refuse(w, r) {
			return false
		}
		if !d.SessionOK(r) {
			secretJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			return false
		}
		return true
	}
	name := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		var in struct {
			Name string `json:"name"`
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, codexLoginBodyMax+1))
		if err != nil || len(raw) > codexLoginBodyMax || json.Unmarshal(raw, &in) != nil {
			codexLoginRefusal(w, codexaccounts.ErrBadName)
			return "", false
		}
		return in.Name, true
	}
	// The registry the running chats use is the hub's when the runtime has one; the routes of a runtime without
	// agents keep the list all the same.
	store := func() *codexaccounts.Store {
		if h := codexLoginHub.Load(); h != nil {
			return h.store
		}
		return st
	}
	changed := func(removed string) {
		if h := codexLoginHub.Load(); h != nil {
			if removed != "" {
				h.forget(removed)
			}
			h.touched()
			return
		}
		syncCodexLoginProfiles(st)
	}

	list := func(w http.ResponseWriter, r *http.Request) {
		if !pageOK(w, r) {
			return
		}
		s := store()
		switch r.Method {
		case http.MethodGet:
			all, err := s.List()
			if err != nil {
				codexLoginRefusal(w, err)
				return
			}
			views := make([]codexLoginView, 0, len(all))
			for _, a := range all {
				views = append(views, codexLoginViewOf(s, a))
			}
			secretJSON(w, http.StatusOK, map[string]any{"logins": views, "max": codexaccounts.MaxAccounts})
		case http.MethodPost:
			n, ok := name(w, r)
			if !ok {
				return
			}
			a, err := s.Add(n)
			if err != nil {
				codexLoginRefusal(w, err)
				return
			}
			changed("")
			agent.Log("info", "codex-logins: added", agent.F{K: "id", V: a.ID})
			secretJSON(w, http.StatusCreated, map[string]any{"login": codexLoginViewOf(s, a)})
		default:
			secretJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		}
	}
	one := func(w http.ResponseWriter, r *http.Request) {
		if !pageOK(w, r) {
			return
		}
		id, _ := under(r)
		s := store()
		switch r.Method {
		case http.MethodPatch:
			n, ok := name(w, r)
			if !ok {
				return
			}
			a, err := s.Rename(id, n)
			if err != nil {
				codexLoginRefusal(w, err)
				return
			}
			changed("")
			secretJSON(w, http.StatusOK, map[string]any{"login": codexLoginViewOf(s, a)})
		case http.MethodPut:
			// {main: true} makes it the sign-in a new chat is offered first; {main: false} gives that back to the CLI's own.
			var in struct {
				Main *bool `json:"main"`
			}
			raw, err := io.ReadAll(io.LimitReader(r.Body, codexLoginBodyMax+1))
			if err != nil || len(raw) > codexLoginBodyMax || json.Unmarshal(raw, &in) != nil || in.Main == nil {
				secretJSON(w, http.StatusBadRequest, map[string]string{"error": "bad-request"})
				return
			}
			all, err := s.List()
			if err != nil {
				codexLoginRefusal(w, err)
				return
			}
			i := slices.IndexFunc(all, func(a codexaccounts.Account) bool { return a.ID == id })
			if i < 0 {
				codexLoginRefusal(w, codexaccounts.ErrNotFound)
				return
			}
			to := id
			if !*in.Main {
				to = ""
				if !all[i].Main { // it is not the main one: nothing to give back
					secretJSON(w, http.StatusOK, map[string]bool{"ok": true})
					return
				}
			}
			if err := s.SetMain(to); err != nil {
				codexLoginRefusal(w, err)
				return
			}
			changed("")
			secretJSON(w, http.StatusOK, map[string]bool{"ok": true})
		case http.MethodDelete:
			if h := codexLoginHub.Load(); h != nil && h.inUse != nil && h.inUse(codexLoginProfileID(id)) {
				codexLoginRefusal(w, errCodexLoginInUse)
				return
			}
			if err := s.Remove(id); err != nil {
				codexLoginRefusal(w, err)
				return
			}
			changed(id)
			agent.Log("info", "codex-logins: removed", agent.F{K: "id", V: id})
			secretJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			secretJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		}
	}
	return []gate.Route{
		{Match: func(r *http.Request) bool { return pathIs(r, base) }, Handler: http.HandlerFunc(list)},
		{Match: func(r *http.Request) bool { _, ok := under(r); return ok }, Handler: http.HandlerFunc(one)},
	}
}
