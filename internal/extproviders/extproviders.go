// Package extproviders keeps the external model providers a person added by API: a name, an address, the kind of API it
// speaks (OpenAI-compatible or Anthropic-compatible) and the name of the secret of the machine's safe that holds its
// key. The key itself is never kept here and never leaves in an answer: the registry file holds a reference, and the
// value is read from the safe only to make a request to the provider.
//
// A provider is no chat profile (internal/providers): those are the vendor setups an agent CLI runs on. A provider of
// this registry is what the background features call (internal/bgmodel); one that speaks the Anthropic API is marked
// as fit for Claude CLI, for the day a chat may pick it.
package extproviders

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// API is the protocol a provider speaks.
type API string

const (
	// APIOpenAI: GET <base>/models and POST <base>/chat/completions, the key as a Bearer token.
	APIOpenAI API = "openai"
	// APIAnthropic: GET <base>/v1/models and POST <base>/v1/messages, the key in x-api-key.
	APIAnthropic API = "anthropic"
)

// Valid says whether a is a protocol the registry knows.
func (a API) Valid() bool { return a == APIOpenAI || a == APIAnthropic }

// Limits of what a person may save.
const (
	MaxProviders = 32
	MaxName      = 60
	MaxURL       = 300
	MaxKey       = 4096
	MaxModels    = 500
	MaxModelID   = 200
)

// Reasons of a refusal, as the page is told them.
const (
	ReasonBadName  = "bad-name"
	ReasonBadURL   = "bad-url"
	ReasonBadAPI   = "bad-api"
	ReasonBadKey   = "bad-key"
	ReasonBadModel = "bad-model"
	ReasonNotFound = "not-found"
	ReasonTooMany  = "too-many"
	ReasonFailed   = "failed"
)

// Refusal is a reason the caller is told.
type Refusal struct {
	Reason string
	Err    error
}

func (r *Refusal) Error() string {
	if r.Err != nil {
		return "extproviders: " + r.Reason + ": " + r.Err.Error()
	}
	return "extproviders: " + r.Reason
}

func (r *Refusal) Unwrap() error { return r.Err }

func refuse(reason string) error { return &Refusal{Reason: reason} }

// ReasonOf is the reason of any error of the registry.
func ReasonOf(err error) string {
	var r *Refusal
	if errors.As(err, &r) {
		return r.Reason
	}
	return ReasonFailed
}

// Model is one model of a provider, as the provider lists it.
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// Provider is one provider as the registry file keeps it.
type Provider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	API     API    `json:"api"`
	// Preset is the template it was made from ("" for an address of the person's own).
	Preset string `json:"preset,omitempty"`
	// KeyRef names the secret of the safe that holds the key; "" for a provider that takes none (a local server).
	KeyRef string  `json:"keyRef,omitempty"`
	Models []Model `json:"models"`
	// ModelsAt is when the models were last taken from the provider (ms); 0 when they never were.
	ModelsAt  int64 `json:"modelsAt,omitempty"`
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// View is a provider as the page sees it: everything but the name of its secret, and whether a key is saved.
type View struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	BaseURL  string  `json:"baseUrl"`
	Host     string  `json:"host"`
	API      API     `json:"api"`
	Preset   string  `json:"preset,omitempty"`
	HasKey   bool    `json:"hasKey"`
	Models   []Model `json:"models"`
	ModelsAt int64   `json:"modelsAt,omitempty"`
	// ClaudeCLI: the provider speaks the Anthropic API, so Claude CLI could run a chat on it.
	ClaudeCLI bool  `json:"claudeCli"`
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// ClaudeCLI says whether Claude CLI could run a chat on the provider.
func (p Provider) ClaudeCLI() bool { return p.API == APIAnthropic }

// Host is the host of the provider's address.
func (p Provider) Host() string {
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// HasModel says whether the provider lists a model of that id.
func (p Provider) HasModel(id string) bool {
	return slices.ContainsFunc(p.Models, func(m Model) bool { return m.ID == id })
}

// Preset is a template of the form: a provider the person only has to give a key for.
type Preset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	API     API    `json:"api"`
	// KeyURL is the page of the vendor where a key is made.
	KeyURL string `json:"keyUrl,omitempty"`
}

// PresetCustom is the template of an address of the person's own.
const PresetCustom = "custom"

var presets = []Preset{
	{ID: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", API: APIOpenAI, KeyURL: "https://platform.deepseek.com/api_keys"},
	{ID: "groq", Name: "Groq", BaseURL: "https://api.groq.com/openai/v1", API: APIOpenAI, KeyURL: "https://console.groq.com/keys"},
	{ID: "xai", Name: "xAI", BaseURL: "https://api.x.ai/v1", API: APIOpenAI, KeyURL: "https://console.x.ai"},
	{ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", API: APIOpenAI, KeyURL: "https://openrouter.ai/keys"},
	{ID: PresetCustom, Name: "Свой адрес", API: APIOpenAI},
}

// Presets are the templates of the form, in the order it lists them.
func Presets() []Preset { return slices.Clone(presets) }

func validPreset(id string) bool {
	return id == "" || slices.ContainsFunc(presets, func(p Preset) bool { return p.ID == id })
}

// CleanURL is a provider's address as it is kept: http or https, a host, no credentials, no query, no trailing slash.
func CleanURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxURL {
		return "", refuse(ReasonBadURL)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", refuse(ReasonBadURL)
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.EscapedPath(), "/"), nil
}

func cleanName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || utf8.RuneCountInString(s) > MaxName {
		return "", refuse(ReasonBadName)
	}
	return s, nil
}

func cleanKey(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) > MaxKey || strings.ContainsAny(s, "\r\n\x00") {
		return "", refuse(ReasonBadKey)
	}
	return s, nil
}

func cleanModels(in []Model) ([]Model, error) {
	if len(in) > MaxModels {
		return nil, refuse(ReasonBadModel)
	}
	out := make([]Model, 0, len(in))
	seen := map[string]bool{}
	for _, m := range in {
		m.ID = strings.TrimSpace(m.ID)
		m.Label = strings.TrimSpace(m.Label)
		if m.ID == "" || len(m.ID) > MaxModelID || len(m.Label) > MaxModelID || strings.ContainsAny(m.ID, " \t\r\n") {
			return nil, refuse(ReasonBadModel)
		}
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	return out, nil
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	s := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 24 {
		s = strings.Trim(s[:24], "-")
	}
	return s
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Secrets is the part of the machine's safe the registry uses: the keys go there under the names it makes.
type Secrets interface {
	Put(name, value string) error
	Reveal(name string) (string, error)
	Remove(name string) error
	Has(name string) bool
}

// Store is the registry file of one data folder: <Dir>/providers.json, written whole beside and renamed over.
type Store struct {
	Dir string
	Now func() time.Time

	mu sync.Mutex
}

type fileBody struct {
	Providers []Provider `json:"providers"`
}

func (s *Store) path() string { return filepath.Join(s.Dir, "providers.json") }

func (s *Store) now() int64 {
	if s.Now != nil {
		return s.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}

func (s *Store) read() (fileBody, error) {
	var body fileBody
	raw, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return body, nil
	}
	if err != nil {
		return body, err
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return fileBody{}, err
	}
	for i := range body.Providers {
		if body.Providers[i].Models == nil {
			body.Providers[i].Models = []Model{}
		}
	}
	return body, nil
}

func (s *Store) write(body fileBody) error {
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// List is the providers in the order they were added.
func (s *Store) List() ([]Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	return body.Providers, err
}

// Get is the provider of that id.
func (s *Store) Get(id string) (Provider, error) {
	list, err := s.List()
	if err != nil {
		return Provider{}, err
	}
	for _, p := range list {
		if p.ID == id {
			return p, nil
		}
	}
	return Provider{}, refuse(ReasonNotFound)
}

// change reads the file under the lock, lets edit change it and writes it back.
func (s *Store) change(edit func(*fileBody) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	if err != nil {
		return err
	}
	if err := edit(&body); err != nil {
		return err
	}
	return s.write(body)
}
