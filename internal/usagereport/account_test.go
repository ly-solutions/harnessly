package usagereport

import (
	"strings"
	"testing"
)

// Two accounts of one provider on the same day, model and project are two rows of spend, each under the account's
// name; a session nobody noted an account of keeps a row as rows were before, without a name at its end.
func TestSpendIsCountedByTheAccountOfTheSession(t *testing.T) {
	const now = 1_800_000_000_000.0
	ts := now/1000 - 3600
	call := func(session, key string) *Facts {
		f := newFacts("claude", session, false)
		f.Models, f.Projects = []any{"kimi-k3.1-pro"}, []string{"shop"}
		f.FirstTs, f.LastTs = ts, ts
		f.Messages = []MsgRow{{Key: key, Ts: ts, Model: 0, Project: 0, Input: 100, Output: 10}}
		return f
	}
	accounts := map[string]string{"claude:s-work": "Kimi рабочий", "claude:s-home": "Kimi личный"}
	accountOf := func(provider, session string) string { return accounts[provider+":"+session] }
	files := []*Facts{call("s-work", "a"), call("s-work", "b"), call("s-home", "c"), call("s-old", "d")}

	r, err := BuildWith(files, "p@example.com", "box", "UTC", Clock{}, now, accountOf)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, u := range r.Usage {
		got[u.Account] += u.APICalls
	}
	if len(r.Usage) != 3 || got["Kimi рабочий"] != 2 || got["Kimi личный"] != 1 || got[""] != 1 {
		t.Fatalf("rows %d, calls by account %v", len(r.Usage), got)
	}
	out := string(r.JSON())
	if !strings.Contains(out, `,2,"Kimi рабочий"]`) || !strings.Contains(out, `,1,"Kimi личный"]`) {
		t.Fatalf("the account is not the last of its row: %s", out)
	}

	// Without accounts the report is what it was: one row, nothing after the calls.
	plain, err := Build(files, "p@example.com", "box", "UTC", Clock{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Usage) != 1 || plain.Usage[0].APICalls != 4 || strings.Contains(string(plain.JSON()), "Kimi") {
		t.Fatalf("a report without accounts changed: %+v", plain.Usage)
	}
}
