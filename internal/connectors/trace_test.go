package connectors

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTraceWritesOnlyWhenOn(t *testing.T) {
	tr := NewTracer(3)
	tr.Add("c1", "poll", TraceInfo, "off", nil)
	if got := tr.Recent("", 0); len(got) != 0 {
		t.Fatalf("written while off: %v", got)
	}
	on := true
	tr.SetEnabled(func() bool { return on })
	for i := range 5 {
		tr.Add("c1", "poll", TraceInfo, "line", map[string]int{"n": i})
	}
	tr.Add("c2", "check", TraceError, "other", nil)
	got := tr.Recent("c1", 0)
	if len(got) != 3 || string(got[0].Data) != `{"n":2}` || string(got[2].Data) != `{"n":4}` {
		t.Fatalf("ring: %+v", got)
	}
	if all := tr.Recent("", 2); len(all) != 2 || all[1].Conn != "c2" || all[0].Seq >= all[1].Seq {
		t.Fatalf("all: %+v", all)
	}
	if c := tr.Conns(); len(c) != 2 {
		t.Fatalf("conns: %v", c)
	}
	on = false
	tr.Add("c1", "poll", TraceInfo, "off again", nil)
	if got := tr.Recent("c1", 0); got[len(got)-1].Message == "off again" {
		t.Fatal("written after the mode went off")
	}
	tr.Forget("c1")
	if got := tr.Recent("c1", 0); len(got) != 0 {
		t.Fatalf("forgotten: %v", got)
	}
}

func TestTraceStreamFollowsItsConnection(t *testing.T) {
	tr := NewTracer(10)
	tr.SetEnabled(func() bool { return true })
	ch, stop := tr.Subscribe("c1")
	defer stop()
	tr.Add("c2", "poll", TraceInfo, "not mine", nil)
	tr.Add("c1", "poll", TraceInfo, "mine", nil)
	select {
	case e := <-ch:
		if e.Message != "mine" {
			t.Fatalf("got %q", e.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("no entry")
	}
	stop()
	tr.Add("c1", "poll", TraceInfo, "after stop", nil) // must not block or panic
}

func TestTraceDataClippedAndKeptJSON(t *testing.T) {
	if string(traceData([]byte("not json"))) != `"not json"` {
		t.Fatal("non-JSON bytes")
	}
	big := traceData(json.RawMessage(`"` + strings.Repeat("я", traceDataMax) + `"`))
	var v struct {
		Clipped bool `json:"clipped"`
		Head    string
	}
	if err := json.Unmarshal(big, &v); err != nil || !v.Clipped || len(big) > 3*traceDataMax {
		t.Fatalf("clip: %v %d", err, len(big))
	}
}

func TestTraceSyncSeen(t *testing.T) {
	old := Trace
	Trace = NewTracer(10)
	defer func() { Trace = old }()
	TraceSyncSeen("mcp-connections.json", []SyncItem{{ID: "s1", Name: "A", Taken: true}, {ID: "s2", Name: "B", Why: "no tools"}})
	if len(Trace.Recent(TraceSync, 0)) != 0 {
		t.Fatal("written while off")
	}
	Trace.SetEnabled(func() bool { return true })
	TraceSyncSeen("mcp-connections.json", []SyncItem{{ID: "s1", Name: "A", Taken: true}, {ID: "s2", Name: "B", Why: "no tools"}})
	got := Trace.Recent(TraceSync, 0)
	if len(got) != 1 || got[0].Message != "mcp-connections.json: 2 found, 1 taken" || !strings.Contains(string(got[0].Data), "no tools") {
		t.Fatalf("sync: %+v", got)
	}
}
