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
	waiter, _ := s.view.(LibraryWaiter)
	_, stored := s.view.(Librarian)
	since, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil || since < 0 {
		since = 0
	}
	if m.SpaceID == "" || (stored && waiter == nil) {
		s.writeError(w, fail("store_off", "Хранилище дизайнов не подключено"))
		return
	}
	s.watchView(w, r, u, m, waiter, since)
}

// watchView holds the poll until the person's view of the library differs from the one the page has: the
// files' revision in the store (waiter; nil without a store) or what they may see of the space. It answers the view's
// revision when it moved, or as it is when the wait ends. A view that cannot be read now is an error, never the revision
// the page already holds: a page that fails to refresh must not go on showing a grant as current.
func (s *Service) watchView(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine, waiter LibraryWaiter, since int64) {
	ctx, cancel := context.WithTimeout(r.Context(), libraryWait)
	defer cancel()
	watch := &accessWatch{s: s, u: u, m: m}
	head := int64(-1)
	rev := int64(-1)
	for {
		if waiter != nil {
			// Returns at once the first time (nothing is past -1); after that when the files move, or after a poll.
			step, stop := context.WithTimeout(ctx, grantedPoll)
			h, err := waiter.WaitLibrary(step, m.SpaceID, head)
			stop()
			if err != nil {
				if s.endedWait(w, r, ctx, rev) {
					return
				}
				s.writeError(w, err)
				return
			}
			head = h
		}
		if ctx.Err() != nil && rev >= 0 {
			// The wait ended: the view as last read.
			s.endedWait(w, r, ctx, rev)
			return
		}
		digest, err := watch.current(ctx)
		if err != nil {
			if s.endedWait(w, r, ctx, rev) {
				return
			}
			s.writeError(w, err)
			return
		}
		rev = s.viewRev(u, m, fmt.Sprintf("%d\x00%s", head, digest))
		if rev > since || ctx.Err() != nil {
			if r.Context().Err() != nil {
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, map[string]int64{"rev": rev})
			return
		}
		if waiter == nil {
			select {
			case <-ctx.Done():
			case <-time.After(grantedPoll):
			}
		}
	}
}

// endedWait answers a poll whose wait ran out: with the view's revision as last read (rev), or, when none was read, an
// error the page asks again after. True when the answer is given (or nobody waits for one any more).
func (s *Service) endedWait(w http.ResponseWriter, r *http.Request, ctx context.Context, rev int64) bool {
	if r.Context().Err() != nil {
		return true
	}
	if ctx.Err() == nil {
		return false
	}
	if rev >= 0 {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]int64{"rev": rev})
		return true
	}
	s.writeError(w, fail("unavailable", "Права доступа сейчас не прочитать"))
	return true
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
