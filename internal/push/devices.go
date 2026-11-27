package push

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DeviceName is a device as the person knows it, from its browser's User-Agent: «iPhone · Safari», «Android · Chrome»,
// «Mac · Chrome». Empty when the agent says nothing.
func DeviceName(ua string) string {
	if strings.TrimSpace(ua) == "" {
		return ""
	}
	os := ""
	switch {
	case strings.Contains(ua, "iPhone"):
		os = "iPhone"
	case strings.Contains(ua, "iPad"):
		os = "iPad"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "CrOS"):
		os = "ChromeOS"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		os = "Mac"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	browser := ""
	switch {
	case strings.Contains(ua, "Electron/"):
		browser = "harnessly"
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "EdgA/") || strings.Contains(ua, "EdgiOS/"):
		browser = "Edge"
	case strings.Contains(ua, "YaBrowser/"):
		browser = "Yandex"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "SamsungBrowser/"):
		browser = "Samsung Internet"
	case strings.Contains(ua, "CriOS/") || strings.Contains(ua, "Chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/") || os == "iPhone" || os == "iPad":
		browser = "Safari"
	}
	switch {
	case os != "" && browser != "":
		return os + " · " + browser
	case os != "":
		return os
	case browser != "":
		return browser
	}
	return "Browser"
}

// Device is a subscribed device as the agent and the page see it: never its endpoint or keys.
type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Mobile     bool   `json:"mobile"`
	CreatedAt  string `json:"createdAt,omitempty"`
	LastSeen   string `json:"lastSeen,omitempty"`
	LastActive string `json:"lastActive,omitempty"`
	// Visible: the person looks at the app on it now.
	Visible bool `json:"visible"`
	// Last: the device the person used most recently.
	Last bool `json:"last,omitempty"`
}

// deviceID is a subscription's device id, or one made from its endpoint for a record without it.
func deviceID(s Subscription) string {
	if s.DeviceID != "" {
		return s.DeviceID
	}
	return "ep-" + Topic(s.Endpoint)[:10]
}

// activity is when the person last used the device: its lastActive, else lastSeen.
func activity(s Subscription) time.Time {
	for _, v := range []string{s.LastActive, s.LastSeen} {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// recentFirst orders subscriptions by the device's last use, most recent first (stable for equal times).
func recentFirst(subs []Subscription) []Subscription {
	out := append([]Subscription(nil), subs...)
	sort.SliceStable(out, func(i, j int) bool { return activity(out[i]).After(activity(out[j])) })
	return out
}

// Devices are the subscribed devices, the most recently used first.
func (s *Store) Devices() []Device {
	subs := recentFirst(s.Subscriptions())
	visible := s.Visible()
	out := make([]Device, 0, len(subs))
	for i, sub := range subs {
		name := sub.Name
		if name == "" {
			name = "Browser"
		}
		out = append(out, Device{ID: deviceID(sub), Name: name, Mobile: sub.Mobile != nil && *sub.Mobile,
			CreatedAt: sub.CreatedAt, LastSeen: sub.LastSeen, LastActive: sub.LastActive,
			Visible: sub.DeviceID != "" && visible[sub.DeviceID], Last: i == 0})
	}
	return out
}

// ErrNoDevice: the selector names no subscribed device.
var ErrNoDevice = errors.New("no-device")

var selectorRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Pick are the subscriptions a selector names: "all", "last" (the device used most recently), "mobile" (the phone
// used most recently), or a device id.
func (s *Store) Pick(selector string) ([]Subscription, error) {
	subs := recentFirst(s.Subscriptions())
	if len(subs) == 0 {
		return nil, ErrNoDevice
	}
	switch selector {
	case "", "all":
		return subs, nil
	case "last":
		return subs[:1], nil
	case "mobile":
		for _, sub := range subs {
			if sub.Mobile != nil && *sub.Mobile {
				return []Subscription{sub}, nil
			}
		}
		return nil, ErrNoDevice
	}
	if !selectorRe.MatchString(selector) {
		return nil, ErrNoDevice
	}
	var out []Subscription
	for _, sub := range subs {
		if deviceID(sub) == selector {
			out = append(out, sub)
		}
	}
	if len(out) == 0 {
		return nil, ErrNoDevice
	}
	return out, nil
}

// Looking says whether some device shows the chat conv on screen now.
func (s *Store) Looking(conv string) bool {
	if conv == "" {
		return false
	}
	visible := s.Visible()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range visible {
		if s.looking[id] == conv {
			return true
		}
	}
	return false
}

// ApprovalTargets are the devices a proposal waiting for the person's yes goes to: none when the chat with its card is
// on a screen now; else the phone used most recently; no phone, every device.
func (s *Store) ApprovalTargets(conv string) []Subscription {
	if s.Looking(conv) {
		return nil
	}
	if subs, err := s.Pick("mobile"); err == nil {
		return subs
	}
	subs, _ := s.Pick("all")
	return subs
}
