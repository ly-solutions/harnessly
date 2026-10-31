package main

import (
	"fmt"
	"strings"

	"github.com/ly-solutions/harnessly/internal/design"
	"github.com/ly-solutions/harnessly/internal/locale"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// What the first message of a new video tells the agent (WP5 writes the texts).

// videoInit is the create screen's video as the library keeps it.
func videoInit(v *wire.DesignlyVideo) design.VideoInit {
	in := design.VideoInit{Seconds: v.Seconds, Subs: v.Subs}
	for _, f := range v.Formats {
		in.Formats = append(in.Formats, design.VideoFormat(f))
	}
	return in
}

// videoInitOf is the settings a create starts the video with: the page's, or the defaults.
func videoInitOf(c wire.DesignlyCreate) design.VideoInit {
	if c.Video != nil {
		return videoInit(c.Video)
	}
	d := design.DefaultVideoSettings()
	return design.VideoInit{Formats: d.Formats, Seconds: d.Seconds, Subs: d.Subs}
}

// videoKindLine is the kind line of a video: what a scene is and how it is built.
func videoKindLine() string {
	return locale.Pick("The project is a video: every board is one scene, a page of the video in the order of the pages. A scene is an "+
		"adaptive <main> (width 100vw, height 100vh, container-type: size) that lays itself out for landscape 16:9 and portrait 9:16 "+
		"with @container; colour and type come from the --deck-* variables of project/deck-theme.css and var(--deck-font), no "+
		"hard-coded colours or fonts. Name the parts with data-block=\"<Name>\", time their entrance with data-build-in=\"fade|rise|pop\" "+
		"and data-tk-at=\"1.8\" (seconds from the scene's start; data-tk-out to leave), or your own @keyframes. No setTimeout, "+
		"setInterval, Date or Math.random: a frame must depend only on the time.",
		"Проект — видео: каждая доска — одна сцена, страница видео в порядке страниц. Сцена — адаптивный <main> (ширина 100vw, "+
			"высота 100vh, container-type: size), который раскладывается под горизонталь 16:9 и вертикаль 9:16 через @container; "+
			"цвета и шрифт берутся из переменных --deck-* файла project/deck-theme.css и var(--deck-font), без своих цветов и "+
			"шрифтов. Части называй data-block=\"<Имя>\", время появления задавай data-build-in=\"fade|rise|pop\" и "+
			"data-tk-at=\"1.8\" (секунды от начала сцены; data-tk-out — когда уйти) или своими @keyframes. Без setTimeout, "+
			"setInterval, Date и Math.random: кадр зависит только от времени.")
}

// videoLine is the line of the first message that tells the agent what the person asked of the video (v nil: the
// defaults) and how its scenes are timed with the CLI.
func videoLine(v *wire.DesignlyVideo, cli, project string) string {
	in := videoInitOf(wire.DesignlyCreate{Video: v})
	formats := make([]string, 0, len(in.Formats))
	for _, f := range in.Formats {
		formats = append(formats, string(f))
	}
	length := locale.Pick("any length", "длина свободная")
	if in.Seconds != nil {
		length = locale.Pick(fmt.Sprintf("about %d seconds in all", *in.Seconds), fmt.Sprintf("около %d секунд в сумме", *in.Seconds))
	}
	subs := locale.Pick("without subtitles", "без субтитров")
	if in.Subs {
		subs = locale.Pick("with subtitles (they are the scene text: write it with design notes set)", "с субтитрами (это текст сцены: пиши его через design notes set)")
	}
	settings := locale.Pick(fmt.Sprintf("The person asked for a video in %s, %s, %s.", strings.Join(formats, " and "), length, subs),
		fmt.Sprintf("Человек попросил видео в форматах %s, %s, %s.", strings.Join(formats, " и "), length, subs))
	how := locale.Pick(fmt.Sprintf("The settings are in meta/video.json (the runtime owns it: do not edit it); scene lengths change only with `%[1]s design video scene set %[2]s <scene> --dur 8s|auto`. "+
		"After an edit run `%[1]s design video check %[2]s --scene <scene>`, then `%[1]s design video frame %[2]s <scene> --strip 0:6:1s` in both formats and open the frames with Read.", cli, project),
		fmt.Sprintf("Настройки лежат в meta/video.json (файл ведёт рантайм: не правь его); длину сцен меняй только командой `%[1]s design video scene set %[2]s <сцена> --dur 8s|auto`. "+
			"После правки запусти `%[1]s design video check %[2]s --scene <сцена>`, затем `%[1]s design video frame %[2]s <сцена> --strip 0:6:1s` в обоих форматах и открой кадры через Read.", cli, project))
	return settings + " " + how
}
