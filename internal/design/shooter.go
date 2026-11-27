package design

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Shooter draws boards headless at an address, the way the library draws its gallery pictures, for the hub's render
// worker (docs/hub/render-worker.md): the worker has no library of its own, only the board's address on the
// hub. One browser serves every shot, one at a time, and is started again when it dies.
type Shooter struct {
	l   *Library
	bin string

	mu sync.Mutex
	br *browser
}

// NewShooter finds Chrome (chrome, else RUNTIME_DESIGN_CHROME, else the usual places); an error without one.
func NewShooter(chrome string) (*Shooter, error) {
	l := &Library{cfg: Config{Chrome: chrome, Texts: Texts{Tag: "render"}}}
	bin := l.findChrome()
	if bin == "" {
		return nil, errors.New("design: no Chrome to draw with")
	}
	return &Shooter{l: l, bin: bin}, nil
}

// ThumbSize is the size a board's gallery picture is drawn at: its own (w, h) when the project says it, a phone for a
// page built for one, a desktop otherwise.
func ThumbSize(boardPath string, w, h int) (width, height int, mobile bool) {
	b := Board{Path: boardPath}
	if w > 0 && h > 0 {
		b.W, b.H = &w, &h
	}
	v := thumbView(b)
	return v.Width, v.Height, v.Mobile
}

// ShootThumb draws the board at url as its gallery picture (JPEG). A page with errors still gives a picture, as on the
// machine; one that did not draw at all is an error.
func (s *Shooter) ShootThumb(ctx context.Context, url, boardPath string, w, h int) ([]byte, error) {
	return s.shootBoard(ctx, url, boardPath, w, h, false)
}

// ShootPage draws the board at url as a page of a guest link's snapshot: as the gallery picture, but only when the page
// drew cleanly (loaded, no script errors, no file failed), as the machine's own snapshot asks (Library.pinnedRaster).
func (s *Shooter) ShootPage(ctx context.Context, url, boardPath string, w, h int) ([]byte, error) {
	return s.shootBoard(ctx, url, boardPath, w, h, true)
}

func (s *Shooter) shootBoard(ctx context.Context, url, boardPath string, w, h int, clean bool) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.br != nil {
		select {
		case <-s.br.exited:
			s.br = nil
		default:
		}
	}
	if s.br == nil {
		br, err := s.l.launchBrowser(s.bin)
		if err != nil {
			return nil, err
		}
		s.br = br
	}
	br := s.br
	t, err := br.cdp.send("Target.createTarget", map[string]any{"url": "about:blank"}, "")
	if err != nil {
		s.drop()
		return nil, err
	}
	targetID := t["targetId"]
	dir, err := os.MkdirTemp("", "render-shot-")
	if err != nil {
		return nil, err
	}
	file := filepath.Join(dir, "shot.jpg")
	defer func() {
		_, _ = br.cdp.send("Target.closeTarget", map[string]any{"targetId": targetID}, "")
		_ = os.RemoveAll(dir)
	}()
	a, err := br.cdp.send("Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "")
	if err != nil {
		return nil, err
	}
	session, _ := a["sessionId"].(string)
	for _, d := range []string{"Page", "Runtime", "Log", "Network", "DOM"} {
		if _, err := br.cdp.send(d+".enable", nil, session); err != nil {
			return nil, err
		}
	}
	width, height, mobile := ThumbSize(boardPath, w, h)
	done := make(chan shotResult, 1)
	go func() { done <- shoot(br.cdp, session, url, shotView{width, height, mobile}, file, false) }()
	select {
	case res := <-done:
		if !res.Saved {
			return nil, errors.New("design: the board was not drawn")
		}
		if clean && (!res.Loaded || len(res.Errors) > 0 || len(res.Failed) > 0) {
			return nil, fmt.Errorf("design: the board did not draw cleanly (loaded %v, %d errors, %d files failed)",
				res.Loaded, len(res.Errors), len(res.Failed))
		}
		return os.ReadFile(file)
	case <-ctx.Done():
	case <-time.After(thumbRenderWait):
	}
	// The browser may be stuck on the page: it is started anew for the next shot.
	s.drop()
	<-done
	return nil, errors.New("design: the board took too long to draw")
}

func (s *Shooter) drop() {
	if s.br != nil {
		s.br.close()
		s.br = nil
	}
}

// Close stops the browser.
func (s *Shooter) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drop()
}
