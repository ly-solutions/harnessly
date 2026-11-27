// Package obsidian is the Obsidian connector (docs/connectors.md, «Obsidian»): the notes of a vault on the person's
// own laptop or phone, as many as they chose to share, for the agent to read. The harnessly plugin of Obsidian
// (clients/obsidian) decides in the vault what is shared — folders, tags, a note's own «harnessly: true|false» — and
// pushes those notes, signed by the connection's secret, to the connection's address on the hub, which passes the
// request over the machine's link like any incoming webhook. Here, on the machine, the signature is checked, the notes
// are kept under the connection's folder, a note the person stopped sharing is removed, and the agent reads the
// list, a note with its links and backlinks, a search, the tags and the changes, all as data, not instructions. New
// notes, changed notes and new dated tasks are events without their text, for the hub's rules.
package obsidian

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"rsc.io/qr"

	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/connectors/webhook"
)

// ID names the connector.
const ID = "obsidian"

// Event types.
const (
	EventCreated = "note.created"
	EventChanged = "note.changed"
	EventTask    = "note.task"
)

const (
	// Proto is the version of the batch the plugin sends.
	Proto = 1
	// maxNote is the largest note taken; the plugin does not send a bigger one.
	maxNote = 512 << 10
	// maxNotes is the most notes a connection keeps.
	maxNotes = 50000
	// skew is how far a batch's time may be from now: an older one is a replay.
	skew = 10 * time.Minute
	// changeQuiet: a note gives one note.changed in this time, however often it is saved.
	changeQuiet = 15 * time.Minute
	// bulk: more events than this in one batch are one event with change=bulk.
	bulk = 100
)

// PluginBase is the public address the hub hands the plugin out at (<hub>/dl/obsidian); the runtime may set it.
var PluginBase = func() string {
	base := webhook.Base()
	if i := strings.Index(base, "/api/platform/"); i > 0 {
		return base[:i] + "/dl/obsidian"
	}
	return ""
}

var now = time.Now

type vaultConn struct{}

func init() { connectors.Register(vaultConn{}) }

func (vaultConn) Spec() connectors.Spec {
	return connectors.Spec{
		ID:    ID,
		Kind:  connectors.Files,
		Inbox: "folder", // the same notes and tasks as a vault folder on the machine (Folder connector)
		Title: connectors.T("Obsidian", "Obsidian"),
		Summary: connectors.T("Notes of your vault on any device: you choose in Obsidian what the agent sees",
			"Заметки хранилища с любого устройства: в Obsidian вы сами выбираете, что видит агент"),
		Fields: []connectors.Field{
			{ID: "secret", Type: connectors.FieldPassword, Secret: true, Generate: true,
				Label: connectors.T("Pairing key", "Ключ привязки")},
		},
		Events: []string{EventCreated, EventChanged, EventTask},
		Tools: []connectors.Tool{
			{Name: "notes", Summary: "the shared notes, newest first: modified, path, tags", Args: []connectors.Arg{
				{Name: "folder", Summary: "only this folder and its subfolders"},
				{Name: "tag", Summary: "only notes with this tag (without #)"},
				{Name: "limit", Summary: "how many, 100 by default"}}},
			{Name: "note", Summary: "one note's text with its tags, links and backlinks", Args: []connectors.Arg{
				{Name: "path", Summary: "the note's path in the vault, as notes shows it", Required: true}}},
			{Name: "search", Summary: "lines of the shared notes with all the words, and notes whose name has them", Args: []connectors.Arg{
				{Name: "query", Summary: "the words, case does not matter", Required: true},
				{Name: "limit", Summary: "how many lines, 50 by default"}}},
			{Name: "tags", Summary: "the tags of the shared notes with how many notes have each"},
			{Name: "links", Summary: "the notes a note links to and the notes that link to it", Args: []connectors.Arg{
				{Name: "path", Summary: "the note's path", Required: true}}},
			{Name: "changes", Summary: "what changed in a period: when, created/modified/removed, path", Args: []connectors.Arg{
				{Name: "since", Summary: "a time (2026-11-15T10:00:00Z) or how long ago (24h), 24h by default"},
				{Name: "limit", Summary: "how many, 100 by default"}}},
		},
	}
}

// address is the connection's address on the hub, without the secret.
func address(in connectors.Instance) string {
	base := strings.TrimRight(webhook.Base(), "/")
	if base == "" {
		return ""
	}
	return base + "/" + in.ID
}

// pairing is what the plugin takes to pair: the address and the key, as «hly1.<base64url JSON>».
type pairing struct {
	URL    string `json:"u"`
	Secret string `json:"s"`
	Name   string `json:"n,omitempty"`
	Plugin string `json:"p,omitempty"`
}

func code(in connectors.Instance) (string, error) {
	secret, err := in.Secret("secret")
	if err != nil || secret == "" {
		return "", connectors.Sayf("The pairing key is not kept: press «Reissue»")
	}
	addr := address(in)
	if addr == "" {
		return "", connectors.Sayf("This machine is not linked to a hub: Obsidian reaches it through the hub's address")
	}
	raw, _ := json.Marshal(pairing{URL: addr, Secret: secret, Name: in.Name, Plugin: PluginBase()})
	return "hly1." + base64.RawURLEncoding.EncodeToString(raw), nil
}

// link opens Obsidian and pairs its harnessly plugin.
func link(c string) string { return "obsidian://harnessly?pair=" + c }

func (vaultConn) Check(_ context.Context, in connectors.Instance) (connectors.Account, error) {
	if address(in) == "" {
		return connectors.Account{}, connectors.Sayf("This machine is not linked to a hub: Obsidian reaches it through the hub's address")
	}
	ix, err := store{dir: in.Dir}.load()
	if err != nil || ix.PairedAt.IsZero() {
		return connectors.Account{Label: "Obsidian"}, nil
	}
	label := "Obsidian: " + ix.Vault
	if ix.Device != "" {
		label += " (" + ix.Device + ")"
	}
	return connectors.Account{Label: fmt.Sprintf("%s · %d", label, len(ix.Notes))}, nil
}

// Reveal is what the person sees once: the link that pairs Obsidian, and the same as a code to paste.
func (vaultConn) Reveal(in connectors.Instance) ([]connectors.Shown, error) {
	c, err := code(in)
	if err != nil {
		return nil, err
	}
	return []connectors.Shown{
		{Label: connectors.T("Link for Obsidian", "Ссылка для Obsidian"), Value: link(c)},
		{Label: connectors.T("Pairing code", "Код привязки"), Value: c},
	}, nil
}

// Next is the step until the plugin paired: install it, then open the link (or scan the code with the phone). The
// link carries the key, so it is shown only on the person's own page, and only until the first batch came.
func (vaultConn) Next(_ context.Context, in connectors.Instance) (connectors.Step, bool) {
	if in.Dir == "" {
		return connectors.Step{}, false
	}
	ix, err := (store{dir: in.Dir}).load()
	if err == nil && !ix.PairedAt.IsZero() {
		return connectors.Step{}, false
	}
	c, err := code(in)
	if err != nil {
		return connectors.Step{Text: connectors.T(connectors.Say(err), connectors.Say(err))}, true
	}
	plugin := PluginBase()
	text := connectors.T(
		"Put the Harnessly plugin into Obsidian ("+plugin+"/harnessly-obsidian.zip, unpack it into <vault>/.obsidian/plugins and turn it on), then open the link on the device with the vault, or scan the code with the phone.",
		"Поставьте плагин Harnessly в Obsidian ("+plugin+"/harnessly-obsidian.zip — распакуйте в <хранилище>/.obsidian/plugins и включите), затем откройте ссылку на устройстве с хранилищем или отсканируйте код телефоном.")
	step := connectors.Step{Text: text, URL: link(c)}
	if img, err := qrImage(link(c)); err == nil {
		step.Image = img
	}
	return step, true
}

func qrImage(text string) (string, error) {
	q, err := qr.Encode(text, qr.L)
	if err != nil {
		return "", err
	}
	q.Scale = 4
	img, err := png.Decode(bytes.NewReader(q.PNG()))
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// batch is what the plugin sends: the notes to keep, the ones to drop, and at the end of a full sync the whole list
// of what is shared, in parts.
type batch struct {
	V        int      `json:"v"`
	ID       string   `json:"id"`
	At       int64    `json:"at"`
	Vault    string   `json:"vault"`
	Device   string   `json:"device"`
	Plugin   string   `json:"plugin"`
	Upsert   []upsert `json:"upsert"`
	Delete   []string `json:"delete"`
	Manifest *struct {
		Sync  string   `json:"sync"`
		Part  int      `json:"part"`
		Parts int      `json:"parts"`
		Paths []string `json:"paths"`
	} `json:"manifest"`
}

type upsert struct {
	Path  string   `json:"path"`
	Text  string   `json:"text"`
	MTime int64    `json:"mtime"`
	CTime int64    `json:"ctime"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Links []string `json:"links"`
}

// verify checks the batch is signed by the connection's key: an HMAC-SHA256 of the body in X-Signature-256.
func verify(in connectors.Instance, req connectors.Incoming) error {
	secret, err := in.Secret("secret")
	if err != nil || secret == "" || req.Path != "" {
		return connectors.ErrUnauthorized
	}
	given := strings.TrimPrefix(strings.TrimSpace(req.Header.Get("X-Signature-256")), "sha256=")
	raw, err := hex.DecodeString(given)
	if err != nil || len(raw) != sha256.Size {
		return connectors.ErrUnauthorized
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(req.Body)
	if !hmac.Equal(raw, m.Sum(nil)) {
		return connectors.ErrUnauthorized
	}
	return nil
}

func ms(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

func short(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func (vaultConn) Receive(_ context.Context, in connectors.Instance, req connectors.Incoming) ([]connectors.Event, error) {
	if len(req.Body) > webhook.MaxBody {
		return nil, connectors.ErrBadArgs
	}
	if err := verify(in, req); err != nil {
		return nil, err
	}
	var b batch
	if err := json.Unmarshal(req.Body, &b); err != nil || b.V != Proto || b.ID == "" || len(b.ID) > 64 {
		return nil, fmt.Errorf("%w: not a batch of the plugin", connectors.ErrBadArgs)
	}
	t := now()
	if at := ms(b.At); at.IsZero() || at.Before(t.Add(-skew)) || at.After(t.Add(skew)) {
		return nil, connectors.ErrUnauthorized
	}
	if in.Dir == "" {
		return nil, fmt.Errorf("%w: the connection has no folder", connectors.ErrBadArgs)
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	st := store{dir: in.Dir}
	ix, err := st.load()
	if err != nil {
		return nil, err
	}
	if ix.seenID(b.ID) {
		return nil, connectors.ErrDuplicate
	}
	if ix.PairedAt.IsZero() {
		ix.PairedAt = t.UTC()
	}
	ix.Vault, ix.Device, ix.Plugin = short(b.Vault, 120), short(b.Device, 120), short(b.Plugin, 20)
	ix.SyncedAt = t.UTC()

	var events []connectors.Event
	var journal []change
	learned := ix.Learned
	for _, u := range b.Upsert {
		p, ok := cleanPath(u.Path)
		if !ok || len(u.Text) > maxNote || !utf8.ValidString(u.Text) {
			continue
		}
		sum := sha256.Sum256([]byte(u.Text))
		hash := hex.EncodeToString(sum[:])
		var tags, links []string
		for _, tag := range u.Tags {
			if c := cleanTag(tag); c != "" && !contains(tags, c) && len(tags) < 50 {
				tags = append(tags, c)
			}
		}
		for _, l := range u.Links {
			if c, ok := cleanPath(l); ok && !contains(links, c) && len(links) < 500 {
				links = append(links, c)
			}
		}
		old := ix.Notes[p]
		if old != nil && old.Hash == hash {
			// The same text with other links, tags or title (a note it links to became private): no event.
			old.Title, old.Tags, old.Links = short(u.Title, 200), tags, links
			continue
		}
		if old == nil && len(ix.Notes) >= maxNotes {
			continue
		}
		var oldText string
		if old != nil {
			oldText, _ = st.readNote(p)
		}
		if err := st.writeNote(p, u.Text); err != nil {
			return nil, err
		}
		n := &note{Hash: hash, Size: len(u.Text), MTime: ms(u.MTime), CTime: ms(u.CTime), Title: short(u.Title, 200), Seen: t.UTC()}
		if n.MTime.IsZero() {
			n.MTime = t.UTC()
		}
		n.Tags, n.Links = tags, links
		kind := "created"
		if old != nil {
			kind, n.Seen, n.EvAt = "modified", old.Seen, old.EvAt
		}
		ix.Notes[p] = n
		journal = append(journal, change{At: t.UTC(), Path: p, Change: kind, Size: n.Size})
		if !learned {
			continue
		}
		facts := map[string]string{"path": p, "folder": folderOf(p), "vault": ix.Vault}
		if len(n.Tags) > 0 {
			facts["tags"] = strings.Join(n.Tags, ",")
		}
		title := n.Title
		if title == "" {
			title = strings.TrimSuffix(p[strings.LastIndex(p, "/")+1:], ".md")
		}
		ev := connectors.Event{ID: p + "@" + hash[:16], At: n.MTime, Title: title, Thread: "obsidian:" + p,
			Ref: "note path=" + p, Mine: true, Facts: facts}
		switch {
		case old == nil:
			ev.Type = EventCreated
			events = append(events, ev)
		case t.Sub(old.EvAt) >= changeQuiet:
			ev.Type = EventChanged
			facts["change"] = "modified"
			n.EvAt = t.UTC()
			events = append(events, ev)
		}
		known := map[string]bool{}
		for _, k := range tasks(oldText) {
			known[k.Key] = true
		}
		for _, k := range tasks(u.Text) {
			if known[k.Key] {
				continue
			}
			events = append(events, connectors.Event{Type: EventTask, ID: p + "#" + k.Key, At: n.MTime, Title: title,
				Thread: "obsidian:" + p, Ref: "note path=" + p, Mine: true,
				Facts: map[string]string{"path": p, "due": k.Due, "line": strconv.Itoa(k.Line), "vault": ix.Vault}})
		}
	}
	gone := b.Delete
	if m := b.Manifest; m != nil && m.Sync != "" && m.Parts > 0 && m.Parts <= 1000 && m.Part >= 0 && m.Part < m.Parts {
		if ix.Pending == nil || ix.Pending.Sync != m.Sync {
			ix.Pending = &manifest{Sync: m.Sync, Parts: m.Parts}
		}
		if !slicesContainsInt(ix.Pending.Got, m.Part) {
			ix.Pending.Got = append(ix.Pending.Got, m.Part)
			ix.Pending.Paths = append(ix.Pending.Paths, m.Paths...)
		}
		if len(ix.Pending.Got) == ix.Pending.Parts {
			listed := map[string]bool{}
			for _, p := range ix.Pending.Paths {
				listed[p] = true
			}
			for p := range ix.Notes {
				if !listed[p] {
					gone = append(gone, p)
				}
			}
			ix.Pending = nil
			ix.Learned = true
		}
	}
	for _, raw := range gone {
		p, ok := cleanPath(raw)
		if !ok || ix.Notes[p] == nil {
			continue
		}
		delete(ix.Notes, p)
		st.removeNote(p)
		journal = append(journal, change{At: t.UTC(), Path: p, Change: "removed"})
		if learned {
			events = append(events, connectors.Event{Type: EventChanged, ID: p + "@removed@" + strconv.FormatInt(t.UnixMilli(), 10),
				At: t.UTC(), Title: strings.TrimSuffix(p[strings.LastIndex(p, "/")+1:], ".md"), Thread: "obsidian:" + p, Mine: true,
				Facts: map[string]string{"path": p, "folder": folderOf(p), "change": "removed", "vault": ix.Vault}})
		}
	}
	if len(events) > bulk {
		events = []connectors.Event{{Type: EventChanged, ID: "bulk@" + b.ID, At: t.UTC(), Title: ix.Vault, Mine: true,
			Ref: "changes since=1h", Facts: map[string]string{"change": "bulk", "count": strconv.Itoa(len(events)), "vault": ix.Vault}}}
	}
	if err := st.save(ix); err != nil {
		return nil, err
	}
	if err := st.journal(journal); err != nil {
		return nil, err
	}
	return events, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func slicesContainsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
