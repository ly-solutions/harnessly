package main

import (
	"context"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/bgmodel"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// The summary of a finished turn is one setting: «Сводки для уведомлений» of the background features. Its
// value is who writes the summary:
//
//	default             the chat's own agent, its cheap model
//	off                 nobody: no summaries
//	claude/<model>      a model of Claude Code; codex/<model> one of Codex ("" after the slash is the cheap one)
//	<provider>/<model>  a model of a connected provider
//
// The first three live in the runtime settings (wire.NoticeSummaryModel), a provider's model in the
// background features' choices; this file reads and writes the two as one, so no page shows them apart.

// noticeSummary is that setting of a data folder.
type noticeSummary struct {
	rs     *wire.FileRuntimeSettings
	runner *bgmodel.Runner
}

func noticeSummaryFor(d deps) noticeSummary {
	return noticeSummary{rs: runtimeSettingsFor(d.DataDir), runner: bgRunnerFor(d)}
}

func isAgentProvider(p string) bool {
	return p == string(wire.ProviderClaude) || p == string(wire.ProviderCodex)
}

// Pick is the setting as a provider and a model: ("", "") is default, ("off", "") is off.
func (n noticeSummary) Pick() (provider, model string) {
	if c, ok := n.runner.Store.Get(bgmodel.FeatureNotifySummary); ok {
		return c.Provider, c.Model
	}
	ns := n.rs.RuntimeSettings().NoticeSummary
	switch {
	case ns.Off():
		return wire.NoticeSummaryOff, ""
	case ns.Auto():
		return "", ""
	}
	return ns.Provider, ns.Model
}

// Value is the setting as the catalog's one value.
func (n noticeSummary) Value() string {
	provider, model := n.Pick()
	switch {
	case provider == "":
		return backgroundAsNow
	case provider == wire.NoticeSummaryOff:
		return wire.NoticeSummaryOff
	}
	return provider + "/" + model
}

// Set writes the setting: provider "" is default, "off" is off, an agent's or a connected provider's name with its
// model otherwise. A connected provider wins over an agent of the same name: its id is the person's own word.
func (n noticeSummary) Set(provider, model string) error {
	agentSide := wire.NoticeSummaryModel{Provider: wire.NoticeSummaryAuto}
	var choice *bgmodel.Choice
	_, extErr := n.runner.Providers.Get(provider)
	switch {
	case provider == "":
	case extErr == nil:
		choice = &bgmodel.Choice{Provider: provider, Model: model}
	case provider == wire.NoticeSummaryOff:
		agentSide = wire.NoticeSummaryModel{Provider: wire.NoticeSummaryOff}
	case isAgentProvider(provider):
		agentSide = wire.NoticeSummaryModel{Provider: provider, Model: strings.TrimSpace(model)}
	default:
		return bgmodel.ErrUnknownProvider
	}
	if err := agentSide.Validate(); err != nil {
		return bgmodel.ErrBadModel
	}
	if _, err := n.runner.Pick(bgmodel.FeatureNotifySummary, choice); err != nil {
		return err
	}
	s := n.rs.RuntimeSettings()
	s.NoticeSummary = &agentSide
	_, err := n.rs.SetRuntimeSettings(s)
	return err
}

// SetValue writes the setting from the catalog's value.
func (n noticeSummary) SetValue(v string) error {
	if v == backgroundAsNow {
		return n.Set("", "")
	}
	if v == wire.NoticeSummaryOff {
		return n.Set(wire.NoticeSummaryOff, "")
	}
	provider, model, _ := strings.Cut(v, "/")
	return n.Set(provider, model)
}

// Takes says whether v is a value of the setting that names no listed option: an agent's model.
func (n noticeSummary) Takes(v string) bool {
	if v == wire.NoticeSummaryOff {
		return true
	}
	provider, model, ok := strings.Cut(v, "/")
	return ok && isAgentProvider(provider) && wire.NoticeSummaryModel{Provider: provider, Model: model}.Validate() == nil
}

// turnSummaryProviderWait bounds a provider's answer: past it the agent's CLI writes the summary.
const turnSummaryProviderWait = 25 * time.Second

// turnSummarizer writes the summary of a turn with the connected provider's model when one is picked, and with the
// agent's CLI otherwise or when the provider failed (the failure is on the feature's record).
func turnSummarizer(dataDir string, cli func(p wire.Provider, model, message string, replies []string) (string, error)) func(p wire.Provider, model, message string, replies []string) (string, error) {
	return func(p wire.Provider, model, message string, replies []string) (string, error) {
		if r := bgRunnerOf(dataDir); r.Picked(bgmodel.FeatureNotifySummary) {
			ctx, cancel := context.WithTimeout(context.Background(), turnSummaryProviderWait)
			text, _, err := r.TurnSummary(ctx, summaryPrompt(message, replies))
			cancel()
			if text = cleanSummary(text); err == nil && text != "" {
				return redactSummary(text), nil
			}
		}
		return cli(p, model, message, replies)
	}
}
