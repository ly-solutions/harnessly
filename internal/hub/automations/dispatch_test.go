package automations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/config"
	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/link"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hub/store/storetest"
)

// fakeLinks is the machines of a test: which are connected and what a start answers.
type fakeLinks struct {
	online  map[string]bool
	started []string
	params  []any
	fail    error
}

func (f *fakeLinks) Get(id string) *link.Link {
	if f.online[id] {
		return &link.Link{MachineID: id}
	}
	return nil
}

func (f *fakeLinks) Do(_ context.Context, machineID, _ string, params any, _ time.Duration) (json.RawMessage, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	f.started = append(f.started, machineID)
	f.params = append(f.params, params)
	return json.RawMessage(`{"conversationId":"c-1"}`), nil
}

type movingClock struct{ at time.Time }

func dispatcher(t *testing.T) (*Service, *fakeLinks, *movingClock) {
	t.Helper()
	db := storetest.Open(t)
	u := &store.User{ID: "u1", Email: "u1@example.com"}
	if err := db.S().CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"m1", "m-on", "m-off", "m-gone"} {
		if err := db.S().CreateMachine(context.Background(), &store.Machine{ID: id, Name: id, KeyPub: "k-" + id, CreatedBy: "u1"}); err != nil {
			t.Fatal(err)
		}
	}
	links := &fakeLinks{online: map[string]bool{}}
	clock := &movingClock{at: noon()}
	s := New(Deps{Config: &config.Config{Automations: true, AutomationsSecret: "s3cret", AutomationsTokenEnv: "HARNESS_AUTOMATION_RESULT_TOKEN"}, DB: db, Links: links, Now: func() time.Time { return clock.at }})
	return s, links, clock
}

func queued(t *testing.T, s *Service, name, machine, fingerprint string) string {
	t.Helper()
	r, ok, err := s.addRule(context.Background(), Rule{OwnerID: "u1", Name: name, MachineID: machine, Trigger: Trigger{Source: "alert"}, PromptTemplate: "fix it", Model: "sonnet", Enabled: true})
	if err != nil || !ok {
		t.Fatalf("addRule: %v %v", ok, err)
	}
	out, err := s.queueRun(context.Background(), r, fingerprint, json.RawMessage(`{}`))
	if err != nil || out.RunID == "" {
		t.Fatalf("queueRun: %+v %v", out, err)
	}
	return out.RunID
}

func statusOf(t *testing.T, s *Service, id string) Run {
	t.Helper()
	r, err := s.runByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A run whose machine is not connected stays in the queue, is not failed, and starts once the machine is back.
func TestRunWaitsInTheQueueWhileItsMachineIsOff(t *testing.T) {
	s, links, _ := dispatcher(t)
	ctx := context.Background()
	id := queued(t, s, "a", "m-off", "f1")
	for range 3 {
		if s.Dispatch(ctx) {
			t.Fatal("a run was started on a machine that is not connected")
		}
	}
	if r := statusOf(t, s, id); r.Status != StatusQueued || r.Error != "" {
		t.Fatalf("the run of a machine that is off: %+v", r)
	}
	links.online["m-off"] = true
	if !s.Dispatch(ctx) {
		t.Fatal("the run did not start once its machine was back")
	}
	if r := statusOf(t, s, id); r.Status != StatusRunning || r.ConversationID != "c-1" || r.MachineID != "m-off" {
		t.Fatalf("the started run: %+v", r)
	}
}

// A run's chat is marked for the routines' folder of the sidebar and named after its rule: a trigger's
// run as a routine's.
func TestRunChatStandsInTheRoutinesFolder(t *testing.T) {
	s, links, _ := dispatcher(t)
	queued(t, s, "Разбор алертов", "m1", "f1")
	links.online["m1"] = true
	if !s.Dispatch(context.Background()) {
		t.Fatal("nothing was started")
	}
	params, _ := links.params[0].(map[string]any)
	from, _ := params["from"].(map[string]string)
	if from["app"] != "routines" || from["label"] != "Регулярные" || from["title"] != "Разбор алертов" {
		t.Fatalf("the run's mark: %#v", params["from"])
	}
}

// A run that waits for its machine does not hold up the run of a machine that is connected.
func TestWaitingRunDoesNotHoldUpAnotherMachines(t *testing.T) {
	s, links, clock := dispatcher(t)
	ctx := context.Background()
	waiting := queued(t, s, "off", "m-off", "f1")
	clock.at = clock.at.Add(time.Second)
	ready := queued(t, s, "on", "m-on", "f2")
	links.online["m-on"] = true
	if !s.Dispatch(ctx) {
		t.Fatal("nothing was started")
	}
	if statusOf(t, s, ready).Status != StatusRunning || statusOf(t, s, waiting).Status != StatusQueued {
		t.Errorf("ready %s, waiting %s", statusOf(t, s, ready).Status, statusOf(t, s, waiting).Status)
	}
}

// A run taken by a hub that stopped before the conversation was started goes back to the queue when its lease runs
// out, and is given up after maxAttempts takes. A run whose conversation was started is never taken back.
func TestAbandonedStartReturnsToTheQueue(t *testing.T) {
	s, links, clock := dispatcher(t)
	ctx := context.Background()
	id := queued(t, s, "a", "m1", "f1")
	links.online["m1"] = true
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// taken and never finished: the hub stopped between the take and the start
		if run, _, _, _, err := s.claim(ctx, s.machineFor); err != nil || run.ID != id {
			t.Fatalf("take %d: %v", attempt, err)
		}
		s.reap(ctx)
		if st := statusOf(t, s, id).Status; st != StatusRunning {
			t.Fatalf("take %d: reaped before the lease ran out: %s", attempt, st)
		}
		clock.at = clock.at.Add(startLease + time.Second)
		s.reap(ctx)
		want := StatusQueued
		if attempt == maxAttempts {
			want = StatusFailed
		}
		if r := statusOf(t, s, id); r.Status != want {
			t.Fatalf("after take %d: %s, want %s", attempt, r.Status, want)
		}
	}

	started := queued(t, s, "b", "m1", "f2")
	if !s.Dispatch(ctx) {
		t.Fatal("the second run did not start")
	}
	clock.at = clock.at.Add(10 * startLease)
	s.reap(ctx)
	if r := statusOf(t, s, started); r.Status != StatusRunning {
		t.Errorf("a run whose conversation runs was taken back: %s", r.Status)
	}
}

// A start that may have reached the machine (it did not answer in time) is not tried again: a second start would do
// the work twice.
func TestStartWithoutAnAnswerIsNotTriedAgain(t *testing.T) {
	s, links, _ := dispatcher(t)
	ctx := context.Background()
	id := queued(t, s, "a", "m1", "f1")
	links.online["m1"] = true
	links.fail = errors.New("no answer in time")
	s.Dispatch(ctx)
	if r := statusOf(t, s, id); r.Status != StatusFailed || r.Error != "no answer in time" {
		t.Fatalf("after a start without an answer: %+v", r)
	}
}

// A start the machine was never asked for (it went away) puts the run back for another try, with the reason kept.
func TestFailedStartIsTriedAgain(t *testing.T) {
	s, links, _ := dispatcher(t)
	ctx := context.Background()
	id := queued(t, s, "a", "m1", "f1")
	links.online["m1"] = true
	links.fail = httpx.ErrMachineOffline
	for attempt := 1; attempt < maxAttempts; attempt++ {
		if s.Dispatch(ctx) {
			t.Fatal("a failed start was reported as started")
		}
		if r := statusOf(t, s, id); r.Status != StatusQueued || r.Error != httpx.ErrMachineOffline.Error() {
			t.Fatalf("after a failed start %d: %+v", attempt, r)
		}
	}
	s.Dispatch(ctx)
	if r := statusOf(t, s, id); r.Status != StatusFailed {
		t.Fatalf("after %d failed starts: %s", maxAttempts, r.Status)
	}
}

// A run nobody could start for a day is given up, so the queue does not grow without end.
func TestRunWithoutAMachineForADayIsGivenUp(t *testing.T) {
	s, _, clock := dispatcher(t)
	ctx := context.Background()
	id := queued(t, s, "a", "m-gone", "f1")
	clock.at = clock.at.Add(queuedFor + time.Minute)
	s.Dispatch(ctx)
	if r := statusOf(t, s, id); r.Status != StatusFailed || r.Error == "" {
		t.Fatalf("a run that waited a day: %+v", r)
	}
}

// The report token reaches the conversation under the name the runtime lets a started conversation set (its prefix), and
// the prompt names the same variable.
func TestStartNamesTheTokenVariableFromTheConfig(t *testing.T) {
	s, links, _ := dispatcher(t)
	s.cfg.AutomationsTokenEnv = "TEST_AUTOMATION_RESULT_TOKEN"
	links.online["m1"] = true
	queued(t, s, "rule", "m1", "fp-env")
	if !s.Dispatch(context.Background()) {
		t.Fatal("run not dispatched")
	}
	p, ok := links.params[0].(map[string]any)
	if !ok {
		t.Fatalf("params %T", links.params[0])
	}
	env, _ := p["env"].(map[string]string)
	if env["TEST_AUTOMATION_RESULT_TOKEN"] == "" || len(env) != 1 {
		t.Fatalf("env %v", env)
	}
	if prompt, _ := p["prompt"].(string); !strings.Contains(prompt, "$TEST_AUTOMATION_RESULT_TOKEN") {
		t.Fatalf("prompt does not name the variable: %s", prompt)
	}
}

// A routine starts on the machine the person works on while it is connected, and on the oldest connected machine of the
// owner only when that one is off: a run elsewhere makes a chat the person does not see.
func TestRoutineRunPrefersItsOwnMachine(t *testing.T) {
	s, links, _ := dispatcher(t)
	ctx := context.Background()
	for i, id := range []string{"m-old", "m-mine"} {
		if _, err := s.db.Exec(ctx, `INSERT INTO machines (id, name, hostname, os, arch, unix_user, profile, via, key_pub, enc_pub, agent_version, created_at, info, created_by)
			VALUES (?, ?, 'h', '', '', '', '', '', ?, ?, '', ?, '', 'u1')`, id, id, "k"+id, "e"+id, i+1); err != nil {
			t.Fatal(err)
		}
	}
	rule := Rule{OwnerID: "u1", Routine: &Routine{Machine: "m-mine"}}
	links.online["m-old"], links.online["m-mine"] = true, true
	if got, err := s.machineFor(ctx, rule); err != nil || got != "m-mine" {
		t.Fatalf("both on: %q %v", got, err)
	}
	links.online["m-mine"] = false
	if got, err := s.machineFor(ctx, rule); err != nil || got != "m-old" {
		t.Fatalf("own machine off: %q %v", got, err)
	}
	if got, err := s.machineFor(ctx, Rule{OwnerID: "u1"}); err != nil || got != "m-old" {
		t.Fatalf("no preference: %q %v", got, err)
	}
}

// A rule never starts a chat on a machine that is not its owner's any more, was handed to them and not registered
// again, or while the owner's account is off.
func TestRunStaysOffAMachineThatIsNotTheOwnersToUse(t *testing.T) {
	ctx := context.Background()
	for name, change := range map[string]func(s *Service) error{
		"handed to somebody else": func(s *Service) error { return s.db.S().SetMachineOwner(ctx, "m1", "u2") },
		"handed away and back by an administrator's keys": func(s *Service) error {
			if _, err := s.db.Exec(ctx, `UPDATE machines SET created_by = 'u2' WHERE id = 'm1'`); err != nil {
				return err
			}
			return s.db.S().SetMachineOwner(ctx, "m1", "u1")
		},
		"the owner switched off": func(s *Service) error { _, err := s.db.S().DisableUser(ctx, "u1", noon()); return err },
	} {
		for _, named := range []string{"m1", ""} {
			s, links, _ := dispatcher(t)
			if err := s.db.S().CreateUser(ctx, &store.User{ID: "u2", Email: "u2@example.com"}); err != nil {
				t.Fatal(err)
			}
			id := queued(t, s, "a", named, "f1")
			if err := change(s); err != nil {
				t.Fatal(err)
			}
			links.online["m1"] = true
			if s.Dispatch(ctx) {
				t.Errorf("%s (rule machine %q): a chat was started", name, named)
			}
			if r := statusOf(t, s, id); r.Status != StatusQueued {
				t.Errorf("%s (rule machine %q): %s", name, named, r.Status)
			}
		}
	}
}

// A rule that names a machine handed to somebody else goes on running there while its owner may still give that machine
// tasks, and stops when they may not. Their own machine that waits for them is not run this way, nor is any machine for
// an account that is off.
func TestNamedMachineOfAnotherPersonRunsOnlyWithAccess(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string]struct {
		owner       string
		access, off bool
		starts      bool
	}{
		"access kept":                {owner: "u2", access: true, starts: true},
		"no access":                  {owner: "u2"},
		"their own, not accepted":    {owner: "u1", access: true},
		"access, account turned off": {owner: "u2", access: true, off: true},
	} {
		s, links, _ := dispatcher(t)
		s.operates = func(_ context.Context, userID string, m store.Machine) bool { return c.access && userID == "u1" }
		if err := s.db.S().CreateUser(ctx, &store.User{ID: "u2", Email: "u2@example.com"}); err != nil {
			t.Fatal(err)
		}
		id := queued(t, s, "a", "m1", "f1")
		if _, err := s.db.Exec(ctx, `UPDATE machines SET created_by = ?, keys_by = 'u3' WHERE id = 'm1'`, c.owner); err != nil {
			t.Fatal(err)
		}
		if c.off {
			if _, err := s.db.S().DisableUser(ctx, "u1", noon()); err != nil {
				t.Fatal(err)
			}
		}
		links.online["m1"] = true
		if got := s.Dispatch(ctx); got != c.starts {
			t.Errorf("%s: started %v, want %v (%s)", name, got, c.starts, statusOf(t, s, id).Status)
		}
	}
}
