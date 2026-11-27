package machines

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/journal"
	"github.com/ly-solutions/harnessly/internal/jsvalue"
)

// A browser's failed subscription reaches the machine's journal, where an agent reads it.
func TestPushDiagnosticGoesToTheJournal(t *testing.T) {
	j := journal.Open(t.TempDir(), journal.Options{})
	journal.SetDefault(j)
	defer func() { journal.SetDefault(nil); j.Close() }()
	var logged string
	s := &service{cfg: Config{Log: func(level, msg string, kv ...any) { logged = level + " " + msg }}}
	body := jsvalue.NewObject("step", "subscribe", "name", "Error", "message", "the device did not answer",
		"permission", "granted", "standalone", true, "userAgent", "Mozilla/5.0 (Linux; Android 15)")
	r := httptest.NewRequest(http.MethodPost, "/api/push/diagnostic", nil)
	if status, _, _ := s.diagnostic(r, body); status != http.StatusNoContent {
		t.Fatalf("status %d", status)
	}
	j.Flush()
	got := j.Query(journal.Filter{Since: time.Now().Add(-time.Minute), Level: journal.Warn, Source: journal.SourceWeb})
	if len(got) != 1 || got[0].Event != "push.subscribe" || !strings.Contains(got[0].Text, "step=subscribe") ||
		!strings.Contains(got[0].Text, "standalone=true") || !strings.Contains(got[0].Text, "Android 15") {
		t.Fatalf("journal: %+v", got)
	}
	if !strings.HasPrefix(logged, "warn push subscription failed") {
		t.Fatalf("service log: %q", logged)
	}
}
