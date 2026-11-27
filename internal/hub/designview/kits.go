package designview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/design"
	"github.com/ly-solutions/harnessly/internal/designkit"
	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/safego"
)

// HRN-1799 (stage 4). The reads of UI kits — the kits' canvases, a theme's canvas, a kit's picture or font,
// the fonts and the slide layouts the kits offer, a kit's versions — are answered by the hub from the space's kits
// collection when the machine is off: the kits a person may see are laid out in a folder of this hub at their newest
// revisions, and the runtime's own code answers from it, as it answers from the machine's kits folder.

// keepKits is how many sets of kits stay laid out.
const keepKits = 16

// kitRoutes are the read routes answered from the store, by the runtime's handler of each.
var kitRoutes = map[string]func(a *design.API, w http.ResponseWriter, r *http.Request){
	"kit/canvases": answer((*design.API).KitCanvasList),
	"kit/canvas":   answer((*design.API).KitCanvas),
	"kit/fonts":    answer((*design.API).KitFontsList),
	"kit/layouts":  answer((*design.API).KitLayoutsList),
	"kits/state":   answer((*design.API).KitVersionsState),
	"kit/file":     (*design.API).KitFile,
}

func answer(f func(a *design.API, r *http.Request) design.Answer) func(a *design.API, w http.ResponseWriter, r *http.Request) {
	return func(a *design.API, w http.ResponseWriter, r *http.Request) {
		ans := f(a, r)
		gate.WriteJSON(w, ans.Status, ans.Body, ans.Header)
	}
}

// KitRoute says whether route (after design/) is a read of kits the store answers.
func KitRoute(route string) bool { _, ok := kitRoutes[route]; return ok }

type kitSet struct {
	ready chan struct{}
	err   error
	api   *design.API
	dir   string
}

// ServeKits answers a read of kits (route as KitRoute takes it) from the newest revisions of the kits named in the
// space's kits collection; a kit the collection has nothing of is left out.
func (v *View) ServeKits(w http.ResponseWriter, r *http.Request, spaceID string, kits []string, route string) error {
	serve, ok := kitRoutes[route]
	if !ok || v.o.Kits == nil {
		return ErrMissing
	}
	set, err := v.kitSet(r.Context(), spaceID, kits)
	if err != nil {
		return err
	}
	serve(set.api, w, r)
	return nil
}

// kitSet is the kits laid out at their newest revisions, one folder for each set of kit revisions.
func (v *View) kitSet(ctx context.Context, spaceID string, kits []string) (*kitSet, error) {
	if strings.ContainsAny(spaceID, "/\\.") {
		return nil, ErrMissing
	}
	room, err := v.o.Kits(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	sort.Strings(kits)
	heads := map[string]int64{}
	h := newHasher()
	for _, kit := range kits {
		if !validKit(kit) {
			continue
		}
		es, err := room.HistoryUnder(ctx, kit+"/", 1)
		if err != nil {
			return nil, err
		}
		if len(es) == 0 {
			continue
		}
		heads[kit] = es[0].Rev
		fmt.Fprintf(h, "%s@%d\n", kit, es[0].Rev)
	}
	key := spaceID + "/" + h.hex()[:24]
	v.mu.Lock()
	if v.kits == nil {
		v.kits = map[string]*kitSet{}
	}
	s := v.kits[key]
	if s == nil {
		s = &kitSet{ready: make(chan struct{}), dir: filepath.Join(v.o.Dir, "kits", spaceID, h.hex()[:24])}
		v.kits[key] = s
		safego.Go("designview kits", func() { v.fillKits(s, room, spaceID, heads) })
		v.kitOrder = append(v.kitOrder, key)
		v.evictKitsLocked()
	} else {
		v.touchKitLocked(key)
	}
	v.mu.Unlock()
	select {
	case <-s.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if s.err != nil {
		v.mu.Lock()
		if v.kits[key] == s {
			delete(v.kits, key)
		}
		v.mu.Unlock()
		return nil, s.err
	}
	return s, nil
}

func validKit(id string) bool {
	return id != "" && len(id) <= 63 && path.Clean(id) == id && !strings.ContainsAny(id, "/\\.") && designkit.ValidID(id)
}

func (v *View) fillKits(s *kitSet, room Room, spaceID string, heads map[string]int64) {
	defer close(s.ready)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	kitsDir := filepath.Join(s.dir, "kits")
	if _, err := os.Stat(kitsDir); err != nil {
		if s.err = v.extractKits(ctx, room, spaceID, heads, s.dir); s.err != nil {
			v.o.Log.Warn("design view: kits not read from the store", "space", spaceID, "error", s.err)
			return
		}
	}
	lib := design.New(design.Config{
		LibraryDir: filepath.Join(s.dir, "library"),
		KitsDir:    kitsDir,
		DataDir:    filepath.Join(v.o.Dir, "data"),
		Passive:    true,
		ShareProbe: func(string) design.ShareFSReport { return design.ShareFSReport{OK: true, At: time.Now()} },
	})
	s.api = &design.API{
		L:           lib,
		PlatformAPI: "/api/platform",
		// The hub let the person in and chose the kits they may see (designaccess): nothing to add.
		Session:    func(*http.Request) bool { return true },
		APIRefusal: func(*http.Request) (int, any, http.Header, bool) { return 0, nil, nil, false },
	}
}

func (v *View) extractKits(ctx context.Context, room Room, spaceID string, heads map[string]int64, dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(dir, ".fill-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "library"), 0o700); err != nil {
		return err
	}
	pruneMu.RLock()
	defer pruneMu.RUnlock()
	var total int64
	files := map[string]string{}
	var shas []string
	for kit, at := range heads {
		snapshot, err := room.Snapshot(ctx, kit+"/", at)
		if err != nil {
			return err
		}
		if len(files)+len(snapshot.Entries) > maxFiles {
			return fmt.Errorf("designview: over %d files in the kits", maxFiles)
		}
		for _, e := range snapshot.Entries {
			if !strings.HasPrefix(e.Path, kit+"/") || path.Clean(e.Path) != e.Path || strings.Contains(e.Path, "../") || !filestore.ValidSha(e.Sha) {
				continue
			}
			if total += e.Size; total > v.o.MaxBytes {
				return fmt.Errorf("designview: the kits are over %d bytes", v.o.MaxBytes)
			}
			files[e.Path] = e.Sha
			shas = append(shas, e.Sha)
		}
	}
	kept, err := v.blobs(ctx, room, spaceID, shas)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(tmp, "kits"), 0o700); err != nil {
		return err
	}
	for rel, sha := range files {
		file := filepath.Join(tmp, "kits", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return err
		}
		if err := os.Link(kept[sha], file); err != nil {
			if err := copyFile(kept[sha], file); err != nil {
				return err
			}
		}
	}
	for _, sub := range []string{"kits", "library"} {
		if err := os.Rename(filepath.Join(tmp, sub), filepath.Join(dir, sub)); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return nil
}

func (v *View) touchKitLocked(key string) {
	for i, k := range v.kitOrder {
		if k == key {
			v.kitOrder = append(append(v.kitOrder[:i:i], v.kitOrder[i+1:]...), key)
			return
		}
	}
}

// evictKitsLocked drops the sets of kits least recently read past keepKits.
func (v *View) evictKitsLocked() {
	for len(v.kitOrder) > keepKits {
		old := v.kitOrder[0]
		v.kitOrder = v.kitOrder[1:]
		s := v.kits[old]
		delete(v.kits, old)
		if s == nil {
			continue
		}
		safego.Go("designview prune kits", func() {
			<-s.ready
			// A read that still holds the set finishes first: it was given its answer's files before.
			time.Sleep(time.Minute)
			_ = os.RemoveAll(s.dir)
			v.pruneBlobs()
		})
	}
}

// projectKits is the kits folder a project's library at base reads: its pinned kit laid out at the newest revision of the
// kits collection, so a guest link, the render worker and the hub's library show a board on a kit with the kit's styles
// while the machine is off. "" for a project on no kit, or a kit the store cannot give now.
func (v *View) projectKits(ctx context.Context, spaceID, base, project string) string {
	if v.o.Kits == nil {
		return ""
	}
	pin, ok := design.KitPinIn(base, project)
	if !ok || !validKit(pin.ID) {
		return ""
	}
	set, err := v.kitSet(ctx, spaceID, []string{pin.ID})
	if err != nil {
		v.o.Log.Warn("design view: the project's kit not read from the store", "space", spaceID, "project", project, "kit", pin.ID, "error", err)
		return ""
	}
	return filepath.Join(set.dir, "kits")
}
