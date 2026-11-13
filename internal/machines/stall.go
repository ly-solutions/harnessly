package machines

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// A stalled machine: its load is high while its processors stand idle, so the tasks counted in the load do not compute,
// they wait for a disk or for the network (a network folder whose server went away holds every program that touches
// it, and the load climbs into the hundreds on an idle CPU). It is told apart from a busy CPU, because the cure is
// another one. Everything here is read from /proc: no program is started and no path of a mount is touched, for a
// look at a dead mount would hang the reader too.

// stallLevel is the level of a load per core that the processors do not explain. known: the busy share is measured.
func stallLevel(perCore, busyPct float64, known bool, prev wire.HealthLevel) wire.HealthLevel {
	t := HealthLimits
	if !known {
		return prev
	}
	over := func(line float64, holding bool) bool {
		if holding {
			return perCore >= line*t.stallExitFactor && busyPct < t.stallBusyExitPct
		}
		return perCore >= line && busyPct < t.stallBusyPct
	}
	switch {
	case over(t.stallCriticalPerCore, prev == wire.HealthCritical):
		return wire.HealthCritical
	case over(t.stallWarnPerCore, held(prev, wire.HealthWarn)):
		return wire.HealthWarn
	}
	return wire.HealthOK
}

// cpuSince is the busy share over the newest stretch at least d long; false until the history is that long.
func (h *HealthMonitor) cpuSince(now time.Time, d time.Duration, ticks cpuTicks) (float64, bool) {
	edge := now.Add(-d)
	for i := len(h.cpu) - 1; i >= 0; i-- {
		if !h.cpu[i].at.After(edge) {
			return cpuPctFromDelta(h.cpu[i].ticks, ticks), true
		}
	}
	return 0, false
}

// netMount is a network mount's requests sent and answers received since it was mounted.
type netMount struct {
	point        string
	sends, recvs float64
}

// unescapeMount turns the octal escapes of a mount line (\040 is a space) back into the characters.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseMountstats reads /proc/self/mountstats: the NFS mounts with their transports' counters, summed per mount.
func parseMountstats(text string) []netMount {
	var out []netMount
	at := -1
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "device "); ok {
			at = -1
			_, after, ok := strings.Cut(rest, " mounted on ")
			if !ok {
				continue
			}
			point, fstype, ok := strings.Cut(after, " with fstype ")
			if !ok || !strings.HasPrefix(fstype, "nfs") {
				continue
			}
			out = append(out, netMount{point: unescapeMount(point)})
			at = len(out) - 1
			continue
		}
		if at < 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "xprt:" {
			continue
		}
		// tcp: port bind connects connectTime idle sends recvs …; udp: port bind sends recvs ….
		sends, recvs := -1, -1
		switch f[1] {
		case "tcp":
			sends, recvs = 7, 8
		case "udp":
			sends, recvs = 4, 5
		}
		if sends < 0 || len(f) <= recvs {
			continue
		}
		s, r := jsNumber(f[sends]), jsNumber(f[recvs])
		if finiteAll(s, r) {
			out[at].sends += s
			out[at].recvs += r
		}
	}
	return out
}

// mountSeen is a mount's counters when its server last answered.
type mountSeen struct {
	recvs   float64
	sent    float64
	replyAt time.Time
}

// silentMounts are the network mounts that were asked since their server last answered and have heard nothing for
// mountSilentAfter. A mount nobody asks is not silent: it is idle.
func (h *HealthMonitor) silentMounts(now time.Time) []string {
	raw, ok := h.read("self/mountstats")
	if !ok {
		h.mounts = nil
		return nil
	}
	seen := map[string]*mountSeen{}
	var silent []string
	for _, m := range parseMountstats(raw) {
		was := h.mounts[m.point]
		if was == nil || m.recvs != was.recvs || m.sends < was.sent {
			seen[m.point] = &mountSeen{recvs: m.recvs, sent: m.sends, replyAt: now}
			continue
		}
		seen[m.point] = was
		if m.sends > was.sent && now.Sub(was.replyAt) >= HealthLimits.mountSilentAfter {
			silent = append(silent, m.point)
		}
	}
	h.mounts = seen
	sort.Strings(silent)
	return silent
}

// waitingMost is how many programs' names a stall carries.
const waitingMost = 3

// waitingScanMost bounds a look at the processes: a machine with more of them is told by its first ones.
const waitingScanMost = 8192

// waitingNames are the programs with the most tasks in uninterruptible sleep (state D of /proc/<pid>/stat), most
// first. Only stat is read: a command line of a hung process may hang the reader.
func (h *HealthMonitor) waitingNames() []string {
	entries, err := os.ReadDir(h.d.ProcRoot)
	if err != nil {
		return nil
	}
	count := map[string]int{}
	scanned := 0
	for _, e := range entries {
		if !isDigits(e.Name()) {
			continue
		}
		if scanned++; scanned > waitingScanMost {
			break
		}
		raw, err := os.ReadFile(filepath.Join(h.d.ProcRoot, e.Name(), "stat"))
		if err != nil {
			continue
		}
		text := string(raw)
		open, closing := strings.IndexByte(text, '('), strings.LastIndexByte(text, ')')
		if open < 0 || closing < open || closing+2 >= len(text) || text[closing+2] != 'D' {
			continue
		}
		count[text[open+1:closing]]++
	}
	names := make([]string, 0, len(count))
	for name := range count {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if count[names[i]] != count[names[j]] {
			return count[names[i]] > count[names[j]]
		}
		return names[i] < names[j]
	})
	return names[:min(waitingMost, len(names))]
}

// stall samples the load against the busy share and answers the level its figures point to; while it is not ok it
// keeps the silent mounts and, looked at every waitingEvery, the programs that wait.
func (h *HealthMonitor) stall(now time.Time, ticks cpuTicks) {
	silent := h.silentMounts(now)
	raw, ok := h.read("loadavg")
	if !ok {
		return
	}
	load1, ok := parseLoadavg(raw)
	if !ok || h.d.Cores <= 0 {
		return
	}
	busy, known := h.cpuSince(now, HealthLimits.stallWindow, ticks)
	level := stallLevel(load1/float64(h.d.Cores), busy, known, h.levels[wire.HealthIO])
	h.levels[wire.HealthIO] = level
	if level == wire.HealthOK {
		// What was known of the stall stays while its level is still said (verdict forgets it with the level).
		return
	}
	if h.stalled == nil || now.Sub(h.waitingAt) >= HealthLimits.waitingEvery {
		h.waiting, h.waitingAt = h.waitingNames(), now
	}
	h.stalled = &wire.HealthStall{Mounts: append([]string{}, silent...), Waiting: append([]string{}, h.waiting...)}
}
