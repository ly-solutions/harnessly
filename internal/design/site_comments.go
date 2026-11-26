package design

import (
	"encoding/json"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
	"github.com/ly-solutions/harnessly/internal/locale"
)

// Comments of the people on a site's boards: a pin at a point of a board or at one of its blocks, a thread of
// replies under it, reactions, «Решено», @people (the hub rings them, designaccess) and @harnessly (the comment goes to
// the agent of the chat the author has the board open beside: SiteCommentAgent). They are kept as the deck's are, in
// meta/comments/<ULID>.json, with View "site", so the hub saves them as it saves a deck's (designview.EditRoutes) and a
// project's history carries them.

// DeckCommentSite is the View of a site board's comment.
const DeckCommentSite = "site"

const (
	siteBoardMax    = 300
	siteMentionsMax = 10
	// siteYMax bounds a site comment's Y: the CSS pixels from the page's top.
	siteYMax = 1e6
	// SiteAgentHandle is the @ that calls the agent.
	SiteAgentHandle = "harnessly"
)

// SiteReactions is the emojis a comment may get: a fixed set, so a reaction is never free text.
var SiteReactions = []string{"👍", "❤️", "🎉", "👀", "✅", "🤔"}

// SiteCommentSpec is a new site comment or reply.
type SiteCommentSpec struct {
	Board    string
	X, Y     float64
	Block    string
	Label    string
	Text     string
	Parent   string
	Mentions []DeckPerson
	Agent    bool
	ByAgent  bool
	// Drawing goes with a thread's first comment only; with it, the words may be left out.
	Drawing *SiteDrawing
}

// SiteReaction is one emoji of a comment as a person sees it.
type SiteReaction struct {
	Emoji string   `json:"e"`
	Count int      `json:"n"`
	Mine  bool     `json:"mine"`
	Names []string `json:"names"`
}

// SiteComment is a site comment as answered: the stored one, its reactions told as counts and whether they are the
// asker's (the people's addresses stay in the file).
type SiteComment struct {
	DeckComment
	Reactions []SiteReaction `json:"reactions"`
}

func cleanBoardPath(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > siteBoardMax || strings.Contains(s, "..") || strings.HasPrefix(s, "/") || strings.ContainsAny(s, "\x00\r\n") {
		return "", false
	}
	return s, true
}

// reactorKey is who a reaction is kept under: the address, else the name.
func reactorKey(p DeckPerson) string {
	if p.Email != "" {
		return strings.ToLower(p.Email)
	}
	return p.Name
}

// ReadSiteComments are the site comments of a board ("" all boards), oldest first, as `me` sees them.
func (l *Library) ReadSiteComments(id, board string, me DeckPerson) []SiteComment {
	all, _ := l.ReadDeckComments(id)
	out := []SiteComment{}
	for _, c := range all {
		if c.View != DeckCommentSite || (board != "" && c.Board != board) {
			continue
		}
		out = append(out, l.siteView(c, me))
	}
	return out
}

func (l *Library) siteView(c DeckComment, me DeckPerson) SiteComment {
	key := reactorKey(me)
	reacts := []SiteReaction{}
	for _, e := range SiteReactions {
		who := c.Reactions[e]
		if len(who) == 0 {
			continue
		}
		r := SiteReaction{Emoji: e, Count: len(who), Names: []string{}}
		for _, p := range who {
			if p == key {
				r.Mine = true
			}
			if id, err := l.IdentityOf(p); err == nil && strings.Contains(p, "@") {
				r.Names = append(r.Names, id.Person)
			} else {
				r.Names = append(r.Names, p)
			}
		}
		reacts = append(reacts, r)
	}
	c.Reactions = nil
	return SiteComment{DeckComment: c, Reactions: reacts}
}

func (l *Library) readComment(id, commentID string) (DeckComment, error) {
	if !deckCommentIDRe.MatchString(commentID) {
		return DeckComment{}, deckErr(http400, "Not a comment id", "Это не номер комментария")
	}
	raw, err := readFileLimited(filepath.Join(l.commentsDir(id), commentID+".json"), 64*1024)
	if err != nil {
		return DeckComment{}, deckErr(http404, "No such comment", "Нет такого комментария")
	}
	var c DeckComment
	if json.Unmarshal(raw, &c) != nil || c.ID != commentID {
		return DeckComment{}, deckErr(http404, "No such comment", "Нет такого комментария")
	}
	return c, nil
}

// AddSiteComment pins a comment to a board, or adds a reply to a thread (Parent), in the name of `by`.
func (l *Library) AddSiteComment(id string, by DeckPerson, spec SiteCommentSpec) (DeckComment, error) {
	board, ok := cleanBoardPath(spec.Board)
	if !ok && spec.Parent == "" {
		return DeckComment{}, deckErr(http400, "board is the board's path", "board — путь доски")
	}
	text := cleanCommentText(spec.Text)
	drawn := spec.Drawing != nil && spec.Parent == ""
	if (text == "" && !drawn) || utf8.RuneCountInString(text) > maxDeckCommentRune {
		return DeckComment{}, deckErr(http400, "The comment is empty or over 2000 characters", "Комментарий пуст или длиннее 2000 знаков")
	}
	existing, _ := l.ReadDeckComments(id)
	if len(existing) >= maxDeckComments {
		return DeckComment{}, deckErr(http409, "Too many comments on the project", "Слишком много комментариев к проекту")
	}
	c := DeckComment{ID: NewPageID(), View: DeckCommentSite, Board: board, Text: text, By: by,
		At: l.clock().UTC().Format(time.RFC3339), Agent: spec.Agent, ByAgent: spec.ByAgent}
	if drawn {
		c.Drawing = spec.Drawing
	}
	if spec.Parent != "" {
		parent, err := l.readComment(id, spec.Parent)
		if err != nil || parent.View != DeckCommentSite || parent.Parent != "" {
			return DeckComment{}, deckErr(http404, "No such thread", "Нет такого обсуждения")
		}
		c.Parent, c.Board = parent.ID, parent.Board
	} else {
		bad := func(f, max float64) bool { return math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > max }
		if bad(spec.X, 100) || bad(spec.Y, siteYMax) {
			return DeckComment{}, deckErr(http400, "x is a percent of the page's width, y its CSS pixels from the top", "x — процент ширины страницы, y — пиксели от её верха")
		}
		c.X, c.Y = math.Round(spec.X*100)/100, math.Round(spec.Y*100)/100
		c.Block, _ = CleanText(spec.Block, 120)
		c.Label, _ = CleanText(spec.Label, 80)
	}
	for _, m := range spec.Mentions {
		if len(c.Mentions) == siteMentionsMax {
			break
		}
		email := strings.ToLower(strings.TrimSpace(m.Email))
		name, _ := CleanText(m.Name, 64)
		if (email == "" || !strings.Contains(email, "@") || len(email) > 128) || strings.EqualFold(email, by.Email) {
			continue
		}
		dup := false
		for _, have := range c.Mentions {
			dup = dup || have.Email == email
		}
		if !dup {
			c.Mentions = append(c.Mentions, DeckPerson{Name: name, Email: email})
		}
	}
	if err := l.writeCommentFile(id, c.ID, c); err != nil {
		return DeckComment{}, err
	}
	return c, nil
}

// ChangeSiteComment resolves or opens a thread (resolved non-nil), or turns `by`'s reaction on or off (emoji).
func (l *Library) ChangeSiteComment(id, commentID string, by DeckPerson, resolved *bool, emoji string) (DeckComment, error) {
	c, err := l.readComment(id, commentID)
	if err != nil || c.View != DeckCommentSite {
		return DeckComment{}, deckErr(http404, "No such comment", "Нет такого комментария")
	}
	if resolved != nil {
		if c.Parent != "" {
			return DeckComment{}, deckErr(http400, "Only a thread is resolved", "Решённым отмечается обсуждение целиком")
		}
		at := l.clock().UTC().Format(time.RFC3339)
		c.Resolved = *resolved
		if *resolved {
			c.ResolvedBy, c.ResolvedAt = &by.Name, &at
		} else {
			c.ResolvedBy, c.ResolvedAt = nil, nil
		}
	}
	if emoji != "" {
		known := false
		for _, e := range SiteReactions {
			known = known || e == emoji
		}
		if !known {
			return DeckComment{}, deckErr(http400, "Not a reaction", "Такой реакции нет")
		}
		key := reactorKey(by)
		who := c.Reactions[emoji]
		at := -1
		for i, p := range who {
			if p == key {
				at = i
			}
		}
		if at >= 0 {
			who = append(who[:at], who[at+1:]...)
		} else {
			who = append(who, key)
		}
		if c.Reactions == nil {
			c.Reactions = map[string][]string{}
		}
		if len(who) == 0 {
			delete(c.Reactions, emoji)
		} else {
			sort.Strings(who)
			c.Reactions[emoji] = who
		}
		if len(c.Reactions) == 0 {
			c.Reactions = nil
		}
	}
	if err := l.writeCommentFile(id, c.ID, c); err != nil {
		return DeckComment{}, err
	}
	return c, nil
}

// commenter is who makes the request, as a comment names them.
func (a *API) commenter(r *http.Request) (DeckPerson, error) {
	who, err := a.L.IdentityOf(a.PersonOf(r))
	if err != nil {
		return DeckPerson{}, err
	}
	return DeckPerson{Name: who.Person, Email: who.PersonID}, nil
}

func mentionsOf(v any) []DeckPerson {
	arr, _ := v.([]any)
	out := []DeckPerson{}
	for _, it := range arr {
		o, ok := it.(*jsvalue.Object)
		if !ok {
			continue
		}
		email, _ := strField(o, "email")
		name, _ := strField(o, "name")
		out = append(out, DeckPerson{Name: name, Email: email})
		if len(out) == siteMentionsMax {
			break
		}
	}
	return out
}

// SiteComments is GET ?artifact=&board= (view): {comments}; POST {artifact, board, x, y, block, label, text, parent,
// mentions: [{name, email}], agent} (comment): the SiteComment; PATCH {artifact, id, resolved | react} (comment): the
// SiteComment.
func (a *API) SiteComments(r *http.Request) Answer {
	min := RoleComment
	if r.Method == http.MethodGet {
		min = RoleView
	}
	body, id, no := a.deckGuarded(r, min, http.MethodGet, http.MethodPost, http.MethodPatch)
	if no != nil {
		return *no
	}
	me, err := a.commenter(r)
	if err != nil {
		return a.deckFail(r, err, "know who comments")
	}
	switch r.Method {
	case http.MethodGet:
		board := QueryString(r, "board")
		return JSON(r, http.StatusOK, struct {
			Comments []SiteComment `json:"comments"`
		}{a.L.ReadSiteComments(id, board, me)}, noStore())
	case http.MethodPatch:
		cid, _ := strField(body, "id")
		react, _ := strField(body, "react")
		var resolved *bool
		if v, ok := Field(body, "resolved").(bool); ok {
			resolved = &v
		}
		if cid == "" || (resolved == nil && react == "") {
			return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("id and resolved or react are expected", "Нужны id и resolved или react")), noStore())
		}
		c, err := a.L.ChangeSiteComment(id, cid, me, resolved, react)
		if err != nil {
			return a.deckFail(r, err, "change the comment")
		}
		return JSON(r, http.StatusOK, a.L.siteView(c, me), noStore())
	}
	spec := SiteCommentSpec{Mentions: mentionsOf(Field(body, "mentions"))}
	spec.Board, _ = strField(body, "board")
	spec.Text, _ = strField(body, "text")
	spec.Block, _ = strField(body, "block")
	spec.Label, _ = strField(body, "label")
	spec.Parent, _ = strField(body, "parent")
	spec.Agent, _ = Field(body, "agent").(bool)
	drawing, ok := siteDrawingOf(Field(body, "drawing"))
	if !ok {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("The drawing is not one or is too large", "Рисунок не читается или слишком большой")), noStore())
	}
	spec.Drawing = drawing
	if spec.Parent == "" {
		x, okX := Field(body, "x").(float64)
		y, okY := Field(body, "y").(float64)
		if !okX || !okY {
			return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("board, x, y and text are expected", "Нужны board, x, y и text")), noStore())
		}
		spec.X, spec.Y = x, y
	}
	c, err := a.L.AddSiteComment(id, me, spec)
	if err != nil {
		return a.deckFail(r, err, "save the comment")
	}
	return JSON(r, http.StatusOK, a.L.siteView(c, me), noStore())
}
