package apps

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
)

// Explain names the layer that keeps an app off.
func TestExplainNamesTheLayerThatKeepsAnAppOff(t *testing.T) {
	m := func(mut func(*Manifest)) *Manifest {
		x := &Manifest{ID: "acme.pulse", Trust: TrustSandboxed, Targets: []string{TargetChat}}
		if mut != nil {
			mut(x)
		}
		return x
	}
	off := func(lines []ExplainLine) string {
		for _, l := range lines {
			if l.Off {
				return l.Layer + ": " + l.Says
			}
		}
		return ""
	}
	chat := func(provider string, on ...string) *ExplainChat { return &ExplainChat{Provider: provider, Apps: on} }
	for _, c := range []struct {
		name string
		in   ExplainInput
		off  string
	}{
		{"not installed", ExplainInput{ID: "acme.pulse"}, "machine: acme.pulse is not installed"},
		{"on", ExplainInput{ID: "acme.pulse", App: &Installed{ID: "acme.pulse", Version: "1.0.0", Manifest: m(nil)},
			Chat: chat("claude", "acme.pulse")}, ""},
		{"off in the chat", ExplainInput{ID: "acme.pulse", App: &Installed{ID: "acme.pulse", Manifest: m(nil)},
			Chat: chat("claude")}, "chat: off in this chat"},
		{"on in codex", ExplainInput{ID: "acme.pulse", App: &Installed{ID: "acme.pulse", Manifest: m(nil)},
			Chat: chat("codex", "acme.pulse")}, ""},
		{"another agent", ExplainInput{ID: "acme.pulse", App: &Installed{ID: "acme.pulse", Manifest: m(nil)},
			Chat: chat("gemini", "acme.pulse")}, "chat: this chat runs gemini"},
		{"waits for a secret", ExplainInput{ID: "acme.pulse", App: &Installed{ID: "acme.pulse", Manifest: m(nil)},
			Waiting: []string{"gitlab-token"}}, "setup: waits for secrets: gitlab-token"},
		{"not a chat app", ExplainInput{ID: "acme.pulse", App: &Installed{ID: "acme.pulse",
			Manifest: m(func(x *Manifest) { x.Targets = []string{TargetApp} })}, Chat: chat("claude")}, "activation: works in app"},
		{"host too old", ExplainInput{ID: "acme.pulse", HostVersion: "0.10.0", App: &Installed{ID: "acme.pulse",
			Manifest: m(func(x *Manifest) { x.Harnessly = ">=0.16.0" })}}, "machine: this harnessly is 0.10.0"},
		{"muted team app", ExplainInput{ID: "acme.pulse", App: &Installed{ID: "acme.pulse", Team: true, Muted: true,
			Manifest: m(nil)}}, "team: the person turned it off"},
	} {
		got := off(Explain(c.in))
		if c.off == "" && got != "" || !strings.HasPrefix(got, c.off) {
			t.Errorf("%s: off %q, want %q", c.name, got, c.off)
		}
	}
	// The wall is told as it is: none here, down with its cause, or standing.
	wall := func(in ExplainInput) string {
		lines := Explain(in)
		return lines[len(lines)-1].Says
	}
	base := ExplainInput{ID: "acme.pulse", App: &Installed{ID: "acme.pulse", Manifest: m(nil)}}
	if s := wall(base); !strings.HasPrefix(s, "no wall") {
		t.Errorf("no wall: %q", s)
	}
	base.Wall, base.WallErr = true, errors.New("bwrap: not found")
	if s := wall(base); !strings.Contains(s, "bwrap: not found") {
		t.Errorf("wall down: %q", s)
	}
	base.WallErr = nil
	if s := wall(base); !strings.Contains(s, "inside the wall") {
		t.Errorf("wall up: %q", s)
	}
}

// A dev install whose folder vanished (a scratch folder after a reboot) is named as the layer that keeps the app off,
// and the host's tools say so instead of "the window is not connected".
func TestExplainNamesADevInstallWhoseFolderIsGone(t *testing.T) {
	m := &Manifest{ID: "test.fill-form", Trust: TrustSandboxed, Targets: []string{TargetChat},
		Views: []View{{ID: "main", Targets: []string{TargetPane}}}}
	here := Installed{ID: "test.fill-form", Version: "0.1.0", Manifest: m, Dev: true, Source: t.TempDir()}
	gone := here
	gone.Source = filepath.Join(here.Source, "wiped")
	copied := gone
	copied.Dev = false
	if here.DevSourceGone() || copied.DevSourceGone() || !gone.DevSourceGone() {
		t.Fatalf("gone: here %v, a copied install %v, wiped %v", here.DevSourceGone(), copied.DevSourceGone(), gone.DevSourceGone())
	}
	var off string
	for _, l := range Explain(ExplainInput{ID: gone.ID, App: &gone, Chat: &ExplainChat{Provider: "claude", Apps: []string{gone.ID}}}) {
		if l.Off {
			off = l.Layer + ": " + l.Says
			break
		}
	}
	if !strings.HasPrefix(off, "machine: that folder is gone") {
		t.Fatalf("the layer that keeps it off: %q", off)
	}
	for _, l := range Explain(ExplainInput{ID: here.ID, App: &here, Chat: &ExplainChat{Provider: "claude", Apps: []string{here.ID}}}) {
		if l.Off {
			t.Fatalf("a dev install whose folder is there: %s: %s", l.Layer, l.Says)
		}
	}
	opened := ""
	tools := HostTools{Open: func(view string) error { opened = view; return nil }}
	if text := resultText(tools.open(gone, "")); opened != "" || !strings.Contains(text, "is gone") || !strings.Contains(text, "harnessly app dev") {
		t.Fatalf("open of a vanished install: opened %q, said %q", opened, text)
	}
	if text := resultText(tools.open(here, "")); opened != "test.fill-form/main" {
		t.Fatalf("open of a live install: opened %q, said %q", opened, text)
	}
}

func resultText(res *jsvalue.Object) string {
	content, _ := res.Get("content")
	text, _ := content.([]any)[0].(*jsvalue.Object).Get("text")
	return text.(string)
}
