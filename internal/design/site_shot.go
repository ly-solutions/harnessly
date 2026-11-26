package design

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/locale"
)

// The visible part of a board as a picture: what the person drew over is sent to the agent drawn over this.
// The board is opened afresh in the machine's browser at the person's viewport, scrolled where they stood.

const (
	siteShotMaxSide   = 4096
	siteShotMaxScroll = 1_000_000
)

// SiteShot is a PNG of a board opened at w×h CSS px, scrolled to y, at scale (1 or 2).
func (l *Library) SiteShot(artifact, board string, w, h, y int, scale float64) ([]byte, error) {
	if !l.hasBoard(artifact, board) {
		return nil, &DeckError{Status: http.StatusNotFound, Msg: locale.Pick("No such board", "Нет такой доски")}
	}
	s, err := l.newDeckShooter(artifact)
	if err != nil {
		return nil, err
	}
	defer s.close()
	tab, err := s.tab()
	if err != nil {
		return nil, err
	}
	defer tab.close()
	if err := tab.view(w, h, scale); err != nil {
		return nil, err
	}
	if err := tab.navigate(s.url(deckSlideSrc{Path: board})); err != nil {
		return nil, err
	}
	if _, err := tab.eval(deckReady); err != nil {
		return nil, err
	}
	if _, err := tab.eval("window.scrollTo(0, " + strconv.Itoa(y) + "); true"); err != nil {
		return nil, err
	}
	sleepSettle()
	return tab.png()
}

// SiteShotRoute is POST {artifact, board, w, h, y, scale?} (view): the PNG of the board's visible part.
func (a *API) SiteShotRoute(r *http.Request) Answer {
	body, id, no := a.deckGuarded(r, RoleView, http.MethodPost)
	if no != nil {
		return *no
	}
	board, _ := strField(body, "board")
	num := func(k string) int {
		v, _ := Field(body, k).(float64)
		return int(v)
	}
	w, h, y := num("w"), num("h"), num("y")
	scale, _ := Field(body, "scale").(float64)
	if scale != 2 {
		scale = 1
	}
	if board == "" || w < 16 || h < 16 || w > siteShotMaxSide || h > siteShotMaxSide || y < 0 || y > siteShotMaxScroll {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("board, w, h and y are expected", "Нужны board, w, h и y")), noStore())
	}
	png, err := a.L.SiteShot(id, board, w, h, y, scale)
	if err != nil {
		var de *DeckError
		if errors.As(err, &de) {
			return JSON(r, de.Status, errorBody(de.Msg), noStore())
		}
		return a.deckFail(r, err, "take the picture of the board")
	}
	return Answer{Status: http.StatusOK, Body: gate.Raw{ContentType: "image/png", Body: png}, Header: noStore()}
}
