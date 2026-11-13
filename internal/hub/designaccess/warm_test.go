package designaccess

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type warmView struct {
	fakeLibrary
	warmed []string
}

func (w *warmView) Warm(_ context.Context, _, project string, at int64) error {
	w.warmed = append(w.warmed, project)
	return nil
}

// after a start the hub lays out the projects whose live guest links were opened lately, the most recent
// first; a revoked link, one that ran out, one nobody opened for weeks and a project the store lacks are left alone.
func TestWarmLaysOutTheLatelyOpenedProjects(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	now := r.svc.now()
	q := r.svc.db.S().Q()
	link := func(id, project string, opened time.Duration, revoked, expired bool) {
		t.Helper()
		var rev, exp any
		if revoked {
			rev = ms(now.Add(-time.Minute))
		}
		if expired {
			exp = ms(now.Add(-time.Minute))
		}
		if _, err := q.Exec(ctx, `INSERT INTO design_share_links (id, machine_id, project_id, token_hash, rights, created_by, created_at, last_opened_at, revoked_at, expires_at)
			VALUES (?, ?, ?, ?, ?, 'owner@example.com', ?, ?, ?, ?)`, id, r.owner.ID, project, "h-"+id, roleView, ms(now), ms(now.Add(-opened)), rev, exp); err != nil {
			t.Fatal(err)
		}
	}
	link("a", "older", 3*24*time.Hour, false, false)
	link("b", "fresh", time.Hour, false, false)
	link("b2", "fresh", 2*time.Hour, false, false)
	link("c", "revoked", time.Hour, true, false)
	link("d", "ranout", time.Hour, false, true)
	link("e", "stale", 40*24*time.Hour, false, false)
	link("f", "nostore", time.Hour, false, false)
	v := &warmView{fakeLibrary: fakeLibrary{heads: map[string]int64{"older": 2, "fresh": 5, "revoked": 1, "ranout": 1, "stale": 1}}}
	r.svc.UseView(v)
	n, err := r.svc.warmOpened(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"fresh", "older"}; n != 2 || !reflect.DeepEqual(v.warmed, want) {
		t.Fatalf("laid out: %d %v, want %v", n, v.warmed, want)
	}
}
