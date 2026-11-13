package bgmodel

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/locale"
)

// SearchSource is one thing the search found, as the model is shown it.
type SearchSource struct {
	// Kind says what it is in a word (a chat, a document, a setting); Title is its name; Text is what was found in it.
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// The bounds of one answer: how many sources the model is shown and how much of each.
const (
	SearchSourcesMax    = 12
	SearchSourceTextMax = 1200
	SearchQuestionMax   = 400
)

var searchMark = regexp.MustCompile(`\s*\[(\d{1,2})\]`)

func clipRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max])
}

// SearchAnswer answers a question from what the search found. cited are the sources the answer rests on, by their
// place in sources, in the order the answer names them. used is false when no model is picked for the feature: there
// is nothing the search did before, so the caller says a model has to be picked.
func (r *Runner) SearchAnswer(ctx context.Context, question string, sources []SearchSource) (text string, cited []int, used bool, err error) {
	if len(sources) > SearchSourcesMax {
		sources = sources[:SearchSourcesMax]
	}
	system := locale.Pick(
		"You answer a person's question from the numbered sources below: their own chats, documents, designs and settings. Use only what the sources say. Answer in two or three short sentences, in the language of the question. After each statement put the number of the source it comes from in square brackets, like [2]. If the sources do not answer the question, say so in one sentence and cite nothing. No markdown, no preamble.",
		"Ты отвечаешь на вопрос человека по пронумерованным источникам ниже: это его чаты, документы, макеты и настройки. Опирайся только на то, что сказано в источниках. Ответь двумя-тремя короткими предложениями на языке вопроса. После каждого утверждения ставь в квадратных скобках номер источника, из которого оно взято, например [2]. Если в источниках ответа нет, скажи это одним предложением и ни на что не ссылайся. Без разметки и вступлений.")
	var b strings.Builder
	b.WriteString(locale.Pick("Question: ", "Вопрос: "))
	b.WriteString(clipRunes(question, SearchQuestionMax))
	b.WriteString("\n\n")
	for i, s := range sources {
		b.WriteString("[" + strconv.Itoa(i+1) + "] " + clipRunes(s.Kind, 40) + " · " + clipRunes(s.Title, 200) + "\n")
		if body := clipRunes(s.Text, SearchSourceTextMax); body != "" {
			b.WriteString(body + "\n")
		}
		b.WriteString("\n")
	}
	raw, used, err := r.Run(ctx, FeatureSearchAnswer, Call{System: system, Prompt: b.String(), MaxTokens: 400, Timeout: 20 * time.Second})
	if !used || err != nil {
		return "", nil, used, err
	}
	cited = []int{}
	for _, m := range searchMark.FindAllStringSubmatch(raw, -1) {
		n, _ := strconv.Atoi(m[1])
		if n >= 1 && n <= len(sources) && !slices.Contains(cited, n-1) {
			cited = append(cited, n-1)
		}
	}
	text = strings.Join(strings.Fields(searchMark.ReplaceAllString(raw, "")), " ")
	if text == "" {
		return "", nil, true, &Error{Reason: ReasonEmpty}
	}
	return text, cited, true, nil
}
