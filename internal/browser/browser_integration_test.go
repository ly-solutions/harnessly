package browser

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestServiceStartsRealBrowserStack(t *testing.T) {
	if os.Getenv("HARNESSLY_BROWSER_INTEGRATION") != "1" {
		t.Skip("set HARNESSLY_BROWSER_INTEGRATION=1 to run the real Chrome/Xvfb stack")
	}
	s := New(Config{Home: os.Getenv("HOME"), DataDir: t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	port, err := s.Endpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
