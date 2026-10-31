package apps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ly-solutions/harnessly/internal/agents"
)

// What a chat's launch gets of the apps on in it (agents.AppLauncher): each app's package as a Claude Code plugin, their
// instructions in one system-prompt file, the snapshot of their tool rules (policy.go), and a settings file that is the
// chat's own plus the app-guard hook, which decides every tool call by that snapshot (harnessly hook app-guard
// --policy <file>). The files are written under <data>/apps/launch, named by their content, so a launch with the same
// apps and the same base settings reuses them.

// GuardTimeoutSeconds is how long Claude waits for the guard. The guard reads one local file and answers at once; one
// that could not read it refuses the call.
const GuardTimeoutSeconds = 15

// Launcher is agents.AppLauncher over a registry.
type Launcher struct {
	Registry *Registry
	// GuardCommand is the hook command line; "" is `harnessly hook app-guard` off the agents' PATH.
	GuardCommand string
}

var _ agents.AppLauncher = (*Launcher)(nil)

// Claude is what a Claude Code chat with the apps adds to its launch.
func (l *Launcher) Claude(ids []string, settingsPath string) (agents.ClaudeApps, error) {
	var out agents.ClaudeApps
	var prompt strings.Builder
	snap := Snapshot{Protected: []string{l.Registry.Root}}
	for _, id := range ids {
		a, err := l.Registry.Get(id)
		if err != nil {
			return out, fmt.Errorf("app %s: %w", id, err)
		}
		if a.Manifest == nil || !a.Manifest.HasTarget(TargetChat) {
			return out, fmt.Errorf("app %s does not work in a chat", id)
		}
		out.PluginDirs = append(out.PluginDirs, a.Dir)
		snap.Apps = append(snap.Apps, SnapshotApp{ID: a.ID, Name: a.Manifest.Name, Tools: toolsOf(a.Manifest)})
		snap.Protected = append(snap.Protected, a.Dir)
		text, err := instructions(a)
		if err != nil {
			return out, fmt.Errorf("app %s: %w", id, err)
		}
		if text != "" {
			fmt.Fprintf(&prompt, "## App: %s (%s)\n\n%s\n\n", a.Manifest.Name, a.ID, text)
		}
	}
	dir := filepath.Join(l.Registry.Root, "launch")
	if prompt.Len() > 0 {
		body := "# Apps on in this chat\n\nThe person turned these apps on in this chat. Keep to what each says; a tool the app " +
			"does not allow is refused by its guard, and some calls wait for the person to confirm.\n\n" + prompt.String()
		path, err := writeByContent(dir, "prompt", ".md", []byte(body))
		if err != nil {
			return out, err
		}
		out.PromptFile = path
	}
	rawSnap, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return out, err
	}
	policy, err := writeByContent(dir, "policy", ".json", rawSnap)
	if err != nil {
		return out, err
	}
	settings, err := guardSettings(settingsPath, l.guardCommand()+" --policy "+agents.ShellQuote(policy))
	if err != nil {
		return out, err
	}
	if out.Settings, err = writeByContent(dir, "settings", ".json", settings); err != nil {
		return out, err
	}
	return out, nil
}

func (l *Launcher) guardCommand() string {
	if l.GuardCommand != "" {
		return l.GuardCommand
	}
	return "harnessly hook app-guard"
}

func toolsOf(m *Manifest) *ToolPolicy {
	if m.Agent == nil {
		return nil
	}
	return m.Agent.Tools
}

// instructions is an app's instructions file, trimmed; "" when it has none.
func instructions(a Installed) (string, error) {
	if a.Manifest.Agent == nil || a.Manifest.Agent.Instructions == "" {
		return "", nil
	}
	p := &Package{Dir: a.Dir, Manifest: a.Manifest}
	raw, err := p.File(a.Manifest.Agent.Instructions)
	if err != nil {
		return "", err
	}
	if len(raw) > MaxInstructionsBytes {
		raw = raw[:MaxInstructionsBytes]
	}
	return strings.TrimSpace(string(raw)), nil
}

// guardSettings is the settings file at path with the app-guard hook first among its PreToolUse hooks.
func guardSettings(path, command string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	pre, _ := hooks["PreToolUse"].([]any)
	guard := map[string]any{
		"matcher": "*",
		"hooks":   []any{map[string]any{"type": "command", "command": command, "timeout": GuardTimeoutSeconds}},
	}
	hooks["PreToolUse"] = append([]any{guard}, pre...)
	settings["hooks"] = hooks
	return json.MarshalIndent(settings, "", "  ")
}

// SnapshotIntact says whether a snapshot's bytes are the ones its name was made from (writeByContent): a snapshot
// changed after the launch is not one the guard decides by.
func SnapshotIntact(path string, body []byte) bool {
	sum := sha256.Sum256(body)
	return filepath.Base(path) == "policy-"+hex.EncodeToString(sum[:8])+".json"
}

// writeByContent writes body to <dir>/<prefix>-<hash><ext> unless it is there, and gives the path.
func writeByContent(dir, prefix, ext string, body []byte) (string, error) {
	sum := sha256.Sum256(body)
	path := filepath.Join(dir, prefix+"-"+hex.EncodeToString(sum[:8])+ext)
	if have, err := os.ReadFile(path); err == nil && string(have) == string(body) {
		return path, nil
	}
	if err := writeFileAtomic(path, body); err != nil {
		return "", err
	}
	return path, nil
}
