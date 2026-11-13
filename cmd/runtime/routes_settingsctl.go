package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/bgmodel"
	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/locale"
	"github.com/ly-solutions/harnessly/internal/sameorigin"
	"github.com/ly-solutions/harnessly/internal/settingsctl"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// An agent changes the settings (internal/settingsctl). Who may call what:
//
//	GET  <platform>/settings-catalog                       {settings: [Setting]}
//	PUT  <platform>/settings-catalog/<key>                 {value} -> {change: Change}: the person changes it at once
//	GET  <platform>/settings-changes?limit=50&key=<key>    {changes: [Change]}, the newest first: the journal
//	GET  <platform>/settings-proposals                     {proposals: [Proposal]}: the cards that wait for the person
//	POST <platform>/settings-proposals/<id>/approve        {status: "applied", change, proposal}
//	POST <platform>/settings-proposals/<id>/decline        {status: "declined", proposal}
//	GET  <platform>/settings-agent                         {chats: [<conversation id>]}
//	PUT  <platform>/settings-agent                         {conv} -> {chats}: names the Settings window's own chat
//	                                                       the person's page: a page session, same origin
//	GET  /api/cli/settings                                 {settings: [Setting]}
//	POST /api/cli/settings/set                             {session, key, value, why, wait, id?} ->
//	                                                       {status: applied|waiting|declined|expired, change?, proposal?}
//	                                                       an agent of this machine: the cli-token (harnessly settings)
//
// The chat an agent's request comes from is told by its connection, never by what the request says: only the Settings
// window's own chat changes a setting without the person's yes.

// settingsWaitMax is the longest one wait request holds; the CLI asks again until the person answers.
const settingsWaitMax = 50 * time.Second

// settingsChats are the chats the Settings window made for its own agent, kept across restarts.
type settingsChats struct {
	path string

	mu sync.Mutex
}

const settingsChatsKept = 20

func (s *settingsChats) load() []string {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var f struct {
		Chats []string `json:"chats"`
	}
	_ = json.Unmarshal(raw, &f)
	return f.Chats
}

func (s *settingsChats) list() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.load()...)
}

func (s *settingsChats) has(conv string) bool {
	return conv != "" && slices.Contains(s.list(), conv)
}

func (s *settingsChats) add(conv string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chats := s.load()
	if !slices.Contains(chats, conv) {
		chats = append(chats, conv)
	}
	if len(chats) > settingsChatsKept {
		chats = chats[len(chats)-settingsChatsKept:]
	}
	raw, _ := json.Marshal(map[string]any{"chats": chats})
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return nil, err
	}
	return chats, os.Rename(tmp, s.path)
}

var (
	runtimeSettingsMu  sync.Mutex
	runtimeSettingsAll = map[string]*wire.FileRuntimeSettings{}
	settingsCtlMu      sync.Mutex
	settingsCtlAll     = map[string]*settingsctl.Service{}
)

// runtimeSettingsFor is the process's one store of a data folder's runtime settings: the page and the catalog change
// the same file under the same lock.
func runtimeSettingsFor(dataDir string) *wire.FileRuntimeSettings {
	runtimeSettingsMu.Lock()
	defer runtimeSettingsMu.Unlock()
	if s := runtimeSettingsAll[dataDir]; s != nil {
		return s
	}
	s := &wire.FileRuntimeSettings{Path: filepath.Join(dataDir, "v2", "runtime-settings.json")}
	runtimeSettingsAll[dataDir] = s
	return s
}

// settingsCtlOf is the catalog of a data folder when the routes made one; nil before that.
func settingsCtlOf(dataDir string) *settingsctl.Service {
	settingsCtlMu.Lock()
	defer settingsCtlMu.Unlock()
	return settingsCtlAll[dataDir]
}

func onOff(v wire.OnOff) string {
	if v == wire.Off {
		return settingsctl.Off
	}
	return settingsctl.On
}

func boolOnOff(b bool) string {
	if b {
		return settingsctl.On
	}
	return settingsctl.Off
}

// runtimeSettingValues are the runtime settings as the catalog's values, by key.
func runtimeSettingValues(s wire.RuntimeSettings) map[string]string {
	del := s.DelegationOf()
	return map[string]string{
		"runtime.idleStop":               string(s.IdleStop),
		"runtime.limitContinue":          onOff(s.LimitContinue),
		"runtime.restartContinue":        onOff(s.RestartContinue),
		"runtime.delegation.maxChildren": strconv.Itoa(del.MaxChildren),
		"runtime.delegation.anyChat":     boolOnOff(del.AnyChat),
	}
}

// journaledRuntimeSettings is the runtime settings as the page saves them, with every change written into the journal
// as the person's.
type journaledRuntimeSettings struct {
	inner   *wire.FileRuntimeSettings
	dataDir string
}

func (j journaledRuntimeSettings) RuntimeSettings() wire.RuntimeSettings {
	return j.inner.RuntimeSettings()
}

func (j journaledRuntimeSettings) SetRuntimeSettings(s wire.RuntimeSettings) (wire.RuntimeSettings, error) {
	before := runtimeSettingValues(j.inner.RuntimeSettings())
	got, err := j.inner.SetRuntimeSettings(s)
	if svc := settingsCtlOf(j.dataDir); err == nil && svc != nil {
		after := runtimeSettingValues(got)
		for key, old := range before {
			svc.Record(key, old, after[key], settingsctl.Actor{Kind: settingsctl.ByPerson}, settingsctl.ViaPage)
		}
	}
	return got, err
}

const backgroundAsNow = "default"

func settingsItems(d deps) []settingsctl.Item {
	rs := runtimeSettingsFor(d.DataDir)
	runner := bgRunnerFor(d)
	providers := extProvidersFor(d)
	chats := locale.Pick("Chats", "Чаты")
	delegation := locale.Pick("Delegation", "Делегирование")
	background := locale.Pick("Background tasks", "Фоновые задачи")

	runtimeValue := func(key string) func() (string, error) {
		return func() (string, error) { return runtimeSettingValues(rs.RuntimeSettings())[key], nil }
	}
	change := func(edit func(*wire.RuntimeSettings)) error {
		s := rs.RuntimeSettings()
		edit(&s)
		if err := s.Validate(); err != nil {
			return err
		}
		_, err := rs.SetRuntimeSettings(s)
		return err
	}
	toOnOff := func(v string) wire.OnOff {
		if v == settingsctl.Off {
			return wire.Off
		}
		return wire.On
	}
	idleLabels := map[wire.IdleStop]string{
		wire.IdleStopOff: locale.Pick("never", "никогда"),
		wire.IdleStop1h:  locale.Pick("after 1 hour", "через 1 час"),
		wire.IdleStop2h:  locale.Pick("after 2 hours", "через 2 часа"),
		wire.IdleStop6h:  locale.Pick("after 6 hours", "через 6 часов"),
	}
	items := []settingsctl.Item{
		{Key: "runtime.idleStop", Section: chats, Kind: settingsctl.KindEnum,
			Title:       locale.Pick("Stop an idle chat", "Останавливать чат без работы"),
			Description: locale.Pick("After how long without work a chat's agent is stopped to free the machine. The chat stays and starts again with the next message.", "Через сколько без работы агент чата останавливается, чтобы освободить машину. Чат остаётся и запускается снова со следующим сообщением."),
			Options: func() []settingsctl.Option {
				out := []settingsctl.Option{}
				for _, v := range wire.IdleStops {
					out = append(out, settingsctl.Option{Value: string(v), Label: idleLabels[v]})
				}
				return out
			},
			Get: runtimeValue("runtime.idleStop"),
			Set: func(v string) error { return change(func(s *wire.RuntimeSettings) { s.IdleStop = wire.IdleStop(v) }) }},
		{Key: "runtime.limitContinue", Section: chats, Kind: settingsctl.KindBool,
			Title:       locale.Pick("Go on after the usage limit", "Продолжать после лимита"),
			Description: locale.Pick("A chat held on its usage limit goes on by itself when the limit resets.", "Чат, остановленный лимитом, сам продолжает работу, когда лимит обновится."),
			Get:         runtimeValue("runtime.limitContinue"),
			Set:         func(v string) error { return change(func(s *wire.RuntimeSettings) { s.LimitContinue = toOnOff(v) }) }},
		{Key: "runtime.restartContinue", Section: chats, Kind: settingsctl.KindBool,
			Title:       locale.Pick("Go on after a restart", "Продолжать после перезапуска"),
			Description: locale.Pick("A chat a restart of the machine cut short goes on by itself.", "Чат, который прервал перезапуск машины, сам продолжает работу."),
			Get:         runtimeValue("runtime.restartContinue"),
			Set:         func(v string) error { return change(func(s *wire.RuntimeSettings) { s.RestartContinue = toOnOff(v) }) }},
		{Key: "runtime.delegation.maxChildren", Section: delegation, Kind: settingsctl.KindNumber, Min: 0, Max: wire.DelegationMaxChildren,
			Title:       locale.Pick("Chats one agent may delegate", "Сколько чатов может поручить один агент"),
			Description: locale.Pick("The most chats one conversation's agent may start for its subtasks; 0 turns delegation off.", "Сколько чатов агент одного разговора может завести под свои подзадачи; 0 выключает делегирование."),
			Get:         runtimeValue("runtime.delegation.maxChildren"),
			Set: func(v string) error {
				n, _ := strconv.Atoi(v)
				return change(func(s *wire.RuntimeSettings) {
					del := s.DelegationOf()
					del.MaxChildren = n
					s.Delegation = &del
				})
			}},
		{Key: "runtime.delegation.anyChat", Section: delegation, Kind: settingsctl.KindBool,
			Title:       locale.Pick("Agents write to any chat", "Агенты пишут в любой чат"),
			Description: locale.Pick("An agent may write to any chat of the machine, not only to the ones it delegated.", "Агент может написать в любой чат машины, а не только в те, что поручил сам."),
			Get:         runtimeValue("runtime.delegation.anyChat"),
			Set: func(v string) error {
				return change(func(s *wire.RuntimeSettings) {
					del := s.DelegationOf()
					del.AnyChat = v == settingsctl.On
					s.Delegation = &del
				})
			}},
	}
	for _, f := range bgmodel.Features() {
		feature := f
		items = append(items, settingsctl.Item{Key: "background." + feature.ID, Section: background, Kind: settingsctl.KindEnum,
			Title: feature.Title,
			Description: feature.Description + " " + locale.Pick("The value is «default» (as now: the chat's own agent decides) or «<provider>/<model>» of a connected provider.",
				"Значение — «default» (как сейчас: решает сам агент чата) или «<провайдер>/<модель>» подключённого провайдера."),
			Options: func() []settingsctl.Option {
				out := []settingsctl.Option{{Value: backgroundAsNow, Label: locale.Pick("as now", "как сейчас")}}
				views, _ := providers.List()
				for _, p := range views {
					for _, m := range p.Models {
						out = append(out, settingsctl.Option{Value: p.ID + "/" + m.ID, Label: p.Name + " · " + m.ID})
					}
				}
				return out
			},
			// A model the provider does not list may still be named, as on the Settings screen.
			Free: func(v string) bool {
				c, ok := bgmodel.ParseChoice(v)
				if !ok {
					return false
				}
				_, err := providers.Get(c.Provider)
				return err == nil && !strings.ContainsAny(c.Model, " \t\r\n")
			},
			Get: func() (string, error) {
				if c, ok := runner.Store.Get(feature.ID); ok {
					return c.String(), nil
				}
				return backgroundAsNow, nil
			},
			Set: func(v string) error {
				if v == backgroundAsNow {
					_, err := runner.Pick(feature.ID, nil)
					return err
				}
				c, _ := bgmodel.ParseChoice(v)
				_, err := runner.Pick(feature.ID, &c)
				return err
			}})
	}
	return items
}

// settingsCtlFor is the process's one catalog of a data folder.
func settingsCtlFor(d deps) (*settingsctl.Service, *settingsChats) {
	items := settingsItems(d)
	settingsCtlMu.Lock()
	defer settingsCtlMu.Unlock()
	chats := &settingsChats{path: filepath.Join(d.DataDir, "v2", "settings-chats.json")}
	if s := settingsCtlAll[d.DataDir]; s != nil {
		return s, chats
	}
	s := &settingsctl.Service{
		Items:        items,
		Journal:      &settingsctl.Journal{Path: filepath.Join(d.DataDir, "v2", "settings-changes.jsonl")},
		SettingsChat: chats.has,
	}
	settingsCtlAll[d.DataDir] = s
	return s, chats
}

func settingsMessage(reason string) string {
	switch reason {
	case settingsctl.ReasonUnknownKey:
		return locale.Pick("There is no such setting.", "Такой настройки нет.")
	case settingsctl.ReasonBadValue:
		return locale.Pick("The setting does not take this value.", "Такое значение настройке не подходит.")
	case settingsctl.ReasonSame:
		return locale.Pick("The setting already has this value.", "У настройки уже такое значение.")
	case settingsctl.ReasonNotFound:
		return locale.Pick("This request is gone.", "Этого запроса уже нет.")
	case settingsctl.ReasonClosed:
		return locale.Pick("This request is answered already.", "На этот запрос уже ответили.")
	case settingsctl.ReasonTooMany, settingsctl.ReasonRateLimited:
		return locale.Pick("Too many changes wait for an answer. Answer them first.", "Слишком много изменений ждут ответа. Сначала ответьте на них.")
	}
	return locale.Pick("It did not work. Try again.", "Не получилось. Попробуйте ещё раз.")
}

func settingsRefusal(w http.ResponseWriter, err error) {
	r := settingsctl.RefusalOf(err)
	status := http.StatusBadRequest
	switch r.Reason {
	case settingsctl.ReasonUnknownKey, settingsctl.ReasonNotFound:
		status = http.StatusNotFound
	case settingsctl.ReasonSame, settingsctl.ReasonClosed:
		status = http.StatusConflict
	case settingsctl.ReasonTooMany, settingsctl.ReasonRateLimited:
		status = http.StatusTooManyRequests
	case settingsctl.ReasonFailed:
		status = http.StatusInternalServerError
		if r.Err != nil {
			agent.Log("warn", "settings: a change failed", agent.F{K: "error", V: r.Err.Error()})
		}
	}
	body := map[string]any{"error": r.Reason, "message": settingsMessage(r.Reason)}
	if r.Setting != nil {
		// What the setting takes, so that the caller can pick a value it does.
		body["setting"] = r.Setting
	}
	secretJSON(w, status, body)
}

func settingsCtlRoutes(d deps, c *core) []gate.Route {
	svc, chats := settingsCtlFor(d)
	callers := &callerCache{panes: c.cfg.Tmux, convOf: func(session string) string {
		id, _ := convOf(c, session)
		return id
	}}
	// actorOf is the chat a request comes from, by its connection.
	actorOf := func(r *http.Request) settingsctl.Actor {
		conv, _ := callers.of(r)
		if conv == "" {
			return settingsctl.Actor{Kind: settingsctl.ByAgent}
		}
		return settingsctl.Actor{Kind: settingsctl.ByAgent, Conv: conv, Title: secretIdentity(c, conv).Title}
	}
	under := func(r *http.Request, p string) (string, bool) {
		for _, prefix := range []string{p + "/", "/agents" + p + "/"} {
			if rest, ok := strings.CutPrefix(r.URL.Path, prefix); ok && rest != "" {
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
	cliOK := func(w http.ResponseWriter, r *http.Request, method string) bool {
		if d.CLI == nil || !d.CLI.OK(r) {
			secretJSON(w, http.StatusForbidden, map[string]string{"error": "Forbidden"})
			return false
		}
		if r.Method != method {
			secretJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
			return false
		}
		return true
	}
	read := func(w http.ResponseWriter, r *http.Request, out any) bool {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<10))
		if err != nil || json.Unmarshal(raw, out) != nil {
			secretJSON(w, http.StatusBadRequest, map[string]string{"error": "bad-request"})
			return false
		}
		return true
	}
	notAllowed := func(w http.ResponseWriter) {
		secretJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
	}
	person := settingsctl.Actor{Kind: settingsctl.ByPerson}
	catalogBase, changesBase := platformAPI+"/settings-catalog", platformAPI+"/settings-changes"
	proposalsBase, agentBase := platformAPI+"/settings-proposals", platformAPI+"/settings-agent"

	catalog := func(w http.ResponseWriter, r *http.Request) {
		if !pageOK(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			notAllowed(w)
			return
		}
		secretJSON(w, http.StatusOK, map[string]any{"settings": svc.List()})
	}
	catalogOne := func(w http.ResponseWriter, r *http.Request) {
		if !pageOK(w, r) {
			return
		}
		key, _ := under(r, catalogBase)
		switch r.Method {
		case http.MethodGet:
			got, err := svc.Get(key)
			if err != nil {
				settingsRefusal(w, err)
				return
			}
			secretJSON(w, http.StatusOK, map[string]any{"setting": got})
		case http.MethodPut:
			var b struct {
				Value string `json:"value"`
			}
			if !read(w, r, &b) {
				return
			}
			ch, err := svc.Apply(key, b.Value, person, settingsctl.ViaPage, "")
			if err != nil {
				settingsRefusal(w, err)
				return
			}
			secretJSON(w, http.StatusOK, map[string]any{"change": ch})
		default:
			notAllowed(w)
		}
	}
	changes := func(w http.ResponseWriter, r *http.Request) {
		if !pageOK(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			notAllowed(w)
			return
		}
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil || limit <= 0 || limit > settingsctl.JournalKept {
			limit = 50
		}
		secretJSON(w, http.StatusOK, map[string]any{"changes": svc.Journal.List(limit, r.URL.Query().Get("key"))})
	}
	proposals := func(w http.ResponseWriter, r *http.Request) {
		if !pageOK(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			notAllowed(w)
			return
		}
		secretJSON(w, http.StatusOK, map[string]any{"proposals": svc.Pending()})
	}
	answer := func(w http.ResponseWriter, r *http.Request) {
		if !pageOK(w, r) {
			return
		}
		rest, _ := under(r, proposalsBase)
		id, verb, ok := strings.Cut(rest, "/")
		if !ok || r.Method != http.MethodPost || (verb != "approve" && verb != "decline") {
			notAllowed(w)
			return
		}
		act := svc.Decline
		if verb == "approve" {
			act = svc.Approve
		}
		out, err := act(id)
		if err != nil {
			settingsRefusal(w, err)
			return
		}
		agent.Log("info", "settings: a proposal answered", agent.F{K: "id", V: id}, agent.F{K: "state", V: out.State})
		secretJSON(w, http.StatusOK, out)
	}
	settingsAgent := func(w http.ResponseWriter, r *http.Request) {
		if !pageOK(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			secretJSON(w, http.StatusOK, map[string]any{"chats": chats.list()})
		case http.MethodPut:
			var b struct {
				Conv string `json:"conv"`
			}
			if !read(w, r, &b) {
				return
			}
			if b.Conv == "" || len(b.Conv) > 200 {
				secretJSON(w, http.StatusBadRequest, map[string]string{"error": "bad-request"})
				return
			}
			list, err := chats.add(b.Conv)
			if err != nil {
				settingsRefusal(w, err)
				return
			}
			secretJSON(w, http.StatusOK, map[string]any{"chats": list})
		default:
			notAllowed(w)
		}
	}

	cliList := func(w http.ResponseWriter, r *http.Request) {
		if !cliOK(w, r, http.MethodGet) {
			return
		}
		secretJSON(w, http.StatusOK, map[string]any{"settings": svc.List()})
	}
	cliSet := func(w http.ResponseWriter, r *http.Request) {
		if !cliOK(w, r, http.MethodPost) {
			return
		}
		var b struct {
			Key   string  `json:"key"`
			Value string  `json:"value"`
			Why   string  `json:"why"`
			Wait  float64 `json:"wait"`
			// ID is the proposal a repeated request keeps waiting on.
			ID string `json:"id"`
		}
		if !read(w, r, &b) {
			return
		}
		wait := max(min(time.Duration(b.Wait*float64(time.Second)), settingsWaitMax), 0)
		if b.ID != "" {
			secretJSON(w, http.StatusOK, svc.Wait(r.Context(), b.ID, wait))
			return
		}
		by := actorOf(r)
		out, err := svc.Propose(b.Key, b.Value, b.Why, by)
		if err != nil {
			settingsRefusal(w, err)
			return
		}
		if out.State == settingsctl.StateWaiting && out.Proposal != nil {
			out = svc.Wait(r.Context(), out.Proposal.ID, wait)
		}
		agent.Log("info", "settings: an agent's change", agent.F{K: "key", V: b.Key}, agent.F{K: "conv", V: by.Conv}, agent.F{K: "state", V: out.State})
		secretJSON(w, http.StatusOK, out)
	}

	is := func(p string) func(*http.Request) bool { return func(r *http.Request) bool { return pathIs(r, p) } }
	below := func(p string) func(*http.Request) bool {
		return func(r *http.Request) bool { _, ok := under(r, p); return ok }
	}
	return []gate.Route{
		{Match: is(catalogBase), Handler: http.HandlerFunc(catalog)},
		{Match: below(catalogBase), Handler: http.HandlerFunc(catalogOne)},
		{Match: is(changesBase), Handler: http.HandlerFunc(changes)},
		{Match: is(proposalsBase), Handler: http.HandlerFunc(proposals)},
		{Match: below(proposalsBase), Handler: http.HandlerFunc(answer)},
		{Match: is(agentBase), Handler: http.HandlerFunc(settingsAgent)},
		{Match: is("/api/cli/settings"), Handler: http.HandlerFunc(cliList)},
		{Match: is("/api/cli/settings/set"), Handler: http.HandlerFunc(cliSet)},
	}
}
