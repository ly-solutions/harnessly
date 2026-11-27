package main

import (
	"context"
	"testing"

	"github.com/ly-solutions/harnessly/internal/filestore"
)

// fakeVerdictStore is the store as the verdicts see it: the local copy's and the store's newest revisions, and what a
// push does to them.
type fakeVerdictStore struct {
	local, remote int64
	onPush        func(s *fakeVerdictStore)
	pushes        int
}

func (s *fakeVerdictStore) PushProject(context.Context, string) error {
	s.pushes++
	if s.onPush != nil {
		s.onPush(s)
	}
	return nil
}
func (s *fakeVerdictStore) LocalHead(string) (int64, error)                  { return s.local, nil }
func (s *fakeVerdictStore) StoreHead(context.Context, string) (int64, error) { return s.remote, nil }

// the machine checks a project at the revision its copy and the store agree on, keeps the verdict, attaches
// it to the project's record, and does not check again until the project changes.
func TestShareVerdicts(t *testing.T) {
	ctx := context.Background()
	st := &fakeVerdictStore{local: 5, remote: 5}
	checks, blocked := 0, true
	v := newShareVerdicts(st, func(context.Context, string) bool { checks++; return blocked })

	recs := []filestore.Record{{ID: "site", At: 1}, {ID: "other", At: 1}}
	if !v.refresh(ctx, recs) || checks != 2 {
		t.Fatalf("first pass: %d checks", checks)
	}
	v.attach(recs)
	if recs[0].Share == nil || *recs[0].Share != (filestore.ShareVerdict{Rev: 5, Blocked: true}) {
		t.Fatalf("record carries %+v", recs[0].Share)
	}

	// Unchanged records are not checked again.
	if v.refresh(ctx, []filestore.Record{{ID: "site", At: 1}}) || checks != 2 {
		t.Fatalf("an unchanged project was checked again: %d", checks)
	}
	// A record that changed without a new revision of its files keeps its verdict without a check.
	if v.refresh(ctx, []filestore.Record{{ID: "site", At: 2}}) || checks != 2 {
		t.Fatalf("a project at the same revision was checked again: %d", checks)
	}

	// A new revision is checked, and the new verdict replaces the old one.
	st.local, st.remote, blocked = 6, 6, false
	if !v.refresh(ctx, []filestore.Record{{ID: "site", At: 3}}) || checks != 3 {
		t.Fatalf("a new revision: %d checks", checks)
	}
	recs = []filestore.Record{{ID: "site"}}
	v.attach(recs)
	if *recs[0].Share != (filestore.ShareVerdict{Rev: 6}) {
		t.Fatalf("record carries %+v", recs[0].Share)
	}

	// A copy behind the store (another machine's edit on its way) gets no verdict, and is asked again later.
	st.local, st.remote = 6, 7
	if v.refresh(ctx, []filestore.Record{{ID: "site", At: 4}}) || checks != 3 {
		t.Fatalf("a copy behind the store was checked: %d", checks)
	}
	st.local = 7
	if !v.refresh(ctx, []filestore.Record{{ID: "site", At: 4}}) || checks != 4 {
		t.Fatalf("the same record was not asked again once the copy caught up: %d", checks)
	}

	// The project changed while the checks ran: the verdict speaks for no revision and is dropped.
	st.onPush = func(s *fakeVerdictStore) {
		if s.pushes%2 == 0 {
			s.local, s.remote = 9, 9
		}
	}
	st.local, st.remote, st.pushes = 8, 8, 0
	if v.refresh(ctx, []filestore.Record{{ID: "site", At: 5}}) {
		t.Fatal("a verdict of a project that changed under the checks was kept")
	}
	recs = []filestore.Record{{ID: "site"}}
	v.attach(recs)
	if recs[0].Share.Rev != 7 {
		t.Fatalf("record carries %+v", recs[0].Share)
	}
}
