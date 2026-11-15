package resaccess

import (
	"context"
	"testing"
)

// A chat's snapshot is read by the people of its space and by nobody outside it; one nobody published is closed to
// all, and the sharing API does not take the kind, so it cannot be opened wider.
func TestChatSnapshotIsReadByItsSpaceOnly(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if _, err := s.db.Exec(ctx, `INSERT INTO chat_shares (id, space_id, machine_id, owner_id, owner_email, sha256, expires_at, created_at)
		VALUES ('snap1', 'space1', 'm1', 'u1', 'Author@Example.com', 'abc', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	ref := Ref{Kind: KindChat, ID: "snap1", Space: "space1"}
	role := func(ref Ref, a Actor) string {
		t.Helper()
		r, err := s.Role(ctx, ref, a)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := role(ref, proved("author@example.com")); Rank(r) == 0 {
		t.Fatalf("the author: %q", r)
	}
	if r := role(ref, proved("mate@example.com")); r != "view" {
		t.Fatalf("a person of the space: %q", r)
	}
	away := proved("stranger@example.com")
	away.Outside = true
	if r := role(ref, away); Rank(r) != 0 {
		t.Fatalf("a person outside the space: %q", r)
	}
	for name, other := range map[string]Ref{
		"another space":   {Kind: KindChat, ID: "snap1", Space: "space2"},
		"never published": {Kind: KindChat, ID: "nope", Space: "space1"},
	} {
		if r := role(other, proved("mate@example.com")); Rank(r) != 0 {
			t.Fatalf("%s: %q", name, r)
		}
	}
	if kinds[KindChat] {
		t.Fatal("the sharing API takes a chat's snapshot: it could be opened wider than its space")
	}
}
