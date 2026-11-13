package storelink

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
)

// a project is put into the store on demand, without waiting for the watcher or the rescan.
func TestPushNowSendsAProjectAtOnce(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(Options{DataDir: filepath.Join(dir, "data"), Name: "box", Log: quiet()})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.PushNow(context.Background(), filestore.KindDesign, "p1"); !errors.Is(err, ErrNotSyncing) {
		t.Fatalf("with no sync running: %v", err)
	}
	lib := filepath.Join(dir, "library")
	write(t, lib, "p0/project/index.html", "<h1>first</h1>")
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); l.SyncFolder(ctx, filestore.KindDesign, func() string { return lib }) }()
	defer func() { stop(); <-done }()
	c := l.Client(filestore.KindDesign)
	waitFor(t, 10*time.Second, func() bool {
		e, err := c.Current(context.Background(), "p0/project/index.html")
		return err == nil && e != nil
	})

	write(t, lib, "p1/project/index.html", "<h1>pushed</h1>")
	if err := l.PushNow(context.Background(), filestore.KindDesign, "p1"); err != nil {
		t.Fatal(err)
	}
	e, err := c.Current(context.Background(), "p1/project/index.html")
	if err != nil || e == nil || e.Size != int64(len("<h1>pushed</h1>")) {
		t.Fatalf("the project is not in the store right after the push: %+v %v", e, err)
	}
}
