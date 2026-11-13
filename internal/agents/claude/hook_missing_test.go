package claude

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// a hook script the CLI cannot see must not block the prompt (sh exits 2 on a missing file).
func TestShScriptMissingFileExitsZero(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.sh")
	if err := exec.Command("sh", "-c", shScript(missing, "claude UserPromptSubmit")).Run(); err != nil {
		t.Fatalf("missing hook script: %v, want exit 0", err)
	}
	if err := exec.Command("sh", "-c", `sh "`+missing+`"`).Run(); err == nil {
		t.Fatal("control: bare sh on a missing file should fail")
	}
}
