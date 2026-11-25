package designaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// GET <platform>/design/store-library/changes?since=<rev> holds the page's ask until the space's library in the
// hub's store has a revision past since (a project drawn or edited on any machine, a shelf moved), or LibraryWaitMax
// passes, and answers {"rev": <newest>}. The page reads the library again when rev grows, instead of asking on a timer.
// Behind hub-design-home: while it is off, 404 home_off and the page keeps its timer.

// LibraryWaitMax is how long the hub holds a long poll of the library.
const LibraryWaitMax = 25 * time.Second

// libraryWait is LibraryWaitMax; tests shorten it.
var libraryWait = LibraryWaitMax

// LibraryWaiter waits for the space's library in the store to move past a revision.
type LibraryWaiter interface {
	WaitLibrary(ctx context.Context, spaceID string, since int64) (int64, error)
}

func (s *Service) libraryChanges(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine) {
	if s.homeOn == nil || !s.homeOn(r.Context(), normEmail(u.Email), m.ID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "home_off", "error": "Главная Синопии одним ответом выключена"})
		return
	}
	waiter, ok := s.view.(LibraryWaiter)
	_, stored := s.view.(Librarian)
	since, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil || since < 0 {
		since = 0
	}
	if !ok && !stored && m.SpaceID != "" {
		s.grantedChanges(w, r, u, m, since)
		return
	}
	if !ok || m.SpaceID == "" {
		s.writeError(w, fail("store_off", "Хранилище дизайнов не подключено"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), libraryWait)
	defer cancel()
	rev, err := waiter.WaitLibrary(ctx, m.SpaceID, since)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]int64{"rev": rev})
}

// grantedPoll is how often the hub looks again at a person's grants while it holds a poll.
var grantedPoll = 3 * time.Second

// grantedRevs is the revision a person's granted library has now, per person and space. A revision is a clock reading
// taken when the library's content was first seen to differ, so it only grows, also across a restart of the hub.
type grantedRevs struct {
	mu   sync.Mutex
	seen map[string]grantedRev
}

type grantedRev struct {
	digest string
	rev    int64
}

const maxGrantedRevs = 4096

// grantedDigest is what a page shows of the granted library: a change of a grant, its role or expiry, a revocation, a
// retitled or moved project changes it, and nothing else does.
func grantedDigest(lib storeLibrary) string {
	h := sha256.New()
	for _, a := range lib.Artifacts {
		until := ""
		if a.AccessUntil != nil {
			until = a.AccessUntil.UTC().Format(time.RFC3339Nano)
		}
		owner := ""
		if a.Owner != nil {
			owner = a.Owner.Name
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\n", a.ID, a.CardKey, a.Title, a.Role, a.AccessSource, until, owner, a.UpdatedAt)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// grantedRev is the person's revision now: the one kept while the library reads the same, else a newer one.
func (s *Service) grantedRev(ctx context.Context, u store.User, m store.Machine) (int64, error) {
	// Keys are on: a card of a copy is a card of its own, and the digest tells them apart.
	lib, err := s.grantedLibrary(ctx, u, m, true)
	if err != nil {
		return 0, err
	}
	digest := grantedDigest(lib)
	key := u.ID + "|" + m.SpaceID
	s.revs.mu.Lock()
	defer s.revs.mu.Unlock()
	if s.revs.seen == nil || len(s.revs.seen) >= maxGrantedRevs {
		s.revs.seen = map[string]grantedRev{}
	}
	cur := s.revs.seen[key]
	if cur.digest != digest {
		next := s.now().UnixMilli()
		if next <= cur.rev {
			next = cur.rev + 1
		}
		cur = grantedRev{digest: digest, rev: next}
		s.revs.seen[key] = cur
	}
	return cur.rev, nil
}

// grantedChanges is the poll of the library with no file store: it holds until what the person is granted has changed
// since the revision the page has, or the wait ends. A grant that cannot be read now is an error, never the revision
// the page already holds: a page that fails to refresh must not go on showing the grant as current.
func (s *Service) grantedChanges(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine, since int64) {
	ctx, cancel := context.WithTimeout(r.Context(), libraryWait)
	defer cancel()
	tick := time.NewTicker(grantedPoll)
	defer tick.Stop()
	for {
		rev, err := s.grantedRev(ctx, u, m)
		if err != nil {
			if ctx.Err() != nil && r.Context().Err() == nil {
				// The wait ended while reading: the page asks again.
				s.writeError(w, fail("unavailable", "Права доступа сейчас не прочитать"))
				return
			}
			s.writeError(w, err)
			return
		}
		if rev > since {
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, map[string]int64{"rev": rev})
			return
		}
		select {
		case <-ctx.Done():
			if r.Context().Err() != nil {
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, map[string]int64{"rev": rev})
			return
		case <-tick.C:
		}
	}
}
