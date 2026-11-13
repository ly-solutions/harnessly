package wire

import "testing"

func TestNoticeSummarySettingValidatesAndSticks(t *testing.T) {
	ok := RuntimeSettings{IdleStop: DefaultIdleStop, NoticeSummary: &NoticeSummaryModel{Provider: "codex", Model: "gpt-5-mini"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []NoticeSummaryModel{{Provider: "gemini"}, {Provider: "off", Model: "x"}, {Provider: "claude", Model: "--x"}, {Provider: "claude", Model: "a b"}} {
		s := RuntimeSettings{IdleStop: DefaultIdleStop, NoticeSummary: &bad}
		if s.Validate() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	kept := RuntimeSettings{IdleStop: DefaultIdleStop}.over(ok)
	if kept.NoticeSummary == nil || kept.NoticeSummary.Model != "gpt-5-mini" {
		t.Errorf("a PUT without it must keep it: %+v", kept.NoticeSummary)
	}
	if !(&NoticeSummaryModel{Provider: "auto"}).Auto() || !(*NoticeSummaryModel)(nil).Auto() || !(&NoticeSummaryModel{Provider: "off"}).Off() {
		t.Error("Auto/Off")
	}
}
