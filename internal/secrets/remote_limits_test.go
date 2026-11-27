package secrets

import (
	"context"
	"testing"
)

// A secret limited to a project on one machine of the person is limited on every other: the limits lie with the value.
func TestServerLimitsReachTheOtherMachine(t *testing.T) {
	kv := newMemKV()
	a, b := NewRemote(kv, "me", t.TempDir()), NewRemote(kv, "me", t.TempDir())
	if _, _, err := a.Put("key", "value-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Metadata("key", []string{"https://example.com"}, []string{"project-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Reveal([]string{"key"}, Use{Project: "project-b"}); ReasonOf(err) != ReasonNotFound {
		t.Fatalf("another project on another machine: %v", err)
	}
	if _, err := b.Lookup("key", "project-b"); ReasonOf(err) != ReasonNotFound {
		t.Fatalf("lookup: %v", err)
	}
	if v, err := b.Reveal([]string{"key"}, Use{Project: "project-a", Origin: "https://example.com"}); err != nil || v["key"] != "value-1" {
		t.Fatalf("the allowed project: %v", err)
	}
	// A new value keeps the limits; a card's grant on one machine reaches the other.
	if _, _, err := b.Put("key", "value-2"); err != nil {
		t.Fatal(err)
	}
	if err := b.Grant("key", "project-b", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Reveal([]string{"key"}, Use{Project: "project-b"}); err != nil {
		t.Fatalf("granted elsewhere: %v", err)
	}
	if _, err := a.Reveal([]string{"key"}, Use{Project: "project-c"}); ReasonOf(err) != ReasonNotFound {
		t.Fatalf("still limited: %v", err)
	}
	// Limits taken away on one machine are away on both.
	if _, err := a.Metadata("key", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Reveal([]string{"key"}, Use{Project: "project-c"}); err != nil {
		t.Fatalf("unlimited again: %v", err)
	}
}

// Limits set before they travelled go to the server the first time their machine uses the secret.
func TestServerLimitsOfAnOlderLedgerTravelOnUse(t *testing.T) {
	kv := newMemKV()
	a, b := NewRemote(kv, "me", t.TempDir()), NewRemote(kv, "me", t.TempDir())
	if _, _, err := a.Put("key", "value-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Metadata("key", nil, []string{"project-a"}); err != nil {
		t.Fatal(err)
	}
	_ = kv.Write(context.Background(), "me/key", map[string]string{ValueKey: "value-1"}) // as an older runtime left it
	if _, err := a.Reveal([]string{"key"}, Use{Project: "project-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Reveal([]string{"key"}, Use{Project: "project-b"}); ReasonOf(err) != ReasonNotFound {
		t.Fatalf("another project: %v", err)
	}
}

// A hub of before takes one value only: the secret is still kept, the limits stay on this machine.
func TestServerThatKeepsOneValue(t *testing.T) {
	kv := &oneValueKV{newMemKV()}
	a := NewRemote(kv, "me", t.TempDir())
	if _, _, err := a.Put("key", "value-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Metadata("key", nil, []string{"project-a"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Put("key", "value-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Reveal([]string{"key"}, Use{Project: "project-b"}); ReasonOf(err) != ReasonNotFound {
		t.Fatalf("the ledger still limits: %v", err)
	}
	if v, err := a.Reveal([]string{"key"}, Use{Project: "project-a"}); err != nil || v["key"] != "value-2" {
		t.Fatalf("the allowed project: %v", err)
	}
}

type oneValueKV struct{ *memKV }

func (k *oneValueKV) Write(ctx context.Context, p string, d map[string]string) error {
	if len(d) != 1 {
		return refuse(ReasonEmpty, nil)
	}
	return k.memKV.Write(ctx, p, d)
}
