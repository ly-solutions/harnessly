package design

import (
	"encoding/json"
	"math"
	"net/http"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
	"github.com/ly-solutions/harnessly/internal/locale"
)

// A video's model routes: the state, the settings, a scene's duration and transition, and comments on a moment or a
// stretch. The files they keep are in video.go and video_scenes.go.

// videoGuarded is deckGuarded for a project that must be a video.
func (a *API) videoGuarded(r *http.Request, min Role, methods ...string) (*jsvalue.Object, string, *Answer) {
	body, id, no := a.deckGuarded(r, min, methods...)
	if no != nil {
		return nil, "", no
	}
	if !a.L.IsVideo(id) {
		ans := JSON(r, http.StatusNotFound, errorBody(locale.Pick("Not a video", "Это не видео")), noStore())
		return nil, "", &ans
	}
	return body, id, nil
}

// bodyPatch is the keys of a body as raw JSON, the named ones only.
func bodyPatch(body *jsvalue.Object, keys ...string) (map[string]json.RawMessage, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(jsvalue.Stringify(body, ""), &all); err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for _, k := range keys {
		if raw, ok := all[k]; ok {
			out[k] = raw
		}
	}
	return out, nil
}

// Video is GET ?artifact= (view): the VideoState.
func (a *API) Video(r *http.Request) Answer {
	_, id, no := a.videoGuarded(r, RoleView, http.MethodGet)
	if no != nil {
		return *no
	}
	st, err := a.L.VideoStateOf(id)
	if err != nil {
		return a.deckFail(r, err, "read the video")
	}
	return JSON(r, http.StatusOK, st, noStore())
}

// VideoSettings is PUT {artifact, settings: {formats?, subs?, seconds?}} (edit): {settings: VideoSettings}.
func (a *API) VideoSettings(r *http.Request) Answer {
	body, id, no := a.videoGuarded(r, RoleEdit, http.MethodPut)
	if no != nil {
		return *no
	}
	obj, ok := Field(body, "settings").(*jsvalue.Object)
	if !ok {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("settings is an object", "settings — объект")), noStore())
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(jsvalue.Stringify(obj, ""), &patch); err != nil {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("settings is not valid", "settings неверны")), noStore())
	}
	s, err := a.L.SetVideoSettings(id, patch)
	if err != nil {
		return a.deckFail(r, err, "save the video's settings")
	}
	return JSON(r, http.StatusOK, struct {
		Settings VideoSettings `json:"settings"`
	}{s}, noStore())
}

// VideoScene is PUT {artifact, page, dur: ms|null, transition: {kind, ms}|null} (edit): {scene: VideoScene}. A key left
// out stays as it is.
func (a *API) VideoScene(r *http.Request) Answer {
	body, id, no := a.videoGuarded(r, RoleEdit, http.MethodPut)
	if no != nil {
		return *no
	}
	page, _ := strField(body, "page")
	if page == "" {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("page is expected", "Нужна page")), noStore())
	}
	patch, err := bodyPatch(body, "dur", "transition")
	if err != nil {
		return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("The body is not valid", "Тело запроса неверно")), noStore())
	}
	sc, err := a.L.SetVideoScene(id, page, patch)
	if err != nil {
		return a.deckFail(r, err, "save the scene")
	}
	return JSON(r, http.StatusOK, struct {
		Scene VideoScene `json:"scene"`
	}{sc}, noStore())
}

// ms reads a body number that must be a whole count of ms.
func whole(v any) (int64, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || math.Abs(f) > 1e12 {
		return 0, false
	}
	return int64(f), true
}

// VideoComments is POST {artifact, page, t0, t1|null, format, x|null, y|null, block, label, text} (comment): the
// DeckComment with view "video"; and PATCH as DeckComments' (comment).
func (a *API) VideoComments(r *http.Request) Answer {
	body, id, no := a.videoGuarded(r, RoleComment, http.MethodPost, http.MethodPatch)
	if no != nil {
		return *no
	}
	if r.Method == http.MethodPatch {
		cid, _ := strField(body, "id")
		resolved, isBool := Field(body, "resolved").(bool)
		if cid == "" || !isBool {
			return JSON(r, http.StatusBadRequest, errorBody(locale.Pick("id and resolved are expected", "Нужны id и resolved")), noStore())
		}
		v, err := a.L.ResolveDeckComment(id, cid, resolved)
		if err != nil {
			return a.deckFail(r, err, "change the comment")
		}
		return JSON(r, http.StatusOK, v, noStore())
	}
	spec, msg := videoCommentSpec(body)
	if msg != "" {
		return JSON(r, http.StatusBadRequest, errorBody(msg), noStore())
	}
	c, err := a.L.AddDeckComment(id, spec)
	if err != nil {
		return a.deckFail(r, err, "save the comment")
	}
	return JSON(r, http.StatusOK, c, noStore())
}

// videoCommentSpec reads a POST body; msg is the refusal, "" when the body is good. x and y null: the comment has no
// point on the frame.
func videoCommentSpec(body *jsvalue.Object) (DeckCommentSpec, string) {
	page, _ := strField(body, "page")
	text, _ := strField(body, "text")
	label, _ := strField(body, "label")
	block, _ := strField(body, "block")
	format, _ := strField(body, "format")
	t0, ok0 := whole(Field(body, "t0"))
	if page == "" || !ok0 {
		return DeckCommentSpec{}, locale.Pick("page, t0 and text are expected", "Нужны page, t0 и text")
	}
	spec := DeckCommentSpec{Page: page, Label: label, Text: text, T0: &t0, Format: VideoFormat(format), Block: block}
	if Field(body, "t1") != nil {
		t1, ok := whole(Field(body, "t1"))
		if !ok {
			return DeckCommentSpec{}, locale.Pick("t1 is ms or null", "t1 — миллисекунды или null")
		}
		spec.T1 = &t1
	}
	for _, p := range []struct {
		k string
		v *float64
	}{{"x", &spec.X}, {"y", &spec.Y}} {
		if Field(body, p.k) == nil {
			continue
		}
		f, ok := Field(body, p.k).(float64)
		if !ok || math.IsNaN(f) {
			return DeckCommentSpec{}, locale.Pick("x and y are percents or null", "x и y — проценты или null")
		}
		*p.v = f
	}
	return spec, ""
}
