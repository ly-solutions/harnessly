package main

import (
	"mime"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"

	"github.com/ly-solutions/harnessly/internal/sameorigin"
)

// The first-run setup of a runtime asks for no session: whoever reaches it chooses the password and the permission
// switch. These checks keep a page of another site in the owner's browser from being that one.

// setupRefusal is why a setup request is refused, "" when it may go on. guarded: the runtime asks for its one-time
// password first (init_password.go).
func setupRefusal(r *http.Request, guarded bool) string {
	if sameorigin.CrossSite(r) {
		return "This request comes from a page of another site."
	}
	// A page of another site sends a form without asking; JSON it may send only after a preflight nobody answers.
	if kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || kind != "application/json" {
		return "Send the setup as application/json."
	}
	// A name of the open internet can be pointed at this machine (DNS rebinding), and its page is then of the same
	// origin as the setup. Without the one-time password only the machine's own names and addresses set it up.
	if !guarded && !localName(r.Host) {
		return "Set the runtime up at localhost or by an IP address, or start it with INIT_PASSWORD set."
	}
	return ""
}

// localName says whether host (the request's Host) is one no stranger can point somewhere: an address, localhost, a
// name of a private network, or the machine's own name.
func localName(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if host != "" && !strings.Contains(host, ".") { // localhost, a name of the local network
		return true
	}
	for _, suffix := range []string{".localhost", ".local", ".lan", ".internal", ".home.arpa", ".ts.net"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	own, err := os.Hostname()
	return err == nil && strings.EqualFold(host, own)
}
