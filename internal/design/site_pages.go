package design

import (
	"net/http"
	"strings"

	"github.com/ly-solutions/harnessly/internal/locale"
)

// SitePages is the people's side of the page commands («Макет» → «Страницы»): POST {artifact, action, page?,
// title?} with action create (title), duplicate (page, title?: a variant of it, as `design page create --variant-of`),
// rename (page, title) or remove (page). A page is named by its file, id or title, as in the agents' commands
// (CLIPages); the answer is {ok, page}. Whoever may edit the project may do it; the machine does it, not the hub (a
// removed page goes to the library's trash).
func (a *API) SitePages(r *http.Request) Answer {
	body, id, no := a.deckGuarded(r, RoleEdit, http.MethodPost)
	if no != nil {
		return *no
	}
	h := noStore()
	page, _ := strField(body, "page")
	title, _ := strField(body, "title")
	title = strings.TrimSpace(title)
	answer := func(p Page, err error) Answer {
		if err != nil {
			if pe, ok := err.(*PageError); ok {
				return JSON(r, pe.Status, errorBody(pe.Msg), h)
			}
			a.warn("design site pages", err)
			return JSON(r, http.StatusInternalServerError, errorBody(locale.Pick("Could not change the pages", "Не удалось изменить страницы")), h)
		}
		return JSON(r, http.StatusOK, struct {
			OK   bool `json:"ok"`
			Page Page `json:"page"`
		}{true, p}, h)
	}
	need := func(what ...string) *Answer {
		for _, v := range what {
			if v == "" {
				ans := JSON(r, http.StatusBadRequest, errorBody(locale.Pick("page and title are expected", "Нужны page и title")), h)
				return &ans
			}
		}
		return nil
	}
	switch Field(body, "action") {
	case "create":
		if no := need(title); no != nil {
			return *no
		}
		return answer(a.L.CreatePage(id, PageSpec{Title: title}))
	case "duplicate":
		if no := need(page); no != nil {
			return *no
		}
		if title == "" {
			m, err := a.L.requireModel(id)
			if err != nil {
				return answer(Page{}, err)
			}
			base, err := ResolvePage(m, page)
			if err != nil {
				return answer(Page{}, err)
			}
			title = base.Title + locale.Pick(" (variant)", " (вариант)")
		}
		return answer(a.L.CreatePage(id, PageSpec{Title: title, VariantOf: page}))
	case "rename":
		if no := need(page, title); no != nil {
			return *no
		}
		return answer(a.L.RenamePage(id, page, &title, nil))
	case "remove":
		if no := need(page); no != nil {
			return *no
		}
		return answer(a.L.RemovePage(id, page))
	}
	return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("Unknown action: create, duplicate, rename or remove", "Неизвестное действие: create, duplicate, rename или remove")), h)
}
