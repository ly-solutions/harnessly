package design

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ly-solutions/harnessly/internal/locale"
)

// The agent's side of a site's comments and shared settings: a comment that calls @harnessly is put into the
// chat the author has the board open beside (SiteCommentAgent, the author's own machine asks it); the agent reads the
// threads, answers in them and resolves them, and reads and sets the board's shared settings (`harnessly design
// comments`, `harnessly design props`).

// siteAgentText is what the agent is told of a comment that calls it.
func siteAgentText(project, title string, c, root DeckComment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Комментарий к макету «%s» (проект %s), доска %s", title, project, root.Board)
	if root.Label != "" || root.Block != "" {
		fmt.Fprintf(&b, ", блок «%s»", firstNonEmptyText(root.Label, root.Block))
	}
	fmt.Fprintf(&b, ":\n\n%s\n\n", c.Text)
	fmt.Fprintf(&b, "Обсуждение %s. Ответь в нём: harnessly design comments reply %s %s \"…\"; когда сделано — harnessly design comments resolve %s %s.",
		root.ID, project, root.ID, project, root.ID)
	return b.String()
}

func firstNonEmptyText(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// SiteCommentAgent is POST {artifact, id, chat} (comment): the comment id, which calls @harnessly, goes to the agent of
// the chat. The person's own machine is asked, from the page the chat shows the board in: {sent: true}.
func (a *API) SiteCommentAgent(r *http.Request) Answer {
	body, id, no := a.deckGuarded(r, RoleComment, http.MethodPost)
	if no != nil {
		return *no
	}
	cid, _ := strField(body, "id")
	chat, _ := strField(body, "chat")
	if cid == "" || chat == "" {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("id and chat are expected", "Нужны id и chat")), noStore())
	}
	if a.TellChat == nil {
		return JSON(r, http.StatusServiceUnavailable, errorBody(locale.Pick("No agent here", "Агента здесь нет")), noStore())
	}
	c, err := a.L.readComment(id, cid)
	if err != nil || c.View != DeckCommentSite || !c.Agent {
		return JSON(r, http.StatusNotFound, errorBody(locale.Pick("No comment that calls the agent", "Нет комментария, который зовёт агента")), noStore())
	}
	root := c
	if c.Parent != "" {
		if root, err = a.L.readComment(id, c.Parent); err != nil {
			return a.deckFail(r, err, "read the thread")
		}
	}
	title := id
	if m := a.L.ReadArtifact(id); m != nil && m.Title != "" {
		title = m.Title
	}
	if err := a.TellChat(r.Context(), chat, c.By.Name, "Комментарий к макету «"+title+"»", siteAgentText(id, title, c, root)); err != nil {
		return JSON(r, http.StatusConflict, errorBody(err.Error()), noStore())
	}
	return JSON(r, http.StatusOK, struct {
		Sent bool `json:"sent"`
	}{true}, noStore())
}

// CLISiteComments is the agent's: GET ?project=&board=&open=1 (view): {project, comments}; POST {project, id, text}
// (comment): the agent's reply in the thread id; PATCH {project, id, resolved} (comment): the thread resolved or open.
func (a *API) CLISiteComments(r *http.Request) Answer {
	raw, no := a.CLIGuard(r)
	if no != nil {
		return *no
	}
	body := Fields(raw)
	project := QueryString(r, "project")
	if r.Method != http.MethodGet {
		project, _ = strField(body, "project")
	}
	if ref := a.L.RefOf(project); ref != "" {
		project = ref
	}
	min := RoleComment
	if r.Method == http.MethodGet {
		min = RoleView
	}
	if no := a.gate(r, project, min); no != nil {
		return *no
	}
	if !IsLibraryID(project) || a.L.ReadArtifact(project) == nil {
		return JSON(r, http.StatusNotFound, errorBody(locale.Pick("No such design", "Нет такого макета")), noStore())
	}
	agent := DeckPerson{Name: SiteAgentHandle}
	switch r.Method {
	case http.MethodGet:
		all := a.L.ReadSiteComments(project, QueryString(r, "board"), agent)
		open := map[string]bool{}
		for _, c := range all {
			if c.Parent == "" && !c.Resolved {
				open[c.ID] = true
			}
		}
		out := []SiteComment{}
		for _, c := range all {
			root := c.ID
			if c.Parent != "" {
				root = c.Parent
			}
			if QueryString(r, "open") != "1" || open[root] {
				out = append(out, c)
			}
		}
		return JSON(r, http.StatusOK, struct {
			Project  string        `json:"project"`
			Comments []SiteComment `json:"comments"`
		}{project, out}, noStore())
	case http.MethodPost:
		cid, _ := strField(body, "id")
		text, _ := strField(body, "text")
		c, err := a.L.AddSiteComment(project, agent, SiteCommentSpec{Parent: cid, Text: text, ByAgent: true})
		if err != nil {
			return a.deckFail(r, err, "answer in the thread")
		}
		return JSON(r, http.StatusOK, a.L.siteView(c, agent), noStore())
	case http.MethodPatch:
		cid, _ := strField(body, "id")
		resolved, ok := Field(body, "resolved").(bool)
		if cid == "" || !ok {
			return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("id and resolved are expected", "Нужны id и resolved")), noStore())
		}
		c, err := a.L.ChangeSiteComment(project, cid, agent, &resolved, "")
		if err != nil {
			return a.deckFail(r, err, "change the thread")
		}
		return JSON(r, http.StatusOK, a.L.siteView(c, agent), noStore())
	}
	h := noStore()
	h.Set("Allow", "GET, POST, PATCH")
	return Empty(http.StatusMethodNotAllowed, h)
}

// CLISiteProps is the agent's: GET ?project= (view): {project, props: SiteProps}; POST {project, board, values} (edit):
// the values put over the board's others (a null value goes back to the board's own), {project, props} after.
func (a *API) CLISiteProps(r *http.Request) Answer {
	raw, no := a.CLIGuard(r)
	if no != nil {
		return *no
	}
	body := Fields(raw)
	project := QueryString(r, "project")
	if r.Method != http.MethodGet {
		project, _ = strField(body, "project")
	}
	if ref := a.L.RefOf(project); ref != "" {
		project = ref
	}
	min := RoleEdit
	if r.Method == http.MethodGet {
		min = RoleView
	}
	if no := a.gate(r, project, min); no != nil {
		return *no
	}
	if !IsLibraryID(project) || a.L.ReadArtifact(project) == nil {
		return JSON(r, http.StatusNotFound, errorBody(locale.Pick("No such design", "Нет такого макета")), noStore())
	}
	type answer struct {
		Project string    `json:"project"`
		Props   SiteProps `json:"props"`
	}
	if r.Method == http.MethodGet {
		return JSON(r, http.StatusOK, answer{project, a.L.ReadSiteProps(project)}, noStore())
	}
	if r.Method != http.MethodPost {
		h := noStore()
		h.Set("Allow", "GET, POST")
		return Empty(http.StatusMethodNotAllowed, h)
	}
	board, _ := strField(body, "board")
	values, ok := propValuesOf(Field(body, "values"))
	if board == "" || !ok {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("board and values are expected", "Нужны board и values")), noStore())
	}
	// A name given null goes back to the board's own: it is dropped from the merge.
	cur := a.L.ReadSiteProps(project).Boards[board]
	next := map[string]SitePropValue{}
	for k, v := range cur {
		next[k] = v
	}
	for k, v := range values {
		if v == nil {
			delete(next, k)
		} else {
			next[k] = v
		}
	}
	v, err := a.L.SetSiteProps(project, board, next, false, SiteAgentHandle)
	if err != nil {
		return a.deckFail(r, err, "save the settings")
	}
	return JSON(r, http.StatusOK, answer{project, v}, noStore())
}
