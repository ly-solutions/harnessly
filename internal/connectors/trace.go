package connectors

// The developer mode's trace: what each connection did, kept in a ring on the machine while the person has
// the mode on. A poll's arguments, its time, how many things came, the new ids, the cursor before and after, the
// refusals and errors, the raw envelope of each event and where it went (the hub's journal, the inbox, a rule). The
// trace never leaves the machine: no destination reads it, the runtime serves it to the owner's page alone, and
// nothing is written while the mode is off.

import (
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

// Trace levels.
const (
	TraceInfo  = "info"
	TraceWarn  = "warn"
	TraceError = "error"
)

// TraceSync is the trace's key of the synchronization of the machine's MCP connections (not a connection of its own).
const TraceSync = "mcp-sync"

// TraceEntry is one line of the trace.
type TraceEntry struct {
	Seq  uint64    `json:"seq"`
	At   time.Time `json:"at"`
	Conn string    `json:"conn"`
	// Part is what wrote it: "check", "poll", "source", "commit", "deliver", "recipe", "sync".
	Part    string `json:"part"`
	Level   string `json:"level"`
	Message string `json:"message"`
	// Data is the raw detail, shown folded: JSON of the arguments, the answer (clipped), the envelope.
	Data json.RawMessage `json:"data,omitempty"`
}

// Tracer keeps the last entries of each connection and hands new ones to the streams that follow them.
type Tracer struct {
	mu      sync.Mutex
	size    int
	seq     uint64
	rings   map[string][]TraceEntry
	subs    map[chan TraceEntry]string // stream → the connection it follows ("" — all)
	enabled func() bool
	now     func() time.Time
}

// traceRing entries are kept per connection; traceDataMax bytes of a detail.
const (
	traceRing    = 300
	traceDataMax = 16 << 10
)

// NewTracer keeps size entries per connection; it writes nothing until SetEnabled says the mode is on.
func NewTracer(size int) *Tracer {
	return &Tracer{size: size, rings: map[string][]TraceEntry{}, subs: map[chan TraceEntry]string{}, now: time.Now}
}

// Trace is the runtime's one trace.
var Trace = NewTracer(traceRing)

// SetEnabled gives the question «is the developer mode on»; nil turns the trace off. Asked on every write, so it must
// be cheap (the runtime caches the settings file).
func (t *Tracer) SetEnabled(on func() bool) {
	t.mu.Lock()
	t.enabled = on
	t.mu.Unlock()
}

// On says whether the trace writes now. Callers check it before building an expensive detail.
func (t *Tracer) On() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	on := t.enabled
	t.mu.Unlock()
	return on != nil && on()
}

// Add writes an entry when the mode is on. data is marshalled to JSON and clipped; a []byte or json.RawMessage that is
// not JSON is kept as a string.
func (t *Tracer) Add(conn, part, level, message string, data any) {
	if !t.On() {
		return
	}
	e := TraceEntry{Conn: conn, Part: part, Level: level, Message: message, Data: traceData(data)}
	t.mu.Lock()
	t.seq++
	e.Seq, e.At = t.seq, t.now().UTC()
	ring := append(t.rings[conn], e)
	if len(ring) > t.size {
		ring = append([]TraceEntry(nil), ring[len(ring)-t.size:]...)
	}
	t.rings[conn] = ring
	for ch, follow := range t.subs {
		if follow != "" && follow != conn {
			continue
		}
		select {
		case ch <- e:
		default: // a slow stream misses lines rather than holding a source
		}
	}
	t.mu.Unlock()
}

// Recent is the last n entries of a connection ("" — of all of them), oldest first.
func (t *Tracer) Recent(conn string, n int) []TraceEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []TraceEntry
	if conn != "" {
		out = append(out, t.rings[conn]...)
	} else {
		for _, ring := range t.rings {
			out = append(out, ring...)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	}
	if n > 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// Conns are the keys that have entries, sorted.
func (t *Tracer) Conns() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.rings))
	for k := range t.rings {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Forget drops a removed connection's entries.
func (t *Tracer) Forget(conn string) {
	t.mu.Lock()
	delete(t.rings, conn)
	t.mu.Unlock()
}

// Subscribe follows new entries of a connection ("" — all); stop ends it.
func (t *Tracer) Subscribe(conn string) (<-chan TraceEntry, func()) {
	ch := make(chan TraceEntry, 64)
	t.mu.Lock()
	t.subs[ch] = conn
	t.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			t.mu.Lock()
			delete(t.subs, ch)
			t.mu.Unlock()
		})
	}
}

func traceData(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	var raw []byte
	switch x := v.(type) {
	case json.RawMessage:
		raw = x
	case []byte:
		raw = x
	case string:
		raw, _ = json.Marshal(x)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			raw, _ = json.Marshal(err.Error())
		} else {
			raw = b
		}
	}
	if !json.Valid(raw) {
		raw, _ = json.Marshal(string(raw))
	}
	if len(raw) > traceDataMax {
		cut := string(raw[:traceDataMax])
		for !utf8.ValidString(cut) {
			cut = cut[:len(cut)-1]
		}
		raw, _ = json.Marshal(map[string]any{"clipped": true, "bytes": len(raw), "head": cut})
	}
	return raw
}

// SyncItem is one MCP server the synchronization saw (HRN-1815 fills Taken and Why; today it only lists what is in
// the machine's mcp-connections.json).
type SyncItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind,omitempty"`
	From  string `json:"from"`
	Taken bool   `json:"taken"`
	Why   string `json:"why,omitempty"`
}

// TraceSyncSeen writes what the synchronization found, from where, what it took and why it refused the rest.
func TraceSyncSeen(from string, items []SyncItem) {
	if !Trace.On() {
		return
	}
	taken := 0
	for _, it := range items {
		if it.Taken {
			taken++
		}
	}
	Trace.Add(TraceSync, "sync", TraceInfo, syncLine(from, len(items), taken), items)
}

func syncLine(from string, found, taken int) string {
	return from + ": " + strconv.Itoa(found) + " found, " + strconv.Itoa(taken) + " taken"
}
