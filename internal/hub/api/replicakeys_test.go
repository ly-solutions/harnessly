package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/config"
	"github.com/ly-solutions/harnessly/internal/hub/sshinstall"
)

// A replica never seals or signs with a key of its own: without HUB_SECRET_KEY it needs the key file there.
func TestReplicaKeyCheck(t *testing.T) {
	dir := t.TempDir()
	if err := replicaKeyCheck(&config.Config{DataDir: dir}); err == nil {
		t.Fatal("a replica without HUB_SECRET_KEY and without the key file must not start")
	}
	if err := replicaKeyCheck(&config.Config{DataDir: dir, SecretKey: "keykeykeykeykeykeykeykeykeykeykeykey"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sshinstall.SecretKeyFile), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replicaKeyCheck(&config.Config{DataDir: dir}); err != nil {
		t.Fatal(err)
	}
}

func TestReplicaAppTokenKeyShared(t *testing.T) {
	shared := &sharedKey{secret: "keykeykeykeykeykeykeykeykeykeykeykey"}
	a, err := loadAppTokenKey(filepath.Join(t.TempDir(), "app-token.pem"), shared)
	if err != nil {
		t.Fatal(err)
	}
	pathB := filepath.Join(t.TempDir(), "app-token.pem")
	b, err := loadAppTokenKey(pathB, shared)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Equal(b) {
		t.Fatal("two replicas with one HUB_SECRET_KEY must sign with one key")
	}
	if _, err := os.Stat(pathB); !os.IsNotExist(err) {
		t.Fatal("a replica must not write a key of its own")
	}
	if _, err := loadAppTokenKey(filepath.Join(t.TempDir(), "app-token.pem"), &sharedKey{}); err == nil {
		t.Fatal("a replica without the file and without HUB_SECRET_KEY must not make a key")
	}
}
