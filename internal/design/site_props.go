package design

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
	"github.com/ly-solutions/harnessly/internal/locale"
)

// The settings a board offers (its data-props, the screen state and theme of a wrapper board) as everybody who looks at
// the project sees them: the panel «Макет → Настройки» keeps what the person picked in meta/props.json, every
// viewer's panel applies it to the page, and the agent reads and sets it (`harnessly design props`). A value the board
// does not take any more is left to the page to refuse; the file only says what was picked.

// SitePropsFile is the project's shared settings, under meta/.
const SitePropsFile = "props.json"

const (
	sitePropsBoardsMax = 200
	sitePropsValuesMax = 60
	sitePropsNameMax   = 80
	sitePropsTextMax   = 2000
)

// SitePropValue is a setting's value: a word, a number or a switch.
type SitePropValue = any

// SiteProps is meta/props.json: by board path, the settings picked (name → value), and who picked last and when.
type SiteProps struct {
	Boards map[string]map[string]SitePropValue `json:"boards"`
	By     string                              `json:"by,omitempty"`
	At     string                              `json:"at,omitempty"`
}

func (l *Library) sitePropsPath(id string) string { return filepath.Join(l.MetaDir(id), SitePropsFile) }

// ReadSiteProps is the project's shared settings; empty when none were picked.
func (l *Library) ReadSiteProps(id string) SiteProps {
	out := SiteProps{Boards: map[string]map[string]SitePropValue{}}
	raw, err := readFileLimited(l.sitePropsPath(id), 256*1024)
	if err != nil {
		return out
	}
	var v SiteProps
	if json.Unmarshal(raw, &v) != nil {
		return out
	}
	for board, values := range v.Boards {
		if _, ok := cleanBoardPath(board); !ok {
			continue
		}
		clean := cleanPropValues(values)
		if len(clean) > 0 {
			out.Boards[board] = clean
		}
	}
	out.By, out.At = v.By, v.At
	return out
}

func cleanPropValue(v any) (any, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	case string:
		return x, len(x) <= sitePropsTextMax
	}
	return nil, false
}

func cleanPropValues(values map[string]SitePropValue) map[string]SitePropValue {
	out := map[string]SitePropValue{}
	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if k == "" || len(k) > sitePropsNameMax || len(out) == sitePropsValuesMax {
			continue
		}
		if v, ok := cleanPropValue(values[k]); ok {
			out[k] = v
		}
	}
	return out
}

// SetSiteProps puts a board's values (all of them: a name left out goes back to the board's own); merge keeps the
// names not given. Empty values clear the board.
func (l *Library) SetSiteProps(id, board string, values map[string]SitePropValue, merge bool, by string) (SiteProps, error) {
	board, ok := cleanBoardPath(board)
	if !ok {
		return SiteProps{}, deckErr(http400, "board is the board's path", "board — путь доски")
	}
	cur := l.ReadSiteProps(id)
	next := cleanPropValues(values)
	if merge {
		for k, v := range cur.Boards[board] {
			if _, has := values[k]; !has {
				next[k] = v
			}
		}
	}
	if len(next) == 0 {
		delete(cur.Boards, board)
	} else {
		if _, had := cur.Boards[board]; !had && len(cur.Boards) >= sitePropsBoardsMax {
			return SiteProps{}, deckErr(http409, "Too many boards with settings", "Слишком много досок с настройками")
		}
		cur.Boards[board] = next
	}
	cur.By, cur.At = by, l.clock().UTC().Format(time.RFC3339)
	if err := l.ensureMetaDirs(id); err != nil {
		return SiteProps{}, err
	}
	if len(cur.Boards) == 0 {
		if err := os.Remove(l.sitePropsPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return SiteProps{}, err
		}
		return cur, nil
	}
	if _, err := writeIfChanged(l.sitePropsPath(id), deckJSON(cur)); err != nil {
		return SiteProps{}, err
	}
	return cur, nil
}

func propValuesOf(v any) (map[string]SitePropValue, bool) {
	o, ok := v.(*jsvalue.Object)
	if !ok {
		return nil, false
	}
	out := map[string]SitePropValue{}
	for _, k := range o.Keys() {
		out[k], _ = o.Get(k)
	}
	return out, true
}

// SiteProps is GET ?artifact= (view): the SiteProps; PUT {artifact, board, values} (comment): the board's values put
// whole ({} goes back to the board's own), the SiteProps after.
func (a *API) SiteProps(r *http.Request) Answer {
	min := RoleComment
	if r.Method == http.MethodGet {
		min = RoleView
	}
	body, id, no := a.deckGuarded(r, min, http.MethodGet, http.MethodPut)
	if no != nil {
		return *no
	}
	if r.Method == http.MethodGet {
		return JSON(r, http.StatusOK, a.L.ReadSiteProps(id), noStore())
	}
	board, _ := strField(body, "board")
	values, ok := propValuesOf(Field(body, "values"))
	if board == "" || !ok {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("board and values are expected", "Нужны board и values")), noStore())
	}
	me, err := a.commenter(r)
	if err != nil {
		return a.deckFail(r, err, "know who changes the settings")
	}
	v, err := a.L.SetSiteProps(id, board, values, false, me.Name)
	if err != nil {
		return a.deckFail(r, err, "save the settings")
	}
	return JSON(r, http.StatusOK, v, noStore())
}
