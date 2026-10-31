package machine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func execed(pid int) bool {
	args, ok := ProcessArgs(pid)
	return ok && strings.HasPrefix(args, "sleep ")
}

func TestChildPIDs(t *testing.T) {
	sh := exec.Command("sh", "-c", "sleep 30 & sleep 31 & wait")
	if err := sh.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sh.Process.Kill(); _ = sh.Wait() }()
	// A child is listed as soon as it forks, still as a copy of the shell; wait until both have exec-ed sleep.
	var kids []int
	for i := 0; i < 250; i++ {
		time.Sleep(20 * time.Millisecond)
		kids = ChildPIDs(sh.Process.Pid)
		if len(kids) == 2 && execed(kids[0]) && execed(kids[1]) {
			break
		}
	}
	if len(kids) != 2 || kids[0] >= kids[1] {
		t.Fatalf("children %v", kids)
	}
	if args, ok := ProcessArgs(kids[0]); !ok || args != "sleep 30" && args != "sleep 31" {
		t.Fatalf("args %q", args)
	}
	if ProcessCwd(kids[0]) == "" {
		t.Fatal("cwd")
	}
	if got := ChildPIDs(1 << 30); got == nil || len(got) != 0 {
		t.Fatal("a missing process has no children")
	}
}

func TestValidateDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	_ = os.WriteFile(file, nil, 0o600)
	enc := func(c DirCheck) string { b, _ := json.Marshal(c); return string(b) }
	for in, want := range map[string]string{
		dir + "/":        `{"valid":true,"suggestedName":"` + filepath.Base(dir) + `"}`,
		"/":              `{"valid":true,"suggestedName":""}`,
		file:             `{"valid":false,"error":"Please enter a directory path, not a file"}`,
		dir + "/missing": `{"valid":false,"error":"Directory does not exist"}`,
		dir + "\x00":     `{"valid":false,"error":"Directory does not exist"}`,
	} {
		if got := enc(ValidateDirectory(in)); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
}
