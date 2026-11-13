package design

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/locale"
	"github.com/ly-solutions/harnessly/internal/safego"
)

// A project the page could not open from the hub's store is opened from the machine, and the page asks the
// machine to put it into the store at once (POST <platform api>/design/store-push {project}), so the next look reads
// the hub. The sync would get there by itself; this only moves the project to the front of it.

// StorePusher sends a project's folder to the store now (the runtime's store link).
type StorePusher interface {
	PushProject(ctx context.Context, project string) error
}

type storePusherBox struct{ p StorePusher }

// storePushTimeout bounds one push: a large project's first upload.
const storePushTimeout = 5 * time.Minute

// storePushes are the projects being pushed now: a second ask for one of them joins the first.
var storePushes sync.Map

// UseStorePusher gives the library the way into the store; nil takes it away.
func (l *Library) UseStorePusher(p StorePusher) {
	if p == nil {
		l.pusher.Store(nil)
		return
	}
	l.pusher.Store(&storePusherBox{p: p})
}

func (l *Library) storePusher() StorePusher {
	if b := l.pusher.Load(); b != nil {
		return b.p
	}
	return nil
}

// StorePush is POST {project} (a session, view): {ok: true} once the push has started; 404 for a project the person
// cannot see, 409 where the library is not synced with a store.
func (a *API) StorePush(r *http.Request) Answer {
	body, no := a.Guard(r, http.MethodPost)
	if no != nil {
		return *no
	}
	project, _ := strField(body, "project")
	if !IsLibraryID(project) || a.L.ReadArtifact(project) == nil || !a.L.CanFor(project, RoleView, a.PersonOf(r)) {
		return JSON(r, http.StatusNotFound, errorBody(locale.Pick("The design is gone", "Макета больше нет")), noStore())
	}
	p := a.L.storePusher()
	if p == nil {
		return JSON(r, http.StatusConflict, errorBody(locale.Pick("The shared store is not connected", "Общее хранилище не подключено")), noStore())
	}
	key := a.L.Root() + "\x00" + project
	if _, running := storePushes.LoadOrStore(key, true); !running {
		safego.Go("design store push", func() {
			defer storePushes.Delete(key)
			ctx, cancel := context.WithTimeout(context.Background(), storePushTimeout)
			defer cancel()
			if err := p.PushProject(ctx, project); err != nil {
				a.warn("design not put into the store", err)
			}
		})
	}
	return JSON(r, http.StatusOK, map[string]bool{"ok": true}, noStore())
}
