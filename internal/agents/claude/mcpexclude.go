package claude

import (
	"encoding/json"
	"os"
	"slices"
	"sort"
	"strconv"
)

// The built-in MCP groups a model profile may leave out of a launch (providers.FileProfile exclude_builtin_mcp).
const (
	// ExcludeMedia hides the media tools (images, video, audio) of the runtime's server: some providers reject a whole
	// request whose tool list names tools they do not support.
	ExcludeMedia = "media"
	// ExcludeRuntime leaves the runtime's own MCP server out of the launch.
	ExcludeRuntime = "runtime"
	// ExcludeApps leaves the apps' MCP servers out of the launch.
	ExcludeApps = "apps"
)

// ExcludeGroups are the values a profile may list, for validation.
var ExcludeGroups = []string{ExcludeMedia, ExcludeRuntime, ExcludeApps}

// mediaTools are the tool names of the media group.
var mediaTools = []string{"generate_images", "wait_images", "generate_video", "wait_video", "cancel_video", "generate_audio"}

// MediaDeny are the permission rules that remove the media tools from every server of the --mcp-config file at path.
func MediaDeny(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Servers))
	for n := range cfg.Servers {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		for _, t := range mediaTools {
			out = append(out, "mcp__"+n+"__"+t)
		}
	}
	return out
}

// excludedNote is what the launch journal says about the exclusion: group names and the tool count only, never
// addresses, tokens or request contents.
func excludedNote(groups, denied []string) []string {
	var out []string
	for _, g := range groups {
		if g == ExcludeMedia {
			if len(denied) == 0 {
				out = append(out, g+" (not applied: no server config)")
				continue
			}
			out = append(out, g+" ("+strconv.Itoa(len(mediaTools))+" tools)")
			continue
		}
		if slices.Contains(ExcludeGroups, g) {
			out = append(out, g)
		}
	}
	return out
}
