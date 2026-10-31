package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/ly-solutions/harnessly/internal/browser"
)

func TestBrowserSocketProxiesThePersistentBrowser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer ws.CloseNow()
		_, data, err := ws.Read(r.Context())
		if err == nil {
			_ = ws.Write(r.Context(), websocket.MessageBinary, append([]byte("echo:"), data...))
		}
	}))
	defer upstream.Close()
	port, err := strconv.Atoi(strings.TrimPrefix(upstream.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatal(err)
	}
	handler := browserSocket(func(context.Context) (int, error) { return port, nil })
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	if err := ws.Write(ctx, websocket.MessageBinary, []byte("rfb")); err != nil {
		t.Fatal(err)
	}
	_, got, err := ws.Read(ctx)
	if err != nil || string(got) != "echo:rfb" {
		t.Fatalf("response %q, %v", got, err)
	}
}

func TestBrowserSocketReportsStartupFailure(t *testing.T) {
	server := httptest.NewServer(browserSocket(func(context.Context) (int, error) {
		return 0, errors.New("Xvfb is missing")
	}))
	defer server.Close()
	res, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestBrowserSocketServesRealRFB(t *testing.T) {
	if os.Getenv("HARNESSLY_BROWSER_INTEGRATION") != "1" {
		t.Skip("set HARNESSLY_BROWSER_INTEGRATION=1 to run the real Chrome/Xvfb stack")
	}
	service := browser.New(browser.Config{Home: os.Getenv("HOME"), DataDir: t.TempDir()})
	defer service.Close()
	server := httptest.NewServer(browserSocket(service.Endpoint))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	_, greeting, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(greeting) != "RFB 003.008\n" {
		t.Fatalf("RFB greeting = %q", greeting)
	}
}
