package convo

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// The view of a chat's window (conversation.view) rides the record, is replaced whole, is cleared by an empty object,
// survives a restart, and a chat that is not there is refused.
func viewOf(t *testing.T, r *rig, id string) string {
	t.Helper()
	snap, err := r.rt.Conversations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range snap.Conversations {
		if c.ID == id {
			return string(c.View)
		}
	}
	t.Fatalf("no %s", id)
	return ""
}

func TestAChatKeepsItsView(t *testing.T) {
	r := newRig(t)
	made := r.do(wire.ConversationCreateCmd{OpID: "c", Cwd: "/w", Provider: wire.ProviderClaude})
	id := *made.ID
	if got := viewOf(t, r, id); got != "" {
		t.Fatalf("a chat has no view at first: %q", got)
	}
	set := func(op, view string) wire.CmdResult {
		return r.do(wire.ConversationViewCmd{OpID: op, ID: id, View: json.RawMessage(view)})
	}
	if res := set("v1", `{"screen":"/designly/d1?chat=`+id+`","at":1}`); res.R != wire.ResultAccepted {
		t.Fatalf("keep: %+v", res)
	}
	if got := viewOf(t, r, id); !strings.Contains(got, `"screen"`) {
		t.Fatalf("the view is not on the record: %q", got)
	}
	rev := func() int64 {
		snap, _ := r.rt.Conversations(context.Background())
		for _, c := range snap.Conversations {
			if c.ID == id {
				return c.Rev
			}
		}
		return -1
	}
	was := rev()
	if res := set("v2", `{"screen":"/designly/d1?chat=`+id+`","at":1}`); res.R != wire.ResultAccepted {
		t.Fatalf("the same view: %+v", res)
	}
	if rev() != was {
		t.Fatal("the same view is no change: the record's rev stays")
	}
	r.rt.Stop()
	r.restart()
	if got := viewOf(t, r, id); !strings.Contains(got, `"at":1`) {
		t.Fatalf("after a restart: %q", got)
	}
	if res := set("v3", `{}`); res.R != wire.ResultAccepted {
		t.Fatalf("clear: %+v", res)
	}
	if got := viewOf(t, r, id); got != "" {
		t.Fatalf("an empty object clears the view: %q", got)
	}
	if res := r.do(wire.ConversationViewCmd{OpID: "v4", ID: "tab-nobody", View: json.RawMessage(`{"a":1}`)}); res.Code != wire.RefuseNotFound {
		t.Fatalf("a chat that is not there: %+v", res)
	}
}

func TestTheViewValidates(t *testing.T) {
	ok := wire.ConversationViewCmd{OpID: "x", ID: "tab-a", View: json.RawMessage(`{"a":1}`)}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]wire.ConversationViewCmd{
		"no opId":    {ID: "tab-a", View: json.RawMessage(`{}`)},
		"not object": {OpID: "x", ID: "tab-a", View: json.RawMessage(`[1]`)},
		"null":       {OpID: "x", ID: "tab-a", View: json.RawMessage(`null`)},
		"too large":  {OpID: "x", ID: "tab-a", View: json.RawMessage(`{"a":"` + strings.Repeat("x", wire.MaxConversationView) + `"}`)},
	} {
		if bad.Validate() == nil {
			t.Fatalf("%s must not validate", name)
		}
	}
}
