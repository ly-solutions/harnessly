package hlcli

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The person's devices that get harnessly's push notifications, and a push of the agent's own to them.

const pushHelp = `harnessly devices | push send | push ask

The person's phones and computers that get harnessly's notifications (they turned them on in the app).

  harnessly devices                                   id, name, phone or not, last used; "last" marks the device the
                                                      person used most recently, "on screen" one they look at now
  harnessly push send --title "…" [--body "…"] [--device <id>|last|mobile|all] [--url /path]
                                                      a notification to the person: a tap opens this chat (or --url,
                                                      a page of harnessly); --device all by default; at most 6 in 10
                                                      minutes
  harnessly push ask [--why "…"] [--wait seconds]     no device gets notifications: this chat shows the person a
                                                      card «Turn on notifications on this device» (one tap); waits
                                                      (120 s by default) and prints the devices that subscribed, or
                                                      "waiting"; with a device already subscribed, lists them at once

Send one only when the person asked to be told, or for what cannot wait: a finished turn and a question already notify
them. An action that needs their yes (harnessly connector act) notifies by itself, with «Send / Don't send» buttons:
--notify on act picks where that goes.
`

// devices lists the person's subscribed devices.
func (c *cli) devices(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		c.out("%s", pushHelp)
		return nil
	}
	status, raw, err := c.call(http.MethodGet, "/api/cli/push/devices", nil, nil)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return c.pushRefused(status, raw)
	}
	if c.json {
		c.printRaw(raw)
		return nil
	}
	var r struct {
		Devices []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Mobile     bool   `json:"mobile"`
			LastActive string `json:"lastActive"`
			LastSeen   string `json:"lastSeen"`
			Visible    bool   `json:"visible"`
			Last       bool   `json:"last"`
		} `json:"devices"`
	}
	_ = json.Unmarshal(raw, &r)
	if len(r.Devices) == 0 {
		c.out("No device gets notifications: the person turns them on in the app (Settings → Notifications), on each phone or computer.\n")
		return nil
	}
	for _, d := range r.Devices {
		kind := "computer"
		if d.Mobile {
			kind = "phone"
		}
		used := d.LastActive
		if used == "" {
			used = d.LastSeen
		}
		if used == "" {
			used = "-"
		}
		var marks []string
		if d.Last {
			marks = append(marks, "last")
		}
		if d.Visible {
			marks = append(marks, "on screen")
		}
		c.out("%s\t%s\t%s\tused %s\t%s\n", d.ID, d.Name, kind, used, strings.Join(marks, ", "))
	}
	return nil
}

// push: harnessly push send.
func (c *cli) push(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		c.out("%s", pushHelp)
		return nil
	}
	if args[0] == "ask" {
		return c.pushAsk(args[1:])
	}
	if args[0] != "send" {
		return usageErr("harnessly push send --title \"…\" [--body \"…\"] [--device id|last|mobile|all] [--url /path]")
	}
	body := map[string]string{"device": "all"}
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, value, inline := strings.Cut(a, "=")
		switch name {
		case "--title", "--body", "--device", "--url":
		default:
			return usageErr("push send: unknown option %s", a)
		}
		if !inline {
			if i+1 >= len(rest) {
				return usageErr("%s needs a value", name)
			}
			i++
			value = rest[i]
		}
		body[strings.TrimPrefix(name, "--")] = value
	}
	if strings.TrimSpace(body["title"]) == "" {
		return usageErr("harnessly push send --title \"…\" [--body \"…\"] [--device id|last|mobile|all] [--url /path]")
	}
	status, raw, err := c.call(http.MethodPost, "/api/cli/push/send", nil, body)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return c.pushRefused(status, raw)
	}
	if c.json {
		c.printRaw(raw)
		return nil
	}
	var r struct {
		Sent int `json:"sent"`
	}
	_ = json.Unmarshal(raw, &r)
	c.out("sent to %d device(s)\n", r.Sent)
	return nil
}

func (c *cli) pushRefused(status int, raw []byte) error {
	var r struct {
		Error, Message string
	}
	_ = json.Unmarshal(raw, &r)
	if r.Message != "" {
		return fail("refused: %s — %s", r.Error, r.Message)
	}
	return c.connectorRefused(status, raw)
}

// pushAsk: the chat offers the person to turn notifications on, and waits for a device to subscribe.
func (c *cli) pushAsk(args []string) error {
	why, wait := "", 120*time.Second
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if name != "--why" && name != "--wait" {
			return usageErr("push ask: unknown option %s", args[i])
		}
		if !inline {
			if i+1 >= len(args) {
				return usageErr("%s needs a value", name)
			}
			i++
			value = args[i]
		}
		if name == "--why" {
			why = value
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 900 {
			return usageErr("--wait takes seconds, 0 to 900")
		}
		wait = time.Duration(n) * time.Second
	}
	deadline := time.Now().Add(wait)
	for {
		step := max(min(time.Until(deadline), 45*time.Second), 0)
		status, raw, err := c.call(http.MethodPost, "/api/cli/push/ask", nil, map[string]any{"why": why, "wait": step.Seconds()})
		if err != nil {
			return err
		}
		if status < 200 || status > 299 {
			return c.pushRefused(status, raw)
		}
		var r struct {
			State   string `json:"state"`
			Devices []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"devices"`
		}
		_ = json.Unmarshal(raw, &r)
		if r.State == "subscribed" {
			if c.json {
				c.printRaw(raw)
				return nil
			}
			c.out("subscribed:\n")
			for _, d := range r.Devices {
				c.out("%s\t%s\n", d.ID, d.Name)
			}
			return nil
		}
		if !time.Now().Before(deadline) {
			return askExit{code: exitNoAnswer, msg: "waiting: the card «Turn on notifications» is in this chat; no device subscribed yet"}
		}
	}
}
