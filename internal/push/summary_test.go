package push

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeSummary struct {
	picked bool
	text   string
	err    error
	asked  []string
}

func (f *fakeSummary) Picked() bool { return f.picked }
func (f *fakeSummary) Summarize(_ context.Context, answer string) (string, error) {
	f.asked = append(f.asked, answer)
	return f.text, f.err
}

func TestPushWithASummary(t *testing.T) {
	svc := newFakeService(t)
	dir := t.TempDir()
	laptop := newDevice(t, svc.srv.URL+"/laptop", "a")
	raw, _ := json.MarshalIndent([]any{laptop.sub}, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, SubscriptionsFile), raw, 0o600)
	long := strings.Repeat("Поправил вход и добавил тест. ", 20)
	transcript := writeJSONL(t, user("go", "2026-09-25T10:00:00Z"), assistant([]any{m{"type": "text", "text": long}}, "2026-09-25T10:01:00Z"))
	short := writeJSONL(t, user("go", "2026-09-25T10:00:00Z"), assistant([]any{m{"type": "text", "text": "Готово."}}, "2026-09-25T10:01:00Z"))

	sum := &fakeSummary{}
	s := New(Config{Dir: dir, Subject: "mailto:ops@example.org", TagPrefix: tagPrefix, Now: func() time.Time { return now }, Summary: sum})
	body := func() string {
		t.Helper()
		s.Wait()
		if len(svc.got) != 1 {
			t.Fatalf("%d pushes", len(svc.got))
		}
		plain, err := Decrypt(svc.got[0].body, laptop.key, laptop.auth)
		if err != nil {
			t.Fatal(err)
		}
		var msg Message
		if err := json.Unmarshal(plain, &msg); err != nil {
			t.Fatal(err)
		}
		svc.got = nil
		return msg.Body
	}
	asNow := BuildMessage("tab-9", tabEntry(transcript, nil), "done", tagPrefix, now).Body

	// «As now»: the model is not asked, the words are the answer's.
	s.Push("tab-9", tabEntry(transcript, nil), "done")
	if got := body(); got != asNow || len(sum.asked) != 0 {
		t.Fatalf("as now: %q, asked %d", got, len(sum.asked))
	}

	// Picked: the words are the model's summary of the whole answer.
	sum.picked, sum.text = true, "Поправил вход, тесты зелёные"
	s.Push("tab-9", tabEntry(transcript, nil), "done")
	if got := body(); got != "Готово — Поправил вход, тесты зелёные" || len(sum.asked) != 1 || !strings.HasPrefix(sum.asked[0], "Поправил вход и добавил тест.") || len(sum.asked[0]) < len(long)/2 {
		t.Fatalf("picked: %q, asked %v", got, sum.asked)
	}

	// An answer that fits the notification needs no summary, and neither does a question.
	sum.asked = nil
	s.Push("tab-9", tabEntry(short, nil), "done")
	if got := body(); got != "Готово — Готово." || len(sum.asked) != 0 {
		t.Fatalf("a short answer: %q, asked %d", got, len(sum.asked))
	}
	s.Push("tab-9", tabEntry(transcript, nil), "question")
	if _ = body(); len(sum.asked) != 0 {
		t.Fatal("a question was summarized")
	}

	// The provider fails: the notification goes as it did before.
	sum.err = errors.New("unreachable")
	s.Push("tab-9", tabEntry(transcript, nil), "done")
	if got := body(); got != asNow {
		t.Fatalf("after a failure: %q", got)
	}
}
