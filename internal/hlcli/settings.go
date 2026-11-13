package hlcli

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The settings of harnessly an agent may read and change when the person asks it to (internal/settingsctl). The agent
// of the Settings window's own chat changes a setting at once; from any other chat the person confirms the change on
// a card, and the command waits for the answer.

// settingsStep is the longest one request waits; the command asks again until its own wait is over.
const settingsStep = 45 * time.Second

type cliSetting struct {
	Key         string `json:"key"`
	Section     string `json:"section"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Options     []struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"options"`
	Min        *int   `json:"min"`
	Max        *int   `json:"max"`
	Value      string `json:"value"`
	ValueLabel string `json:"valueLabel"`
}

// takes is the values a setting takes, in a line.
func (s cliSetting) takes() string {
	switch s.Kind {
	case "number":
		if s.Min != nil && s.Max != nil {
			return "a number from " + strconv.Itoa(*s.Min) + " to " + strconv.Itoa(*s.Max)
		}
		return "a number"
	}
	vals := make([]string, 0, len(s.Options))
	for _, o := range s.Options {
		if o.Label != "" && o.Label != o.Value {
			vals = append(vals, o.Value+" ("+o.Label+")")
		} else {
			vals = append(vals, o.Value)
		}
	}
	return strings.Join(vals, ", ")
}

func (c *cli) printSetting(s cliSetting) {
	c.out("%s = %s\n  %s — %s\n  %s\n  takes: %s\n", s.Key, s.Value, s.Section, s.Title, s.Description, s.takes())
}

var settingsReasons = map[string]string{
	"unknown-setting": "no such setting: harnessly settings list shows the keys",
	"bad-value":       "the setting does not take this value",
	"same":            "the setting already has this value: nothing to change",
	"too-many":        "too many changes wait for the person's answer: let them answer first",
	"rate-limited":    "too many changes proposed from this chat in the last minutes: wait before the next",
}

func (c *cli) settingsRefused(status int, raw []byte) error {
	var r struct {
		Error   string      `json:"error"`
		Setting *cliSetting `json:"setting"`
	}
	_ = json.Unmarshal(raw, &r)
	why := settingsReasons[r.Error]
	if why == "" {
		return apiError(status, raw)
	}
	if r.Setting != nil {
		return fail("refused: %s — %s. %s is %s now and takes: %s", r.Error, why, r.Setting.Key, r.Setting.Value, r.Setting.takes())
	}
	return fail("refused: %s — %s", r.Error, why)
}

func (c *cli) settingsList() ([]cliSetting, []byte, error) {
	status, raw, err := c.call(http.MethodGet, "/api/cli/settings", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if status < 200 || status > 299 {
		return nil, nil, apiError(status, raw)
	}
	var out struct {
		Settings []cliSetting `json:"settings"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, err
	}
	return out.Settings, raw, nil
}

// settings: harnessly settings list | get KEY | set KEY VALUE --why "…".
func (c *cli) settings(args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list", "ls":
		if len(args) > 1 {
			return usageErr("settings list takes no arguments")
		}
		list, raw, err := c.settingsList()
		if err != nil {
			return err
		}
		if c.json {
			c.printRaw(raw)
			return nil
		}
		for _, s := range list {
			c.printSetting(s)
		}
		c.out("\nChange one with: harnessly settings set KEY VALUE --why \"what the person asked for\"\n")
		return nil
	case "get":
		if len(args) != 2 {
			return usageErr("settings get takes one key: harnessly settings get KEY")
		}
		list, _, err := c.settingsList()
		if err != nil {
			return err
		}
		for _, s := range list {
			if s.Key == args[1] {
				if c.json {
					raw, _ := json.Marshal(s)
					c.printRaw(raw)
					return nil
				}
				c.printSetting(s)
				return nil
			}
		}
		return fail("refused: unknown-setting — %s", settingsReasons["unknown-setting"])
	case "set":
		return c.settingsSet(args[1:])
	}
	return usageErr("settings: unknown command %s (harnessly settings --help)", args[0])
}

func (c *cli) settingsSet(args []string) error {
	var key, value, why string
	haveValue := false
	wait := 10 * time.Minute
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--why" || a == "--wait":
			if i+1 >= len(args) {
				return usageErr("%s needs a value", a)
			}
			i++
			if a == "--why" {
				why = args[i]
				break
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 || n > 1800 {
				return usageErr("--wait takes seconds, 0 to 1800")
			}
			wait = time.Duration(n) * time.Second
		case strings.HasPrefix(a, "--why="):
			why = strings.TrimPrefix(a, "--why=")
		case strings.HasPrefix(a, "--wait="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--wait="))
			if err != nil || n < 0 || n > 1800 {
				return usageErr("--wait takes seconds, 0 to 1800")
			}
			wait = time.Duration(n) * time.Second
		case strings.HasPrefix(a, "--"):
			return usageErr("settings set: unknown option %s", a)
		case key == "":
			key = a
		case !haveValue:
			value, haveValue = a, true
		default:
			return usageErr("settings set takes a key and a value: harnessly settings set KEY VALUE --why \"...\"")
		}
	}
	if key == "" || !haveValue || strings.TrimSpace(why) == "" {
		return usageErr("settings set needs the key, the value and why: harnessly settings set KEY VALUE --why \"what the person asked for\"")
	}
	deadline := time.Now().Add(wait)
	id := ""
	for {
		step := max(min(time.Until(deadline), settingsStep), 0)
		status, raw, err := c.call(http.MethodPost, "/api/cli/settings/set", nil,
			map[string]any{"session": c.session(), "key": key, "value": value, "why": why, "wait": step.Seconds(), "id": id})
		if err != nil {
			return err
		}
		if status < 200 || status > 299 {
			return c.settingsRefused(status, raw)
		}
		var r struct {
			Status string `json:"status"`
			Change *struct {
				Old string `json:"old"`
				New string `json:"new"`
			} `json:"change"`
			Proposal *struct {
				ID string `json:"id"`
			} `json:"proposal"`
		}
		_ = json.Unmarshal(raw, &r)
		if r.Proposal != nil {
			id = r.Proposal.ID
		}
		switch r.Status {
		case "applied":
			if c.json {
				c.printRaw(raw)
				return nil
			}
			if r.Change != nil {
				c.out("changed: %s was %s, is %s now\n", key, r.Change.Old, r.Change.New)
			} else {
				c.out("changed: %s is %s now (the person confirmed)\n", key, value)
			}
			return nil
		case "declined":
			return fail("declined: the person said no to %s = %s. Do not propose it again; ask them what they want instead", key, value)
		case "expired":
			return fail("expired: the person did not answer about %s = %s in time. Tell them the setting is unchanged", key, value)
		}
		if time.Until(deadline) <= 0 {
			if c.json {
				c.printRaw(raw)
				return nil
			}
			c.out("waiting: the person sees a card in this chat asking to confirm %s = %s; nothing is changed until they say yes. "+
				"Run the same command again to keep waiting.\n", key, value)
			return nil
		}
	}
}
