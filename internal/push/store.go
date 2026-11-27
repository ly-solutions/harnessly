package push

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
)

// SubscriptionsFile is the browsers' push subscriptions in the data folder.
const SubscriptionsFile = "push-subscriptions.json"

// VisibilityTTL: a device that has not said it is visible for this long counts as not looked at.
const VisibilityTTL = 60 * time.Second

// Subscription is one browser's push subscription, with the device that made it.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	DeviceID string `json:"deviceId,omitempty"`
	Mobile   *bool  `json:"mobile,omitempty"`
	// Name is the device as the person knows it, from its browser («iPhone · Safari»).
	Name string `json:"name,omitempty"`
	// Via is the machine the page reached this runtime through (its ?machine=): a notification's buttons answer the
	// same way. Empty when the page talks to the runtime directly.
	Via string `json:"via,omitempty"`
	// CreatedAt, LastSeen (the page last registered) and LastActive (the person last looked at the app on it), RFC 3339.
	CreatedAt  string `json:"createdAt,omitempty"`
	LastSeen   string `json:"lastSeen,omitempty"`
	LastActive string `json:"lastActive,omitempty"`
}

// Store reads and edits the subscriptions file and remembers which devices are on screen. Every writer of this process
// goes through one Store and its lock (the sender dropping a gone subscription, the page's subscribe route); a writer
// in another process (the previous runtime's routes, while their slice is off) replaces the file whole, as these do.
type Store struct {
	Dir string
	Now func() time.Time

	mu      sync.Mutex
	visible map[string]time.Time
	// looking is the chat each visible device shows, as its page last said.
	looking map[string]string
	// activeKept is when a device's lastActive was last written to the file.
	activeKept map[string]time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) path() string { return filepath.Join(s.Dir, SubscriptionsFile) }

func (s *Store) read() []Subscription {
	raw, err := os.ReadFile(s.path())
	if err != nil {
		return nil
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	out := make([]Subscription, 0, len(items))
	for _, it := range items {
		var sub Subscription
		if json.Unmarshal(it, &sub) != nil {
			continue
		}
		out = append(out, sub)
	}
	return out
}

// Subscriptions are every stored subscription; none when the file is missing or unreadable.
func (s *Store) Subscriptions() []Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

// Remove drops the subscriptions of an endpoint (one the push service said is gone, or the page unsubscribed); the
// others are written back as they were.
func (s *Store) Remove(endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.items()
	kept := make([]any, 0, len(items))
	for _, it := range items {
		if e, ok := endpointOf(it); !ok || e != endpoint {
			kept = append(kept, it)
		}
	}
	if len(kept) == len(items) {
		return nil
	}
	return s.write(kept)
}

// items is the file as the page's writer left it, every record with its keys in their order.
func (s *Store) items() []any {
	raw, err := os.ReadFile(s.path())
	if err != nil {
		return nil
	}
	v, err := jsvalue.Parse(raw)
	if err != nil {
		return nil
	}
	items, _ := v.([]any)
	return items
}

func endpointOf(item any) (any, bool) {
	o, ok := item.(*jsvalue.Object)
	if !ok {
		return nil, false
	}
	return o.Get("endpoint")
}

// write replaces the file, two-space indented like the page's writer (0600, through a temporary file).
func (s *Store) write(items []any) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	if items == nil {
		items = []any{}
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, jsvalue.Stringify(items, "  "), 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Add stores a browser's subscription ({endpoint, keys} as the browser gave them) with the device that made it.
func (s *Store) Add(endpoint string, keys any, deviceID string, mobile *bool) error {
	return s.AddDevice(endpoint, keys, deviceID, mobile, DeviceInfo{})
}

// DeviceInfo is what the subscribe request says of its device besides its id.
type DeviceInfo struct {
	// UserAgent names the device (DeviceName).
	UserAgent string
	// Via is the ?machine= the page reached the runtime through.
	Via string
}

// seenEvery: a page load of an unchanged device writes lastSeen no oftener than this.
const seenEvery = time.Hour

// AddDevice stores a subscription as Add, with the device's name and the times it was seen. A device subscribing again
// replaces its earlier subscription and keeps when it was first stored and last active; an unchanged record is not
// written again within seenEvery (every page load registers).
func (s *Store) AddDevice(endpoint string, keys any, deviceID string, mobile *bool, info DeviceInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	stored := jsvalue.NewObject("endpoint", endpoint, "keys", keys)
	if deviceID != "" {
		stored.Set("deviceId", deviceID)
	}
	if mobile != nil {
		stored.Set("mobile", *mobile)
	}
	if name := DeviceName(info.UserAgent); name != "" {
		stored.Set("name", name)
	}
	if info.Via != "" {
		stored.Set("via", info.Via)
	}
	current := s.items()
	var prev *jsvalue.Object
	subs := make([]any, 0, len(current)+1)
	for _, it := range current {
		o, _ := it.(*jsvalue.Object)
		same := false
		if e, ok := endpointOf(it); ok && e == endpoint {
			same = true
		}
		if deviceID != "" && o != nil {
			if d, ok := o.Get("deviceId"); ok && d == deviceID {
				same = true
			}
		}
		if same {
			if prev == nil && o != nil {
				prev = o
			}
			continue
		}
		subs = append(subs, it)
	}
	created := now.Format(time.RFC3339)
	if prev != nil && prev.String("createdAt") != "" {
		created = prev.String("createdAt")
	}
	stored.Set("createdAt", created)
	if prev != nil {
		if a := prev.String("lastActive"); a != "" {
			stored.Set("lastActive", a)
		}
		// Unchanged and seen lately: nothing to write.
		if seen, err := time.Parse(time.RFC3339, prev.String("lastSeen")); err == nil && now.Sub(seen) < seenEvery {
			stored.Set("lastSeen", prev.String("lastSeen"))
			if jsvalue.Equal(prev, stored) {
				return nil
			}
		}
	}
	stored.Set("lastSeen", now.Format(time.RFC3339))
	return s.write(append(subs, stored))
}

// DeviceEndpoints is {endpoint} of every stored subscription, in the file's order.
func (s *Store) DeviceEndpoints() []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.items()
	out := make([]any, 0, len(items))
	for _, it := range items {
		o := jsvalue.NewObject()
		if e, ok := endpointOf(it); ok {
			o.Set("endpoint", e)
		}
		out = append(out, o)
	}
	return out
}

// MarkVisible: the device shows the app now.
func (s *Store) MarkVisible(deviceID string) { s.MarkLooking(deviceID, "") }

// activeEvery: a device on screen writes its lastActive to the file no oftener than this.
const activeEvery = 2 * time.Minute

// MarkLooking: the device shows the app now, with the chat conv open (empty: none, or the page did not say). The
// device's lastActive is kept in the file.
func (s *Store) MarkLooking(deviceID, conv string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.visible == nil {
		s.visible, s.looking, s.activeKept = map[string]time.Time{}, map[string]string{}, map[string]time.Time{}
	}
	now := s.now()
	s.visible[deviceID] = now
	s.looking[deviceID] = conv
	if now.Sub(s.activeKept[deviceID]) < activeEvery {
		return
	}
	s.activeKept[deviceID] = now
	items := s.items()
	changed := false
	for _, it := range items {
		if o, ok := it.(*jsvalue.Object); ok && o.String("deviceId") == deviceID && deviceID != "" {
			o.Set("lastActive", now.UTC().Format(time.RFC3339))
			changed = true
		}
	}
	if changed {
		_ = s.write(items)
	}
}

// MarkHidden: the device hid the app.
func (s *Store) MarkHidden(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.visible, deviceID)
	delete(s.looking, deviceID)
}

// Visible are the devices seen on screen within VisibilityTTL.
func (s *Store) Visible() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	out := map[string]bool{}
	for id, seen := range s.visible {
		if now.Sub(seen) > VisibilityTTL {
			delete(s.visible, id)
			continue
		}
		out[id] = true
	}
	return out
}

// SelectTargets: while someone has the app open on a device, only that device is notified (its page turns the push into
// a toast and the phone in the pocket stays quiet); nobody looking, every device is.
func SelectTargets(subs []Subscription, visible map[string]bool) []Subscription {
	if len(visible) == 0 {
		return subs
	}
	var watching []Subscription
	for _, s := range subs {
		if s.DeviceID != "" && visible[s.DeviceID] {
			watching = append(watching, s)
		}
	}
	if len(watching) > 0 {
		return watching
	}
	return subs
}
