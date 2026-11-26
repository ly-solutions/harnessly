package filesync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path"
	"sync"

	"github.com/ly-solutions/harnessly/internal/filestore"
)

// contents travel several at a time; revisions are still applied and committed one by one, in their order.
// A download is fetched ahead into the agent's tmp folder, a window of revisions at once, and the apply that follows
// reads it from there; an upload sends a small group's contents together before its commits.

// transferWorkers is how many contents travel at once (a variable: a measurement sets 1).
var transferWorkers = 8

// A window of revisions fetched ahead holds at most this many files and about this many bytes on the disk.
const (
	windowFiles = 64
	windowBytes = 64 << 20
)

// together runs f for 0..n-1, at most transferWorkers at once, and waits for all of them.
func together(n int, f func(i int)) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, max(1, transferWorkers))
	for i := range n {
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			f(i)
		}()
	}
	wg.Wait()
}

// window is how many of the entries, from the first, are fetched ahead together; at least one.
func window(entries []filestore.Entry) int {
	var held int64
	for i, e := range entries {
		if !e.Deleted {
			held += e.Size
		}
		if i > 0 && (i >= windowFiles || held > windowBytes) {
			return i
		}
	}
	return len(entries)
}

// prefetch downloads the contents the entries are likely to need into the tmp folder. A download that fails is left to
// the apply, which asks again and reports it. The returned func removes what was fetched.
func (a *Agent) prefetch(ctx context.Context, entries []filestore.Entry) (release func()) {
	var want []filestore.Entry
	if transferWorkers > 1 && len(entries) > 1 {
		a.mu.Lock()
		seen := map[string]bool{}
		for _, e := range entries {
			if !seen[e.Sha] && a.needsContents(e) {
				seen[e.Sha] = true
				want = append(want, e)
			}
		}
		a.mu.Unlock()
	}
	if len(want) == 0 {
		return func() {}
	}
	files := make([]string, len(want))
	together(len(want), func(i int) {
		name, err := a.download(ctx, want[i].Sha)
		if err != nil {
			a.log.Debug("fetch ahead", "path", want[i].Path, "err", err)
			return
		}
		files[i] = name
	})
	a.mu.Lock()
	if a.fetched == nil {
		a.fetched = map[string]string{}
	}
	for i, name := range files {
		if name != "" {
			a.fetched[want[i].Sha] = name
		}
	}
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for i, name := range files {
			if name != "" {
				delete(a.fetched, want[i].Sha)
				_ = a.root.Remove(name)
			}
		}
	}
}

// needsContents tells whether applying e is likely to download it: not a revision the agent has, not contents another
// file of the folder holds, not a file already on the disk that may well be the same (a folder that joins the room).
func (a *Agent) needsContents(e filestore.Entry) bool {
	if e.Deleted || e.Size > windowBytes || !a.policy.Synced(e.Path) {
		return false
	}
	k, err := a.state.get(e.Path)
	if err != nil || k != nil && k.Rev >= e.Rev {
		return false
	}
	tag := e.Tag
	if tag == "" {
		tag = e.Sha
	}
	if paths, err := a.state.withSha(tag, 1); err != nil || len(paths) > 0 {
		return false
	}
	if k == nil {
		if fi, err := a.root.Lstat(e.Path); err == nil && fi.Size() == e.Size {
			return false
		}
	}
	return true
}

// download writes the blob into a file of the tmp folder and returns its name.
func (a *Agent) download(ctx context.Context, sum string) (string, error) {
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	name := path.Join(a.tmp, "ahead-"+hex.EncodeToString(rnd[:]))
	f, err := a.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	err = a.hub.GetBlob(ctx, sum, &limitWriter{w: f, n: filestore.MaxFileSize + 1024})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = a.root.Remove(name)
		return "", err
	}
	return name, nil
}

// ahead returns the contents fetched ahead for this blob, if they are whole.
func (a *Agent) ahead(sum string) ([]byte, bool) {
	name, ok := a.fetched[sum]
	if !ok {
		return nil, false
	}
	f, err := a.root.Open(name)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || sha(data) != sum {
		return nil, false
	}
	return data, true
}

// applyAll applies the revisions in their order, their contents fetched ahead window by window; each is called with
// what its apply answered and stops the run by returning an error.
func (a *Agent) applyAll(ctx context.Context, entries []filestore.Entry, each func(filestore.Entry, error) error) error {
	for len(entries) > 0 {
		n := window(entries)
		release := a.prefetch(ctx, entries[:n])
		for _, e := range entries[:n] {
			if err := each(e, a.Apply(ctx, e)); err != nil {
				release()
				return err
			}
		}
		release()
		entries = entries[n:]
	}
	return nil
}

// uploadGroup sends together the contents of the group the store lacks; failed holds the sums that did not arrive.
func (a *Agent) uploadGroup(ctx context.Context, group []*outgoing, missing map[string]bool) (failed map[string]error) {
	var want []*outgoing
	for _, o := range group {
		if missing[o.sum] && !a.groupRefused(o.rel) {
			delete(missing, o.sum)
			want = append(want, o)
		}
	}
	errs := make([]error, len(want))
	together(len(want), func(i int) { errs[i] = a.hub.PutBlob(ctx, want[i].sum, want[i].blob) })
	failed = map[string]error{}
	for i, err := range errs {
		if err != nil {
			failed[want[i].sum] = err
			missing[want[i].sum] = true
		}
	}
	return failed
}
