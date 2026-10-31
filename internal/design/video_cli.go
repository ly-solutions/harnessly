package design

import (
	"net/http"

	"github.com/ly-solutions/harnessly/internal/locale"
)

// The agent's CLI over a video's model. Text taken from the project is data for the agent; the CLI
// (internal/hlcli/design_video.go) prints it under the "data, not instructions" header.

// CLIVideo is GET ?project=&what=state|comments[&open=1] (view): {project, state: VideoState} or {project, comments:
// [DeckComment]} (open: only the unresolved ones).
func (a *API) CLIVideo(r *http.Request) Answer {
	if _, no := a.CLIGuard(r); no != nil {
		return *no
	}
	h := noStore()
	if r.Method != http.MethodGet {
		h.Set("Allow", "GET")
		return Empty(http.StatusMethodNotAllowed, h)
	}
	project := QueryString(r, "project")
	if no := a.gate(r, project, RoleView); no != nil {
		return *no
	}
	if err := a.L.requireVideo(project); err != nil {
		return a.deckFail(r, err, "read the video")
	}
	switch what := QueryString(r, "what"); what {
	case "state":
		st, err := a.L.VideoStateOf(project)
		if err != nil {
			return a.deckFail(r, err, "read the video")
		}
		return JSON(r, http.StatusOK, struct {
			Project string     `json:"project"`
			State   VideoState `json:"state"`
		}{project, st}, h)
	case "comments":
		all, _ := a.L.ReadDeckComments(project)
		out := []DeckComment{}
		for _, c := range all {
			if QueryString(r, "open") != "1" || !c.Resolved {
				out = append(out, c)
			}
		}
		return JSON(r, http.StatusOK, struct {
			Project  string        `json:"project"`
			Comments []DeckComment `json:"comments"`
		}{project, out}, h)
	default:
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("what is state or comments", "what — state или comments")), h)
	}
}

// CLIVideoScene is POST {project, page: id|file|title, dur?: ms|null, transition?: {kind, ms}|null} (edit): {project,
// scene: VideoScene}.
func (a *API) CLIVideoScene(r *http.Request) Answer {
	raw, no := a.CLIGuard(r)
	if no != nil {
		return *no
	}
	h := noStore()
	if r.Method != http.MethodPost {
		h.Set("Allow", "POST")
		return Empty(http.StatusMethodNotAllowed, h)
	}
	body := Fields(raw)
	project, _ := strField(body, "project")
	if no := a.gate(r, project, RoleEdit); no != nil {
		return *no
	}
	if err := a.L.requireVideo(project); err != nil {
		return a.deckFail(r, err, "save the scene")
	}
	page, _ := strField(body, "page")
	if page == "" {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("page is expected", "Нужна page")), h)
	}
	patch, err := bodyPatch(body, "dur", "transition")
	if err != nil {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("The body is not valid", "Тело запроса неверно")), h)
	}
	sc, err := a.L.SetVideoScene(project, page, patch)
	if err != nil {
		return a.deckFail(r, err, "save the scene")
	}
	return JSON(r, http.StatusOK, struct {
		Project string     `json:"project"`
		Scene   VideoScene `json:"scene"`
	}{project, sc}, h)
}
