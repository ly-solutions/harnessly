package design

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// A video's model: state, rev, settings, scene meta (video_scenes.go) and comments. A video is a deck folder with
// meta/video.json, so notes, theme and comments are the deck's (deck.go).

// IsVideo: the project is a video. Every video keeps meta/video.json from the moment it is made, beside the deck's
// meta/deck.json, so every runtime of a shared library tells a video the same way. A video is a deck too (IsDeck).
func (l *Library) IsVideo(id string) bool {
	return IsLibraryID(id) && isRealFile(l.deckFile(id, VideoSettingsFile))
}

// ValidateVideoSettings checks a video's settings against the contract's bounds.
func ValidateVideoSettings(s VideoSettings) error {
	if s.V != 1 {
		return deckErr(http400, "video.v must be 1", "video.v должен быть 1")
	}
	if len(s.Formats) == 0 || len(s.Formats) > len(VideoFormats) {
		return deckErr(http400, "formats: 16:9, 9:16 or both", "formats: 16:9, 9:16 или оба")
	}
	seen := map[VideoFormat]bool{}
	for _, f := range s.Formats {
		if !f.Valid() || seen[f] {
			return deckErr(http400, "formats: 16:9, 9:16 or both", "formats: 16:9, 9:16 или оба")
		}
		seen[f] = true
	}
	if s.FPS != VideoFPS {
		return deckErr(http400, fmt.Sprintf("fps must be %d", VideoFPS), fmt.Sprintf("fps должен быть %d", VideoFPS))
	}
	if s.Seconds != nil && (*s.Seconds < VideoSecondsMin || *s.Seconds > VideoSecondsMax) {
		return deckErr(http400, "seconds: from 15 to 300, or null for any", "seconds: от 15 до 300 или null, если не важно")
	}
	if s.Voice != VideoVoiceNone {
		return deckErr(http400, "voice must be none", "voice должен быть none")
	}
	return nil
}

// orderedFormats puts formats in the order of VideoFormats.
func orderedFormats(in []VideoFormat) []VideoFormat {
	out := []VideoFormat{}
	for _, f := range VideoFormats {
		for _, g := range in {
			if f == g {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// ReadVideoSettings is meta/video.json. A project without the file is not a video (404); a file that does not parse is
// an error: the settings are not guessed.
func (l *Library) ReadVideoSettings(id string) (VideoSettings, error) {
	if !IsLibraryID(id) {
		return VideoSettings{}, deckErr(http404, "No such design", "Нет такого макета")
	}
	raw, err := readFileLimited(l.deckFile(id, VideoSettingsFile), maxDeckFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return VideoSettings{}, deckErr(http404, "Not a video", "Это не видео")
	}
	if err != nil {
		return VideoSettings{}, err
	}
	s := DefaultVideoSettings()
	if err := json.Unmarshal(raw, &s); err != nil {
		return VideoSettings{}, fmt.Errorf("%s: %w", VideoSettingsFile, err)
	}
	return s, nil
}

// WriteVideoSettings validates and writes meta/video.json, the formats in their fixed order.
func (l *Library) WriteVideoSettings(id string, s VideoSettings) error {
	if !IsLibraryID(id) {
		return deckErr(http404, "No such design", "Нет такого макета")
	}
	s.Formats = orderedFormats(s.Formats)
	if err := ValidateVideoSettings(s); err != nil {
		return err
	}
	if err := l.requireDeck(id); err != nil {
		return err
	}
	if err := l.ensureMetaDirs(id); err != nil {
		return err
	}
	_, err := writeIfChanged(l.deckFile(id, VideoSettingsFile), deckJSON(s))
	return err
}

// VideoInit is what a new video is made with (the create screen's settings). Formats empty: 16:9.
type VideoInit struct {
	Formats []VideoFormat
	Seconds *int
	Subs    bool
}

// BuildVideoSettings are the settings of a new video.
func BuildVideoSettings(in VideoInit) VideoSettings {
	s := DefaultVideoSettings()
	if len(in.Formats) > 0 {
		s.Formats = orderedFormats(in.Formats)
	}
	s.Seconds, s.Subs = in.Seconds, in.Subs
	return s
}

// InitVideo writes the settings of a new video. The deck's files come first (InitDeck): a video is a deck folder.
func (l *Library) InitVideo(id string, in VideoInit) error {
	return l.WriteVideoSettings(id, BuildVideoSettings(in))
}

// ---- settings patch

// videoSettingsPatch merges the keys of a PUT onto the settings: a key that is absent stays, seconds may be null.
func videoSettingsPatch(cur VideoSettings, patch map[string]json.RawMessage) (VideoSettings, error) {
	s := cur
	for k, raw := range patch {
		var err error
		switch k {
		case "formats":
			var fs []VideoFormat
			if err = json.Unmarshal(raw, &fs); err == nil {
				s.Formats = fs
			}
		case "subs":
			err = json.Unmarshal(raw, &s.Subs)
		case "seconds":
			var n *int
			if err = json.Unmarshal(raw, &n); err == nil {
				s.Seconds = n
			}
		default:
			return s, deckErr(http400, "settings."+k+": unknown field", "settings."+k+": неизвестное поле")
		}
		if err != nil {
			return s, deckErr(http400, "settings."+k+": a wrong value", "settings."+k+": неверное значение")
		}
	}
	return s, nil
}

// SetVideoSettings applies a partial settings value (the PUT) and returns what was written.
func (l *Library) SetVideoSettings(id string, patch map[string]json.RawMessage) (VideoSettings, error) {
	if err := l.requireVideo(id); err != nil {
		return VideoSettings{}, err
	}
	cur, err := l.ReadVideoSettings(id)
	if err != nil {
		return VideoSettings{}, err
	}
	next, err := videoSettingsPatch(cur, patch)
	if err != nil {
		return VideoSettings{}, err
	}
	if err := ValidateVideoSettings(next); err != nil {
		return VideoSettings{}, err
	}
	if err := l.WriteVideoSettings(id, next); err != nil {
		return VideoSettings{}, err
	}
	return l.ReadVideoSettings(id)
}

// requireVideo: the project exists and is a video.
func (l *Library) requireVideo(id string) error {
	if err := l.requireDeck(id); err != nil {
		return err
	}
	if !l.IsVideo(id) {
		return deckErr(http404, "Not a video", "Это не видео")
	}
	return nil
}

// ---- scene

// SetVideoScene changes a scene's duration and transition. A key of patch that is absent stays; dur null is automatic,
// transition null is the board's own. ref is a page's id, file or title. Answers the scene as the video now has it.
func (l *Library) SetVideoScene(id, ref string, patch map[string]json.RawMessage) (VideoScene, error) {
	if err := l.requireVideo(id); err != nil {
		return VideoScene{}, err
	}
	p, err := l.deckPage(id, ref)
	if err != nil {
		return VideoScene{}, err
	}
	meta, err := l.ReadVideoSceneMeta(id, p.ID)
	if err != nil {
		return VideoScene{}, err
	}
	for k, raw := range patch {
		var err error
		switch k {
		case "dur":
			var d *int64
			if err = json.Unmarshal(raw, &d); err == nil {
				meta.Dur = d
			}
		case "transition":
			var t *VideoTransition
			if err = json.Unmarshal(raw, &t); err == nil {
				meta.Transition = t
			}
		default:
			return VideoScene{}, deckErr(http400, k+": unknown field", k+": неизвестное поле")
		}
		if err != nil {
			return VideoScene{}, deckErr(http400, k+": a wrong value", k+": неверное значение")
		}
	}
	if t := meta.Transition; t != nil && t.Kind == VideoTransitionNone {
		t.Ms = 0
	}
	if err := l.writeVideoSceneMeta(id, p.ID, meta); err != nil {
		return VideoScene{}, err
	}
	return l.videoSceneByID(id, p.ID)
}

// videoSceneByID is one scene as the video now has it, on the time line.
func (l *Library) videoSceneByID(id, page string) (VideoScene, error) {
	st, err := l.ReadVideoSettings(id)
	if err != nil {
		return VideoScene{}, err
	}
	scenes, err := l.VideoScenes(id, st)
	if err != nil {
		return VideoScene{}, err
	}
	for _, sc := range scenes {
		if sc.ID == page {
			return sc, nil
		}
	}
	return VideoScene{}, deckErr(http404, "No such scene", "Нет такой сцены")
}

// ---- comments

type videoCommentFields struct {
	t0, t1 *int64
	format VideoFormat
	block  string
}

// videoCommentAt checks the time fields of a new comment. A deck's comment has none (nil, nil). A video's has t0 within
// its scene, t1 (when given) from t0 to the scene's end, and a format the video makes.
func (l *Library) videoCommentAt(id string, p Page, spec DeckCommentSpec) (*videoCommentFields, error) {
	if spec.T0 == nil {
		if spec.T1 != nil || spec.Format != "" || spec.Block != "" {
			return nil, deckErr(http400, "t1, format and block go with t0", "t1, format и block — только вместе с t0")
		}
		return nil, nil
	}
	if !l.IsVideo(id) {
		return nil, deckErr(http400, "Only a video has comments on time", "Комментарии ко времени — только у видео")
	}
	settings, err := l.ReadVideoSettings(id)
	if err != nil {
		return nil, err
	}
	enabled := false
	for _, f := range settings.Formats {
		enabled = enabled || f == spec.Format
	}
	if !enabled {
		return nil, deckErr(http400, "format: one the video makes", "format: из тех, что делает видео")
	}
	sc, err := l.videoSceneOf(id, p)
	if err != nil {
		return nil, err
	}
	t0 := *spec.T0
	if t0 < 0 || t0 > sc.Dur {
		return nil, deckErr(http400, "t0: from 0 to the scene's length", "t0: от 0 до длины сцены")
	}
	if spec.T1 != nil && (*spec.T1 < t0 || *spec.T1 > sc.Dur) {
		return nil, deckErr(http400, "t1: from t0 to the scene's length", "t1: от t0 до длины сцены")
	}
	block, _ := CleanText(spec.Block, 80)
	return &videoCommentFields{t0: spec.T0, t1: spec.T1, format: spec.Format, block: block}, nil
}

// ---- state

// videoRev is the newest mtime (ms) of the deck's files, meta/video.json, meta/scenes and the published video.json.
func (l *Library) videoRev(id string) int64 {
	newest := l.deckRev(id)
	touch := func(path string) {
		if st, err := os.Lstat(path); err == nil && st.ModTime().UnixMilli() > newest {
			newest = st.ModTime().UnixMilli()
		}
	}
	touch(l.deckFile(id, VideoSettingsFile))
	touch(l.deckFile(id, VideoPublishedFile))
	dir := l.deckFile(id, VideoScenesDir)
	touch(dir)
	names, _ := readNames(dir)
	for _, n := range names {
		touch(filepath.Join(dir, n))
	}
	return newest
}

// VideoStateOf is everything the video page reads in one go.
func (l *Library) VideoStateOf(id string) (VideoState, error) {
	if err := l.requireVideo(id); err != nil {
		return VideoState{}, err
	}
	settings, err := l.ReadVideoSettings(id)
	if err != nil {
		return VideoState{}, err
	}
	deck, err := l.ReadDeckSettings(id)
	if err != nil {
		return VideoState{}, err
	}
	scenes, err := l.VideoScenes(id, settings)
	if err != nil {
		return VideoState{}, err
	}
	var dur int64
	for _, sc := range scenes {
		dur += sc.Dur
	}
	comments, guests := l.ReadDeckComments(id)
	renders := l.VideoRenders(id, settings, scenes)
	if renders == nil {
		renders = []VideoRenderInfo{}
	}
	return VideoState{Rev: l.videoRev(id), Settings: settings, Deck: deck, Chrome: l.findChrome() != "", Ffmpeg: DeckFFmpeg() != "",
		DurMs: dur, Scenes: scenes, Comments: comments, GuestResolved: guests, Renders: renders, Job: l.VideoRunningJob(id)}, nil
}
