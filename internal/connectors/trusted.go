package connectors

import (
	"encoding/json"
	"errors"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Trusted is a recipient the person let the agent reach without the card: their own tick on a card, never the
// agent's. It lives in dir/trusted.json.
type Trusted struct {
	Connection string    `json:"connection"`
	Key        string    `json:"key"`
	Label      string    `json:"label,omitempty"`
	At         time.Time `json:"at"`
}

// recipientKeys are the trust keys of whom a proposal reaches: each value of the action's recipient arguments, a list
// split at commas, an address without its name, lower case and without a leading @. Nil when the action names none.
func recipientKeys(a Action, args map[string]string) []string {
	var keys []string
	for _, name := range a.Recipients {
		for _, v := range strings.Split(args[name], ",") {
			if k := trustKey(v); k != "" && !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

func trustKey(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "<") {
		if a, err := mail.ParseAddress(v); err == nil {
			v = a.Address
		}
	}
	return strings.TrimPrefix(strings.ToLower(v), "@")
}

func (s *Service) trustedFile() string { return filepath.Join(s.dir, "trusted.json") }

// TrustedList is every trusted recipient, of every connection.
func (s *Service) TrustedList() ([]Trusted, error) {
	raw, err := os.ReadFile(s.trustedFile())
	if errors.Is(err, os.ErrNotExist) {
		return []Trusted{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Trusted
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) keepTrusted(list []Trusted) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.trustedFile() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.trustedFile())
}

// Trust marks the recipients of a connection as trusted. Only the person's page calls it.
func (s *Service) Trust(conn string, keys []string, label string) error {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	list, err := s.TrustedList()
	if err != nil {
		return err
	}
	for _, k := range keys {
		if k = trustKey(k); k == "" || slices.ContainsFunc(list, func(t Trusted) bool { return t.Connection == conn && t.Key == k }) {
			continue
		}
		l := label
		if len(keys) > 1 {
			l = k
		}
		list = append(list, Trusted{Connection: conn, Key: k, Label: l, At: s.now().UTC()})
	}
	if err := s.keepTrusted(list); err != nil {
		return err
	}
	s.changed()
	return nil
}

// Untrust takes a recipient's trust back: the next proposal to them waits for the card again.
func (s *Service) Untrust(conn, key string) error {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	list, err := s.TrustedList()
	if err != nil {
		return err
	}
	key = trustKey(key)
	n := len(list)
	list = slices.DeleteFunc(list, func(t Trusted) bool { return t.Connection == conn && t.Key == key })
	if len(list) == n {
		return ErrUnknown
	}
	if err := s.keepTrusted(list); err != nil {
		return err
	}
	s.changed()
	return nil
}

// allTrusted: the action names recipients and the person trusted every one of them on this connection.
func (s *Service) allTrusted(conn string, keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	list, err := s.TrustedList()
	if err != nil {
		return false
	}
	for _, k := range keys {
		if !slices.ContainsFunc(list, func(t Trusted) bool { return t.Connection == conn && t.Key == k }) {
			return false
		}
	}
	return true
}
