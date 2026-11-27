// Package renderworker is `runtime render-worker`: the hub's worker that draws the design pictures of machines that are
// off (docs/hub/render-worker.md). It claims a job from the hub, opens the board from the hub's store at the
// job's address in a headless Chrome, and sends the picture back. It keeps nothing of its own: the hub holds the queue
// and the pictures, so any number of workers can run and any of them can die.
package renderworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ly-solutions/harnessly/internal/design"
)

// The hub's routes of the worker (designrender.ClaimPath, designrender.DonePath).
const (
	claimPath = "/agent/render/claim"
	donePath  = "/agent/render/done"
)

// job is what a claim gives (designrender.Job).
type job struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Board   string `json:"board"`
	Kind    string `json:"kind"`
	Rev     int64  `json:"rev"`
	BoardW  int    `json:"boardW"`
	BoardH  int    `json:"boardH"`
	Path    string `json:"path"`
}

// shooter draws a board (design.Shooter).
type shooter interface {
	ShootThumb(ctx context.Context, url, boardPath string, w, h int) ([]byte, error)
	ShootPage(ctx context.Context, url, boardPath string, w, h int) ([]byte, error)
}

// kindPage is a page of a guest link's snapshot (designrender.KindPage): drawn only when it draws cleanly.
const kindPage = "page"

// Worker is one worker's loop.
type Worker struct {
	Hub    string // the hub's address, without a trailing slash
	Token  string
	Shoot  shooter
	Client *http.Client
	Log    *slog.Logger
}

// Main is `runtime render-worker`: HUB_URL and HUB_RENDER_TOKEN from the environment (or -hub), Chrome from
// RUNTIME_DESIGN_CHROME or the usual places.
func Main(args []string) error {
	fs := flag.NewFlagSet("render-worker", flag.ContinueOnError)
	hub := fs.String("hub", os.Getenv("HUB_URL"), "the hub's address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	token := strings.TrimSpace(os.Getenv("HUB_RENDER_TOKEN"))
	base := strings.TrimRight(strings.TrimSpace(*hub), "/")
	if u, err := url.Parse(base); base == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("render-worker: HUB_URL must be the hub's http(s) address")
	}
	if token == "" {
		return errors.New("render-worker: HUB_RENDER_TOKEN is not set")
	}
	sh, err := design.NewShooter("")
	if err != nil {
		return err
	}
	defer sh.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	log.Info("render-worker: started", "hub", base)
	w := &Worker{Hub: base, Token: token, Shoot: sh, Client: &http.Client{Timeout: 60 * time.Second}, Log: log}
	w.Run(ctx)
	return nil
}

// Run claims and draws until ctx ends; a hub that does not answer is asked again after a pause. A check or an export
// runs beside the pictures, one at a time: while it runs the worker asks for pictures only.
func (w *Worker) Run(ctx context.Context) {
	pause := time.Second
	var busy atomic.Bool
	var works sync.WaitGroup
	defer works.Wait()
	for ctx.Err() == nil {
		j, err := w.claim(ctx, busy.Load())
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.Log.Warn("render-worker: the hub did not give a job", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(pause):
			}
			pause = min(pause*2, time.Minute)
			continue
		}
		pause = time.Second
		if j == nil {
			continue
		}
		if isWork(j.Kind) {
			busy.Store(true)
			works.Add(1)
			go func(j job) {
				defer works.Done()
				defer busy.Store(false)
				w.work(ctx, j)
			}(*j)
			continue
		}
		w.draw(ctx, *j)
	}
}

// Once claims one job and draws it; false when the hub had none.
func (w *Worker) Once(ctx context.Context) (bool, error) {
	j, err := w.claim(ctx, false)
	if err != nil || j == nil {
		return false, err
	}
	w.draw(ctx, *j)
	return true, nil
}

func (w *Worker) draw(ctx context.Context, j job) {
	if isWork(j.Kind) {
		w.work(ctx, j)
		return
	}
	if !strings.HasPrefix(j.Path, "/") || strings.HasPrefix(j.Path, "//") {
		w.done(ctx, j, nil, "a bad address")
		return
	}
	started := time.Now()
	shoot := w.Shoot.ShootThumb
	if j.Kind == kindPage {
		shoot = w.Shoot.ShootPage
	}
	pic, err := shoot(ctx, w.Hub+j.Path, j.Board, j.BoardW, j.BoardH)
	if err != nil {
		w.Log.Warn("render-worker: a board was not drawn", "project", j.Project, "board", j.Board, "error", err)
		w.done(ctx, j, nil, err.Error())
		return
	}
	w.done(ctx, j, pic, "")
	w.Log.Info("render-worker: drew a board", "project", j.Project, "board", j.Board, "rev", j.Rev, "bytes", len(pic),
		"ms", time.Since(started).Milliseconds())
}

func (w *Worker) claim(ctx context.Context, picturesOnly bool) (*job, error) {
	// A claim waits on the hub for a job up to about 20 s.
	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	target := w.Hub + claimPath
	if picturesOnly {
		target += "?pictures=1"
	}
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+w.Token)
	resp, err := w.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, nil
	case http.StatusOK:
		var j job
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&j); err != nil {
			return nil, err
		}
		return &j, nil
	}
	return nil, fmt.Errorf("claim: %s", resp.Status)
}

// done sends the picture, or why there is none.
func (w *Worker) done(ctx context.Context, j job, pic []byte, why string) {
	q := url.Values{"id": {j.ID}}
	if why != "" {
		q.Set("error", why)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.Hub+donePath+"?"+q.Encode(), bytes.NewReader(pic))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+w.Token)
	if pic != nil {
		req.Header.Set("Content-Type", "image/jpeg")
	}
	resp, err := w.Client.Do(req)
	if err != nil {
		w.Log.Warn("render-worker: the hub did not take the answer", "project", j.Project, "error", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		w.Log.Warn("render-worker: the hub refused the answer", "project", j.Project, "status", resp.Status)
	}
}
