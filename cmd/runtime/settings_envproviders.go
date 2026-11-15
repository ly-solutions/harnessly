package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/extproviders"
	"github.com/ly-solutions/harnessly/internal/hubproto"
	"github.com/ly-solutions/harnessly/internal/locale"
	"github.com/ly-solutions/harnessly/internal/settingsctl"
)

// The model providers of the organization's environment as an agent proposes them. Nothing is changed from
// this machine: the proposal goes to the hub and waits there for an administrator of the organization, who applies or
// declines it under their own session; an applied one comes to every machine of the organization with the environment
// list. The hub is asked twice: first whether it would keep the proposal (so that a wrong one is refused in words
// before any card), then to keep it.

const envProposeTimeout = 60 * time.Second

// envProviderValue is what the setting is given: a provider of the organization, found by its name.
type envProviderValue struct {
	Name   string `json:"name"`
	Hint   string `json:"hint,omitempty"`
	Remove bool   `json:"remove,omitempty"`
	extproviders.ManagedSource
}

// envProposer asks the hub through the machine's agent.
type envProposer func(p hubproto.EnvProposal) (hubproto.EnvProposalResult, error)

func agentEnvProposer(dataDir string) envProposer {
	header := os.Getenv("RUNTIME_AGENT_TOKEN_HEADER")
	return func(p hubproto.EnvProposal) (hubproto.EnvProposalResult, error) {
		var out hubproto.EnvProposalResult
		status, body, err := agent.CallLocal(nil, dataDir, header, "/env/propose", p, envProposeTimeout)
		if err != nil {
			return out, err
		}
		if status != http.StatusOK || json.Unmarshal(body, &out) != nil || out.Status == 0 {
			return out, errors.New("the machine's agent did not pass the proposal on")
		}
		return out, nil
	}
}

// parseEnvProvider reads a value; the words say what is wrong with one that is not taken.
func parseEnvProvider(v string) (hubproto.EnvProviderProposal, extproviders.ManagedSource, string) {
	var in envProviderValue
	if err := json.Unmarshal([]byte(v), &in); err != nil {
		return hubproto.EnvProviderProposal{}, extproviders.ManagedSource{}, locale.Pick("The value is not the JSON of a provider.", "Значение — не JSON провайдера.")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return hubproto.EnvProviderProposal{}, extproviders.ManagedSource{}, locale.Pick("The provider needs a name.", "У провайдера нужно название (\"name\").")
	}
	if in.Remove {
		return hubproto.EnvProviderProposal{Name: name, Remove: true}, extproviders.ManagedSource{}, ""
	}
	src, err := in.ManagedSource.Clean()
	if err != nil {
		return hubproto.EnvProviderProposal{}, extproviders.ManagedSource{}, extProviderMessage(extproviders.ReasonOf(err))
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return hubproto.EnvProviderProposal{}, extproviders.ManagedSource{}, extProviderMessage(extproviders.ReasonFailed)
	}
	return hubproto.EnvProviderProposal{Name: name, Hint: strings.TrimSpace(in.Hint), Source: raw}, src, ""
}

// envProvidersItem is the setting; propose asks the hub.
func envProvidersItem(svc *extproviders.Service, propose envProposer) settingsctl.Item {
	unreachable := locale.Pick("The hub did not answer; nothing was changed. Try again later.", "Хаб не ответил, ничего не изменено. Попробуйте позже.")
	// The check of a value is asked for more than once in a row (does it fit, why not): the hub is asked once.
	var mu sync.Mutex
	var lastValue, lastWhy string
	var lastAt time.Time
	why := func(v string) string {
		p, _, said := parseEnvProvider(v)
		if said != "" {
			return said
		}
		mu.Lock()
		defer mu.Unlock()
		if v == lastValue && time.Since(lastAt) < 5*time.Second {
			return lastWhy
		}
		said = ""
		res, err := propose(hubproto.EnvProposal{Check: true, Provider: p})
		switch {
		case err != nil:
			said = unreachable
		case res.Status != http.StatusOK:
			if said = res.Message; said == "" {
				said = unreachable
			}
		}
		lastValue, lastWhy, lastAt = v, said, time.Now()
		return said
	}
	return settingsctl.Item{Key: "environment.providers", Section: locale.Pick("Team environment", "Окружение команды"), Kind: settingsctl.KindText,
		Title: locale.Pick("Model providers of the organization", "Провайдеры моделей организации"),
		Description: locale.Pick(
			`The model providers the organization's environment gives every member's machine, as a JSON list (read only here). To add one or change the one of that name, set a JSON object: {"name", "hint"?, "api": "anthropic"|"openai", "baseUrl", "models"?: [..], "key"?: "<secret>", "env"?: ["NAME=value"], "secretEnv"?: ["NAME=<secret>"]}; {"name", "remove": true} takes it out of the environment. <secret> is a name every member has in their own safe (each is asked for it, for that server only) or a reference vault:<path>#<key> to the organization's safe; a value of a key or a token is never written here. Setting it changes nothing by itself: the hub keeps it as a proposal, and an administrator of the organization applies or declines it on the hub's environment screen, where they see what changes, the machine and the chat it came from. Tell the person that it waits for the administrator.`,
			`Провайдеры моделей, которые окружение организации раздаёт на машины всех участников, списком JSON (здесь только для чтения). Добавить провайдера или изменить провайдера с таким названием — задать объект JSON: {"name", "hint"?, "api": "anthropic"|"openai", "baseUrl", "models"?: [..], "key"?: "<секрет>", "env"?: ["ИМЯ=значение"], "secretEnv"?: ["ИМЯ=<секрет>"]}; {"name", "remove": true} уберёт его из окружения. <секрет> — имя, под которым секрет лежит в сейфе у каждого участника (каждого спросят о нём, только для этого сервера), или ссылка vault:<путь>#<ключ> на сейф организации; само значение ключа или токена сюда не пишут. Само по себе это ничего не меняет: хаб сохранит это как предложение, а применит или отклонит его админ организации на экране окружения в хабе — там видно, что меняется, с какой машины и из какого чата оно пришло. Скажите человеку, что предложение ждёт админа.`),
		Get: func() (string, error) {
			list, err := svc.Store.List()
			if err != nil {
				return "", err
			}
			out := []map[string]any{}
			for _, p := range list {
				if p.Origin == "" {
					continue
				}
				models := []string{}
				for _, m := range p.Models {
					models = append(models, m.ID)
				}
				out = append(out, map[string]any{"name": p.Name, "api": p.API, "baseUrl": p.BaseURL, "models": models})
			}
			b, err := json.Marshal(out)
			return string(b), err
		},
		Free: func(v string) bool { return why(v) == "" },
		Why:  why,
		Label: func(v string) string {
			p, src, said := parseEnvProvider(v)
			if said != "" {
				return v
			}
			for_ := locale.Pick("propose to the organization's administrator, for every member: ", "предложить админу организации, для всех участников: ")
			if p.Remove {
				return for_ + locale.Pick("remove ", "убрать ") + p.Name
			}
			out := for_ + p.Name + " → " + src.BaseURL
			if refs := src.SecretRefs(); len(refs) > 0 {
				out += locale.Pick("; secrets: ", "; секреты: ") + strings.Join(refs, ", ")
			}
			var vars []string
			for _, line := range append(append([]string{}, src.Env...), src.SecretEnv...) {
				n, _, _ := strings.Cut(line, "=")
				vars = append(vars, strings.TrimSpace(n))
			}
			if len(vars) > 0 {
				out += locale.Pick("; variables: ", "; переменные: ") + strings.Join(vars, ", ")
			}
			return out
		},
		// A provider of the organization replaces nothing of what the machine shows now: the card strikes nothing out.
		Replaces: func(string) (string, bool) { return "", true },
		SetBy: func(v string, by settingsctl.Actor) (string, error) {
			p, _, said := parseEnvProvider(v)
			if said != "" {
				return "", &settingsctl.Said{Message: said}
			}
			res, err := propose(hubproto.EnvProposal{Conv: by.Conv, ConvTitle: by.Title, Provider: p})
			if err != nil {
				return "", &settingsctl.Said{Message: unreachable}
			}
			// Only «kept, waits for an administrator» is a yes: a hub that answers anything else kept nothing.
			if res.Status != http.StatusAccepted {
				msg := res.Message
				if msg == "" {
					msg = unreachable
				}
				return "", &settingsctl.Said{Message: msg}
			}
			mu.Lock()
			lastValue = ""
			mu.Unlock()
			return locale.Pick("the proposal waits for an administrator of the organization, who applies or declines it in the hub (Environment → Proposals of agents); the environment is unchanged until then.",
				"предложение ждёт админа организации: он применит или отклонит его в хабе (Окружение → Предложения агентов); до этого окружение не меняется."), nil
		}}
}
