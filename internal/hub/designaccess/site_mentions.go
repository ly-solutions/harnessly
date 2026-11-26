package designaccess

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// A person called by @ in a comment on a site board hears of it: the hub saved the comment
// (site/comments, an edit route), so it rings each person the comment names who has the project open to them — never
// the author, never anyone the project is closed to — with a link that opens the board on the thread.

const siteMentionsMax = 10

// answerRecorder keeps what an edit answered, so the hub can read the saved comment before passing it on.
type answerRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (a *answerRecorder) WriteHeader(code int) { a.status = code }

func (a *answerRecorder) Write(b []byte) (int, error) { return a.body.Write(b) }

func (a *answerRecorder) flush() {
	if a.status == 0 {
		a.status = http.StatusOK
	}
	a.ResponseWriter.WriteHeader(a.status)
	_, _ = a.ResponseWriter.Write(a.body.Bytes())
}

type siteMentioned struct {
	ID       string `json:"id"`
	Board    string `json:"board"`
	Text     string `json:"text"`
	Parent   string `json:"parent"`
	By       struct{ Name, Email string }
	Mentions []struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"mentions"`
}

func (s *Service) ringSiteMentions(t *txn, p project, m store.Machine, projectID, author string, rec *answerRecorder) {
	if rec.status != http.StatusOK || (s.bell == nil && s.notice == nil) {
		return
	}
	var c siteMentioned
	if json.Unmarshal(rec.body.Bytes(), &c) != nil || c.ID == "" || len(c.Mentions) == 0 {
		return
	}
	text := strings.TrimSpace(c.Text)
	if utf8.RuneCountInString(text) > 140 {
		text = string([]rune(text)[:139]) + "…"
	}
	thread := c.ID
	if c.Parent != "" {
		thread = c.Parent
	}
	link := s.hubURL + "/m/" + m.ID + "/designly/" + url.PathEscape(projectID) + "?comment=" + url.QueryEscape(thread)
	who := c.By.Name
	if who == "" {
		who = "Кто-то"
	}
	seen := map[string]bool{author: true}
	for i, mt := range c.Mentions {
		if i == siteMentionsMax {
			break
		}
		email := normEmail(mt.Email)
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		if f, err := t.fold(p, email); err != nil || f.Role == "" {
			continue
		}
		s.send(m.ID, email, "mention:"+projectID+":"+c.ID, "Вас упомянули в комментарии к макету", who+": "+text, link)
	}
}
