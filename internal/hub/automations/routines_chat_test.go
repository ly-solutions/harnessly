package automations

import (
	"context"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// A run's chat finds its routine, the run and the routine's runs; another chat, another owner or a trigger's run find
// nothing.
func TestRoutineOfChat(t *testing.T) {
	s, _, _ := routineSvc(t)
	ctx := context.Background()
	r := addRoutine(t, s, "digest", Routine{Words: "по будням в 9:00"})
	run, token := started(t, s, r, "schedule:1")
	report(t, s, run, token, Report{Outcome: "ok", Summary: "Готово"})

	got, err := s.MachineCall(ctx, "u1", "me@example.com", "m1", hubproto.RoutinesCall{Op: "chat", ID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	v, ok := m["routine"].(RoutineView)
	if !ok || v.ID != r.ID || v.Routine.Words != "по будням в 9:00" {
		t.Fatalf("routine: %+v", m["routine"])
	}
	if m["run"].(Run).ID != run.ID || len(m["runs"].([]Run)) != 1 {
		t.Fatalf("run and runs: %+v %+v", m["run"], m["runs"])
	}
	for _, c := range []struct{ owner, chat string }{{"u1", "c2"}, {"u2", "c1"}, {"u1", ""}} {
		got, err := s.MachineCall(ctx, c.owner, "", "m1", hubproto.RoutinesCall{Op: "chat", ID: c.chat})
		if err != nil {
			t.Fatal(err)
		}
		if got.(map[string]any)["routine"] != nil {
			t.Fatalf("%+v found %+v", c, got)
		}
	}
}
