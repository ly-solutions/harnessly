package automations

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/ratelimit"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// A person's incoming webhook (docs/connectors.md, «Incoming webhook»): POST <automations>/hook/m/<machine>/<connection>
// [/<secret>]. The hub keeps nothing of the request: it passes the body and the headers that prove where it came from
// over the machine's link (connectors.webhook), where the runtime checks the signature by the connection's secret from
// the machine's safe, keeps the body for the agent and makes the event without text that comes back to the rules as
// automations.event. Here: a size limit, a rate limit per address, and answers without details. The body and the path
// are never logged; the hub holds them only for the time of the call.

const (
	webhookMaxBody = 1 << 20
	webhookTimeout = 15 * time.Second
	// webhookRate requests a minute per address, webhookBurst at once.
	webhookRate  = 60
	webhookBurst = 30
	// The same for one sender's address, whatever it names: GitHub or Sentry call many people's webhooks from a few
	// addresses, so it is well above one webhook's own.
	webhookSenderRate  = 600
	webhookSenderBurst = 300
	// limiterKeys is the most buckets a limiter keeps.
	limiterKeys = 10000
)

var (
	webhookMachineRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	webhookConnRe    = regexp.MustCompile(`^cx[0-9a-f]{12}$`)
	webhookSecretRe  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// webhookHeaders are passed to the machine: what the presets read and check. Nothing else of the request goes.
var webhookHeaders = []string{"Content-Type", "User-Agent", "Authorization",
	"X-Hub-Signature-256", "X-GitHub-Event", "X-GitHub-Delivery",
	"X-Gitlab-Token", "X-Gitlab-Event", "X-Gitlab-Event-UUID", "Idempotency-Key",
	"X-Grafana-Alerting-Signature", "X-Grafana-Alerting-Signature-Timestamp",
	"Sentry-Hook-Signature", "Sentry-Hook-Timestamp", "Sentry-Hook-Resource", "Request-ID",
	"X-Signature-256", "X-Signature", "X-Webhook-Signature", "X-Request-Id", "X-Delivery-Id"}

// webhookStatuses are the answers a sender may get; anything else from a machine is 503.
var webhookStatuses = map[int]bool{http.StatusOK: true, http.StatusAccepted: true, http.StatusBadRequest: true,
	http.StatusUnauthorized: true, http.StatusRequestEntityTooLarge: true, http.StatusServiceUnavailable: true}

type bucket struct {
	tokens float64
	at     time.Time
}

// limiter is a token bucket per key, in memory: a flood of one webhook or from one sender does not reach a machine.
type limiter struct {
	mu    sync.Mutex
	m     map[string]*bucket
	swept time.Time
}

var (
	webhookLimits  = &limiter{m: map[string]*bucket{}}
	webhookSenders = &limiter{m: map[string]*bucket{}}
)

func (l *limiter) allow(key string, now time.Time, rate, burst float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.m[key]
	if b == nil {
		l.makeRoom(now)
		b = &bucket{tokens: burst, at: now}
		l.m[key] = b
	}
	b.tokens = min(burst, b.tokens+now.Sub(b.at).Minutes()*rate)
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// makeRoom keeps the map under its ceiling: idle buckets go, looked for once a minute at most, and when none is idle
// any one does.
func (l *limiter) makeRoom(now time.Time) {
	if len(l.m) < limiterKeys {
		return
	}
	if now.Sub(l.swept) >= time.Minute {
		l.swept = now
		for k, b := range l.m {
			if now.Sub(b.at) > 10*time.Minute {
				delete(l.m, k)
			}
		}
	}
	for k := range l.m {
		if len(l.m) < limiterKeys {
			return
		}
		delete(l.m, k)
	}
}

func webhookAnswer(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	switch status {
	case http.StatusOK, http.StatusAccepted:
		_, _ = w.Write([]byte(`{"ok":true}`))
	default:
		_, _ = w.Write([]byte(`{"ok":false}`))
	}
}

func webhookTooMany(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	webhookAnswer(w, http.StatusTooManyRequests)
}

// webhook takes a request to a person's webhook address and passes it to the machine.
func (s *Service) webhook(w http.ResponseWriter, r *http.Request) {
	machine, conn, secret := r.PathValue("machine"), r.PathValue("conn"), r.PathValue("secret")
	if !webhookMachineRe.MatchString(machine) || !webhookConnRe.MatchString(conn) || (secret != "" && !webhookSecretRe.MatchString(secret)) {
		webhookAnswer(w, http.StatusUnauthorized)
		return
	}
	// Anybody calls this route: the sender's address is limited first, and an address of a webhook costs memory and a
	// body is read only for a machine the hub has.
	sender := ratelimit.IPKey(httpx.ClientIP(r, s.cfg.TrustedProxies))
	if !webhookSenders.allow(sender, s.now(), webhookSenderRate, webhookSenderBurst) {
		webhookTooMany(w)
		return
	}
	m, err := s.db.S().MachineByID(r.Context(), machine)
	if err != nil || m.RevokedAt != nil {
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.log.Warn("webhook machine not read", "error", err)
		}
		// The same answer as for a machine that is away: the route does not say which machines exist.
		webhookAnswer(w, http.StatusServiceUnavailable)
		return
	}
	if !webhookLimits.allow(machine+"/"+conn, s.now(), webhookRate, webhookBurst) {
		webhookTooMany(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookMaxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			webhookAnswer(w, http.StatusRequestEntityTooLarge)
			return
		}
		webhookAnswer(w, http.StatusBadRequest)
		return
	}
	if s.links == nil {
		webhookAnswer(w, http.StatusServiceUnavailable)
		return
	}
	header := map[string]string{}
	// The gateway's sign-in assertion signs its bearer in to the hub: it is no proof of a webhook's sender.
	assertion := ""
	if s.cfg.ProxyJWT.On() && strings.EqualFold(r.Host, s.cfg.ProxyJWT.Host) {
		assertion = s.cfg.ProxyJWT.Header
	}
	for _, name := range webhookHeaders {
		if v := r.Header.Get(name); v != "" && len(v) <= 2048 && !strings.EqualFold(name, assertion) {
			header[name] = v
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), webhookTimeout)
	defer cancel()
	raw, err := s.links.Do(ctx, machine, hubproto.OpConnectorsWebhook,
		map[string]any{"connection": conn, "path": secret, "header": header, "body": body}, webhookTimeout)
	if err != nil {
		// The machine is away or refused: the sender tries again later. The reason stays out of the answer.
		s.log.Info("webhook not passed to the machine", "machine", machine, "connection", conn, "error", err.Error())
		webhookAnswer(w, http.StatusServiceUnavailable)
		return
	}
	var out struct {
		Status int `json:"status"`
	}
	if json.Unmarshal(raw, &out) != nil || !webhookStatuses[out.Status] {
		out.Status = http.StatusServiceUnavailable
	}
	s.log.Info("webhook passed to the machine", "machine", machine, "connection", conn, "status", out.Status)
	webhookAnswer(w, out.Status)
}
