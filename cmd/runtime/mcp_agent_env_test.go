package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The agent's environment is what its login shell exports, read the way a pane starts it.
func TestAgentEnvIsTheLoginShells(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte("echo noise\nexport BRIDGE_PORT=18376\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newAgentEnv(home)
	e.shell = "/bin/sh"
	t.Setenv("BRIDGE_PORT", "1") // the runtime's own value is not the agent's
	if v, ok := e.Getenv("BRIDGE_PORT"); !ok || v != "18376" {
		t.Fatalf("got %q %v", v, ok)
	}
	if _, ok := e.Getenv("NOT_EXPORTED_ANYWHERE"); ok {
		t.Fatal("a variable the login shell does not have")
	}
	// A shell that does not answer: the runtime's own environment stands in.
	broken := newAgentEnv(t.TempDir())
	broken.run = func(context.Context, string, []string) ([]byte, error) { return nil, os.ErrNotExist }
	if v, ok := broken.Getenv("BRIDGE_PORT"); !ok || v != "1" {
		t.Fatalf("fallback %q %v", v, ok)
	}
}

func TestParseEnvDump(t *testing.T) {
	vals, ok := parseEnvDump([]byte("motd\x00x\n" + agentEnvMark + "A=1\x00B=x=y\x00"))
	if !ok || vals["A"] != "1" || vals["B"] != "x=y" || len(vals) != 2 {
		t.Fatal(vals, ok)
	}
	if _, ok := parseEnvDump([]byte("no mark")); ok {
		t.Fatal("no mark")
	}
}
