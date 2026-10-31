package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/loginflow"
	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// loginEnvNames are the variables a CLI that signs in starts with: the person's own place, language and network route.
// Nothing else of the runtime's environment (its tokens, the services' keys) goes to it.
var loginEnvNames = []string{"USER", "LOGNAME", "LANG", "LC_ALL", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "HTTPS_PROXY", "https_proxy",
	"HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy", "ALL_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS", "CODEX_HOME", "CLAUDE_CONFIG_DIR"}

// proxyEnvNames are the variables the proxy of a profile starts with besides HOME and PATH: those of a CLI that signs in,
// and the proxy's own folder.
var proxyEnvNames = slices.Concat(loginEnvNames, []string{"CCP_CONFIG_DIR"})

// personEnv is an environment of the person's own: HOME, PATH, and those of names the runtime has.
func personEnv(home, path string, names []string) []string {
	env := []string{"HOME=" + home, "PATH=" + path}
	for _, n := range names {
		if v, ok := os.LookupEnv(n); ok {
			env = append(env, n+"="+v)
		}
	}
	return env
}

// v2Logins builds the sign-in service over the machine's CLIs and the profiles' proxies. path is the PATH the person's
// login shell builds; changed is told of a CLI's sign-in, profileChanged of a profile's.
func v2Logins(home string, path func() string, changed func(wire.Provider), profileChanged func(id string)) *loginflow.Service {
	return loginflow.New(loginflow.Config{
		Bin: func(p wire.Provider) (string, error) {
			name := string(p)
			if name != "claude" && name != "codex" {
				return "", errors.New("no such CLI")
			}
			for _, dir := range filepath.SplitList(path()) {
				if dir == "" {
					continue
				}
				bin := filepath.Join(dir, name)
				if st, err := os.Stat(bin); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
					return bin, nil
				}
			}
			return exec.LookPath(name)
		},
		Env: func() []string {
			return append(personEnv(home, path(), proxyEnvNames), "TERM=xterm-256color",
				// No browser opens on the machine: the person opens the link on their own screen.
				"BROWSER=/bin/false",
				// The person's tmux shim wraps a CLI that has a terminal; this one is driven by the runtime.
				"JARVIS_TMUX=off")
		},
		OnChange:        changed,
		ProfileBin:      func(string) (string, error) { return providers.FindProxy(path(), home) },
		OnProfileChange: profileChanged,
	})
}

// cachedPath asks the login shell for PATH at most once a minute: it takes seconds on a loaded machine.
func cachedPath(ask func() string) func() string {
	var mu sync.Mutex
	var at time.Time
	var path string
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		if path == "" || time.Since(at) > time.Minute {
			path, at = strings.TrimSpace(ask()), time.Now()
		}
		return path
	}
}
