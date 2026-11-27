package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/settings"
)

// The setup is open until a password is chosen: a page of another site in the owner's browser must not choose it.
func TestSetupRefusesStrangers(t *testing.T) {
	t.Setenv("RUNTIME_GATE_LISTEN", "127.0.0.1:8022")
	t.Setenv("AUTH_PASSWORD", "")
	t.Setenv("INIT_PASSWORD", "")
	t.Setenv("RUNTIME_GENERATE_INIT_PASSWORD", "")
	data := t.TempDir()
	cfgFile := filepath.Join(data, "config.json")
	d := deps{DataDir: data, ConfigFile: cfgFile, Cookie: "session-token", SignSession: func() string { return "a.b.c" },
		SessionOK: func(*http.Request) bool { return false }}
	routes := authRoutes(d, &settings.Store{DataDir: data, ConfigFile: cfgFile}, func(h settings.Handler) func(*http.Request) (int, any, http.Header) { return settings.Guarded(nil, h) })
	post := func(host string, h map[string]string, body string) int {
		r := httptest.NewRequest("POST", "http://"+host+"/api/auth/setup", strings.NewReader(body))
		for k, v := range h {
			r.Header.Set(k, v)
		}
		for _, route := range routes {
			if route.Match(r) {
				status, _, _ := route.Compute(r)
				return status
			}
		}
		t.Fatal("no route")
		return 0
	}
	good, _ := json.Marshal(map[string]string{"authPassword": "mine-mine"})
	for name, status := range map[string]int{
		"a form of another site": post("localhost:8022", map[string]string{"Content-Type": "application/x-www-form-urlencoded",
			"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}, "authPassword=theirs&dangerouslySkipPermissions=1"),
		"a form without browser headers": post("localhost:8022", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "authPassword=theirs"),
		"another local port":             post("localhost:8022", map[string]string{"Content-Type": "application/json", "Origin": "http://localhost:3000", "Sec-Fetch-Site": "same-site"}, string(good)),
		"a rebound name": post("evil.example:8022", map[string]string{"Content-Type": "application/json",
			"Origin": "http://evil.example:8022", "Sec-Fetch-Site": "same-origin"}, string(good)),
	} {
		if status != http.StatusForbidden && status != http.StatusUnsupportedMediaType {
			t.Errorf("%s: %d, want a refusal", name, status)
		}
	}
	if _, err := os.Stat(cfgFile); err == nil {
		t.Fatal("a refused setup wrote the config")
	}
	own := map[string]string{"Content-Type": "application/json", "Origin": "http://localhost:8022", "Sec-Fetch-Site": "same-origin"}
	if status := post("localhost:8022", own, string(good)); status != http.StatusOK {
		t.Errorf("the owner's page: %d", status)
	}
}
