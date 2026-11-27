package main

// «Этот компьютер»: the person's laptop runs this runtime, linked to a hub. Without a network the hub is out
// of reach, and the runtime is not: its port is on the laptop's loopback. So while the network is there the page asks
// the runtime, through the hub, for a pass (a token of this runtime's secret, for the person, for a month); without it
// the page goes to http://127.0.0.1:<port><base> and hands the pass over, and the runtime opens a page session there.
//
// A pass opens nothing by itself: it is taken only on a loopback connection that names a loopback host (no other
// device, no DNS rebinding), only from a page of that origin (no other site logs the browser in), and it makes a
// session of the same person and rights the hub's session had, never more.

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/sameorigin"
)

const (
	localPassAudience = "local-pass"
	localPassTTL      = 30 * 24 * time.Hour
	localEnterPath    = "/api/auth/local"
)

// localPort is the port of a listen address on this computer's loopback (or every address), 0 for any other.
func localPort(listen string) int {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return 0
	}
	if host != "" && host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !(ip.IsLoopback() || ip.IsUnspecified()) {
			return 0
		}
	}
	return portOf(listen)
}

// loopbackRequest: the connection comes from this computer and names it by a loopback host.
func loopbackRequest(r *http.Request) bool {
	addr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	if ip := net.ParseIP(addr); ip == nil || !ip.IsLoopback() {
		return false
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hubOrigin is the origin of the hub this runtime belongs to ("" without one): the one site that may ask «is this you».
func hubOrigin() string {
	u, err := url.Parse(env("RUNTIME_HUB_URL", ""))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func localRoutes(d deps, listen string) []gate.Route {
	// The pass, asked by the page through the hub with the person's session.
	pass := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			secretJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method-not-allowed"})
			return
		}
		claims := d.Claims(r)
		sub, _ := claims["sub"].(string)
		_, aud := claims["aud"]
		level, _ := claims["level"].(string)
		if sub == "" || aud || level != "" {
			secretJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		port := localPort(listen)
		if port == 0 {
			secretJSON(w, http.StatusNotFound, map[string]string{"error": "no-local", "code": "no-local"})
			return
		}
		owner, _ := claims["verifiedOwner"].(bool)
		token := agent.SignPayload(d.Secret, localPassAudience, map[string]any{"sub": sub, "owner": owner}, localPassTTL)
		secretJSON(w, http.StatusOK, map[string]any{
			"port":      port,
			"base":      appBase() + "/",
			"pass":      token,
			"expiresAt": time.Now().Add(localPassTTL).UnixMilli(),
		})
	}

	// The pass handed over: «ping» answers the hub's page whether this is the computer it runs on; «enter» opens the
	// session on this origin.
	enter := func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		hub := hubOrigin()
		fromHub := hub != "" && origin == hub
		if fromHub {
			w.Header().Set("Access-Control-Allow-Origin", hub)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
			w.Header().Set("Access-Control-Allow-Headers", "content-type")
			w.Header().Set("Access-Control-Allow-Methods", "POST")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST, OPTIONS")
			secretJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method-not-allowed"})
			return
		}
		if !loopbackRequest(r) {
			secretJSON(w, http.StatusForbidden, map[string]string{"error": "not-local"})
			return
		}
		var b struct {
			Pass  string `json:"pass"`
			Enter bool   `json:"enter"`
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
		if err != nil || json.Unmarshal(raw, &b) != nil || b.Pass == "" {
			secretJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid-request"})
			return
		}
		claims, ok := agent.VerifiedAudience(d.Secret, b.Pass, localPassAudience, time.Now())
		sub, _ := claims["sub"].(string)
		if !ok || sub == "" {
			secretJSON(w, http.StatusUnauthorized, map[string]string{"error": "bad-pass"})
			return
		}
		if !b.Enter {
			secretJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		// Only this origin's own page opens a session: another site cannot log the browser in with a pass it holds.
		if fromHub || sameorigin.CrossSite(r) {
			secretJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site"})
			return
		}
		owner, _ := claims["owner"].(bool)
		cookie := d.Cookie + "=" + agent.SignSession(d.Secret, sub, owner, sessionMaxAge) +
			"; HttpOnly; SameSite=Lax; Path=/; Max-Age=" + strconv.Itoa(int(sessionMaxAge/time.Second))
		w.Header().Set("Set-Cookie", cookie)
		secretJSON(w, http.StatusOK, map[string]any{"ok": true})
	}

	return []gate.Route{
		{Match: func(r *http.Request) bool { return pathIs(r, platformAPI+"/local-pass") }, Handler: http.HandlerFunc(pass)},
		{Match: func(r *http.Request) bool { return pathIs(r, localEnterPath) }, Handler: http.HandlerFunc(enter)},
	}
}
