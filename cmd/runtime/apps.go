package main

import (
	"os"

	"github.com/ly-solutions/harnessly/internal/agents"
	"github.com/ly-solutions/harnessly/internal/apps"
)

// appGuardCommand is the app-guard hook's command line: HARNESSLY_BIN when the deployment names the CLI, otherwise
// harnessly off the agents' PATH.
func appGuardCommand() string {
	if bin := os.Getenv("HARNESSLY_BIN"); bin != "" {
		return agents.ShellQuote(bin) + " hook app-guard"
	}
	return ""
}

// appsOK is convo.Deps.AppsOK over the machine's registry: an app turns on in a chat when it is installed and works in
// one.
func appsOK(r *apps.Registry) func(ids []string) string {
	return func(ids []string) string {
		list, err := r.List()
		if err != nil {
			return ids[0]
		}
		byID := make(map[string]apps.Installed, len(list))
		for _, a := range list {
			byID[a.ID] = a
		}
		for _, id := range ids {
			a, ok := byID[id]
			if !ok || a.Manifest == nil || !a.Manifest.HasTarget(apps.TargetChat) {
				return id
			}
		}
		return ""
	}
}
