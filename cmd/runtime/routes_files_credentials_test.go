package main

import (
	"os"
	"path/filepath"
	"testing"
)

// With the workspace being the whole home folder, the machine's keys and the tools' sign-ins still open to nobody.
func TestWorkspaceCredentialsClosed(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(home, ".harnessly")
	t.Setenv("AGENT_DATA_DIR", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("RUNTIME_UPLOADS_DIR", "")
	write := func(rel string) string {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	access := func(workspace, dataDir string) func(string) bool {
		a := workspaceAccess(deps{Home: home, DataDir: dataDir, WorkspaceRoot: workspace})
		return func(p string) bool { return a.Download(p).OK }
	}
	open := access(home, data)
	for _, rel := range []string{".harnessly/machine.json", ".harnessly/cli-token", ".harnessly/config.json", ".codex/auth.json",
		".config/gh/hosts.yml", ".config/harnessly/runtime.env", ".git-credentials", ".kube/config", ".docker/config.json",
		".claude.json", ".bash_history", ".local/state/harnessly/safe.key"} {
		if open(write(rel)) {
			t.Errorf("%s is served", rel)
		}
	}
	link := filepath.Join(home, "innocent.txt")
	if err := os.Symlink(filepath.Join(data, "machine.json"), link); err != nil {
		t.Fatal(err)
	}
	if open(link) {
		t.Error("a link to machine.json is served")
	}
	for _, rel := range []string{"work/report.md", ".harnessly/images/a.png", ".harnessly/media/a.mp4", ".harnessly/uploads/a.txt",
		".harnessly/v2/uploads/a.txt", ".codex/generated_images/a.png", ".config/nvim/init.lua"} {
		if !open(write(rel)) {
			t.Errorf("%s is refused", rel)
		}
	}
	// A data folder that is the home folder itself closes its secret files, not the home.
	same := access(home, home)
	if same(write("machine.json")) || !same(filepath.Join(home, "work/report.md")) {
		t.Error("a data folder that is the home folder")
	}
}
