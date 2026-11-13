package design

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type pushed chan string

func (p pushed) PushProject(_ context.Context, project string) error {
	p <- project
	return nil
}

// a project the page opened from the machine is put into the store at once, on the page's ask.
func TestStorePushSendsTheProjectToTheStore(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root, "deck", "Deck", "")
	l := accessLibrary(t, root)
	a := pagesAPI(l)
	push := func(id string) int {
		r := httptest.NewRequest(http.MethodPost, "/store-push", strings.NewReader(`{"project":"`+id+`"}`))
		r.Header.Set("Content-Type", "application/json")
		return a.StorePush(r).Status
	}
	if c := push("deck"); c != http.StatusConflict {
		t.Fatalf("a library with no store: %d", c)
	}
	got := make(pushed, 2)
	l.UseStorePusher(got)
	if c := push("nothing"); c != http.StatusNotFound {
		t.Errorf("a missing project: %d", c)
	}
	if c := push("../deck"); c != http.StatusNotFound {
		t.Errorf("not a project id: %d", c)
	}
	if c := push("deck"); c != http.StatusOK {
		t.Fatalf("the project: %d", c)
	}
	select {
	case id := <-got:
		if id != "deck" {
			t.Fatalf("pushed %q", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the project was not pushed")
	}
	select {
	case id := <-got:
		t.Fatalf("a refused ask pushed %q", id)
	default:
	}
}
