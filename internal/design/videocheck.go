package design

import (
	"context"
	"crypto/sha256"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/ly-solutions/harnessly/internal/jscompat"
	"github.com/ly-solutions/harnessly/internal/locale"
)

// A video's check: the static lint of the boards' scripts, moments past a scene's end, and in Chrome at each scene's
// end per enabled format text overflow, blocks off screen, small text in 9:16 and frames that are not deterministic.

const (
	// videoMinFont is the smallest text in a 9:16 frame 1080 px wide.
	videoMinFont = 28
	// videoCheckBeforeEnd is how far before a scene's end the check looks (one frame at 30 fps).
	videoCheckBeforeEnd = 1000 / VideoFPS
)

var (
	videoScriptRe = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
	videoSrcRe    = regexp.MustCompile(`(?i)\ssrc\s*=`)
	videoLints    = []struct {
		re    *regexp.Regexp
		code  string
		level VideoCheckLevel
		en    string
		ru    string
	}{
		{regexp.MustCompile(`\bset(?:Timeout|Interval)\s*\(`), VideoCheckTimer, VideoCheckError,
			"setTimeout / setInterval run on the wall clock: a frame of the video will not match its time. Drive motion by data-tk-at, @keyframes or requestAnimationFrame",
			"setTimeout / setInterval идут по настенным часам: кадр видео не совпадёт со своим временем. Движение — через data-tk-at, @keyframes или requestAnimationFrame"},
		{regexp.MustCompile(`\bMath\.random\s*\(`), VideoCheckRandom, VideoCheckWarn,
			"Math.random is seeded in a video: the same numbers every time, which may not be what you meant",
			"Math.random в видео с фиксированным зерном: числа каждый раз одни и те же — проверьте, что так и задумано"},
		{regexp.MustCompile(`\bDate\.now\s*\(|\bnew\s+Date\s*\(\s*\)`), VideoCheckClock, VideoCheckWarn,
			"Date.now / new Date() give the scene's time in a video, not today's date",
			"Date.now / new Date() в видео дают время сцены, а не сегодняшнюю дату"},
	}
)

// videoScenesFor is the video's scenes (video_scenes.go) without keys; a variable so the tests can give their own.
var videoScenesFor = func(l *Library, id string) ([]VideoScene, error) { return l.VideoScenes(id, VideoSettings{}) }

// VideoCheck checks the scene page (nil: every scene) in format f (nil: every enabled format).
func (l *Library) VideoCheck(ctx context.Context, id string, page *string, f *VideoFormat) ([]VideoCheckIssue, error) {
	settings, err := l.ReadVideoSettings(id)
	if err != nil {
		return nil, err
	}
	formats := settings.Formats
	if f != nil {
		if !videoHasFormat(settings, *f) {
			return nil, deckErr(http.StatusBadRequest, "The video has no format "+string(*f), "У видео нет формата "+string(*f))
		}
		formats = []VideoFormat{*f}
	}
	scenes, err := l.videoPickScenes(id, page)
	if err != nil {
		return nil, err
	}
	issues := []VideoCheckIssue{}
	for _, sc := range scenes {
		issues = append(issues, l.videoLint(id, sc)...)
		issues = append(issues, videoPastEnd(sc)...)
	}
	if l.findChrome() == "" {
		return nil, &DeckError{Status: http.StatusServiceUnavailable, Msg: errNoChrome().Error()}
	}
	s, err := l.newVideoShooter(id)
	if err != nil {
		return nil, err
	}
	defer s.close()
	tab, release, err := s.videoTab(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	for _, sc := range scenes {
		if sc.Blank {
			continue
		}
		for _, fm := range formats {
			found, err := l.videoCheckInChrome(ctx, tab, id, sc, fm)
			if err != nil {
				return nil, err
			}
			issues = append(issues, found...)
		}
	}
	return issues, nil
}

func videoHasFormat(s VideoSettings, f VideoFormat) bool {
	for _, x := range s.Formats {
		if x == f {
			return true
		}
	}
	return false
}

// videoPickScenes is every scene, or the one page names (its id, file or title).
func (l *Library) videoPickScenes(id string, page *string) ([]VideoScene, error) {
	scenes, err := videoScenesFor(l, id)
	if err != nil {
		return nil, err
	}
	if page == nil {
		return scenes, nil
	}
	sc, err := l.videoPickScene(id, scenes, *page)
	if err != nil {
		return nil, err
	}
	return []VideoScene{sc}, nil
}

// videoPickScene is the scene page names: its id, file or title.
func (l *Library) videoPickScene(id string, scenes []VideoScene, page string) (VideoScene, error) {
	pid := page
	if p, err := l.deckPage(id, page); err == nil {
		pid = p.ID
	}
	for _, sc := range scenes {
		if sc.ID == pid {
			return sc, nil
		}
	}
	return VideoScene{}, deckErr(http.StatusNotFound, "No such scene: "+page, "Нет такой сцены: "+page)
}

// videoLint finds in the board's own scripts what runs on the wall clock.
func (l *Library) videoLint(id string, sc VideoScene) []VideoCheckIssue {
	file := l.ResolveProjectFile(id, strings.Split(sc.Path, "/"))
	if file == "" {
		return nil
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	return videoLintSource(sc.ID, jscompat.DecodeUTF8(raw))
}

// videoLintSource is videoLint on a board's source: one finding per code.
func videoLintSource(scene, src string) []VideoCheckIssue {
	var code strings.Builder
	for _, m := range videoScriptRe.FindAllStringSubmatch(src, -1) {
		if videoSrcRe.MatchString(m[1]) {
			continue
		}
		code.WriteString(m[2])
		code.WriteByte('\n')
	}
	out := []VideoCheckIssue{}
	for _, lint := range videoLints {
		if lint.re.MatchString(code.String()) {
			out = append(out, VideoCheckIssue{Scene: scene, Code: lint.code, Level: lint.level, Msg: locale.Pick(lint.en, lint.ru)})
		}
	}
	return out
}

// videoPastEnd: moments that start after the scene has ended.
func videoPastEnd(sc VideoScene) []VideoCheckIssue {
	out := []VideoCheckIssue{}
	for _, m := range sc.Moments {
		if m.At < sc.Dur {
			continue
		}
		at, block := m.At, m.Block
		out = append(out, VideoCheckIssue{Scene: sc.ID, At: &at, Block: &block, Code: VideoCheckPastEnd, Level: VideoCheckError,
			Msg: locale.Pick("The moment starts after the scene ends ("+videoSecondsText(m.At)+" of "+videoSecondsText(sc.Dur)+")",
				"Момент начинается после конца сцены ("+videoSecondsText(m.At)+" из "+videoSecondsText(sc.Dur)+")")})
	}
	return out
}

// videoSecondsText is ms as seconds for a person: 1.8 s.
func videoSecondsText(ms int64) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64) + locale.Pick(" s", " с")
}

// videoAudit is what the runtime's audit answers.
type videoAudit struct {
	Code  string
	Block *string
	Px    int
}

// videoCheckInChrome looks at the scene's last frame in f, and captures two times twice to see they stay the same.
func (l *Library) videoCheckInChrome(ctx context.Context, tab *deckTab, id string, sc VideoScene, f VideoFormat) ([]VideoCheckIssue, error) {
	end := max(sc.Dur-videoCheckBeforeEnd, 0)
	mid := sc.Dur / 2
	shots := map[int64][]byte{}
	// mid, end, then mid and end again after the start: a frame must not depend on the way it was reached.
	order := []int64{mid, end, 0, mid, end}
	var unstable *int64
	err := l.videoFrames(ctx, tab, id, sc, f, false, order, func(v videoShot) error {
		if v.I < 2 {
			shots[v.T] = v.JPEG
			return nil
		}
		if v.I > 2 && unstable == nil && sha256.Sum256(shots[v.T]) != sha256.Sum256(v.JPEG) {
			at := v.T
			unstable = &at
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	fm := f
	out := []VideoCheckIssue{}
	if unstable != nil {
		out = append(out, VideoCheckIssue{Scene: sc.ID, Format: &fm, At: unstable, Code: VideoCheckUnstable, Level: VideoCheckError,
			Msg: locale.Pick("The same time draws a different frame each time: the board has motion that is not driven by the scene's time",
				"Одно и то же время рисует разные кадры: на доске есть движение не от времени сцены")})
	}
	minFont := 0
	if f == VideoPortrait {
		minFont = videoMinFont
	}
	raw, err := tab.eval(`window.tkVideo.audit({minFont: ` + strconv.Itoa(minFont) + `})`)
	if err != nil {
		return nil, err
	}
	for _, a := range videoAudits(raw) {
		code, level := a.Code, VideoCheckWarn
		switch code {
		case VideoCheckOverflow, VideoCheckOffscreen:
			level = VideoCheckError
		case VideoCheckSmallText:
		default:
			continue
		}
		at := end
		out = append(out, VideoCheckIssue{Scene: sc.ID, Format: &fm, At: &at, Block: a.Block, Code: code, Level: level,
			Msg: videoAuditMsg(code, a.Px)})
	}
	return out, nil
}

// videoAudits reads the runtime's audit; what is not one of its findings is left out.
func videoAudits(raw any) []videoAudit {
	list, _ := raw.([]any)
	out := []videoAudit{}
	for _, item := range list {
		m, _ := item.(map[string]any)
		code, _ := m["code"].(string)
		if code == "" {
			continue
		}
		a := videoAudit{Code: code}
		if b, ok := m["block"].(string); ok {
			a.Block = &b
		}
		if px, ok := m["px"].(float64); ok {
			a.Px = int(px)
		}
		out = append(out, a)
	}
	return out
}

// videoAuditMsg words a finding of the runtime for a person.
func videoAuditMsg(code string, px int) string {
	switch code {
	case VideoCheckOverflow:
		return locale.Pick("Text overflows its box at the scene's end", "Текст не помещается в свой блок к концу сцены")
	case VideoCheckOffscreen:
		return locale.Pick("A block is outside the frame at the scene's end", "Блок выходит за кадр к концу сцены")
	}
	n := strconv.Itoa(px)
	return locale.Pick("Text of "+n+" px in 9:16: under 28 px is hard to read on a phone", "Текст "+n+" px в 9:16: мельче 28 px на телефоне не прочесть")
}
