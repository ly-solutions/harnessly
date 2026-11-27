package obsidian

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/connectors"
)

func limitOf(args map[string]string, def, most int) int {
	if v, err := strconv.Atoi(args["limit"]); err == nil && v > 0 {
		return min(v, most)
	}
	return def
}

func (vaultConn) Read(_ context.Context, in connectors.Instance, tool string, args map[string]string) (connectors.Result, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	st := store{dir: in.Dir}
	ix, err := st.load()
	if err != nil {
		return connectors.Result{}, err
	}
	from := "obsidian: " + in.Name
	if ix.Vault != "" {
		from = "obsidian: " + ix.Vault
	}
	if ix.PairedAt.IsZero() {
		return connectors.Result{Text: "Obsidian is not paired yet: the person installs the harnessly plugin and opens the pairing link from Settings → Connections"}, nil
	}
	switch tool {
	case "notes":
		folder := strings.Trim(strings.TrimSpace(args["folder"]), "/")
		tag := cleanTag(args["tag"])
		if args["tag"] != "" && tag == "" {
			return connectors.Result{}, fmt.Errorf("%w: a tag like project/alpha", connectors.ErrBadArgs)
		}
		paths := make([]string, 0, len(ix.Notes))
		for p, n := range ix.Notes {
			if folder != "" && p != folder && !strings.HasPrefix(p, folder+"/") {
				continue
			}
			if tag != "" && !hasTag(n.Tags, tag) {
				continue
			}
			paths = append(paths, p)
		}
		sort.Slice(paths, func(i, j int) bool {
			a, b := ix.Notes[paths[i]], ix.Notes[paths[j]]
			if !a.MTime.Equal(b.MTime) {
				return a.MTime.After(b.MTime)
			}
			return paths[i] < paths[j]
		})
		limit := limitOf(args, 100, 1000)
		total := len(paths)
		if len(paths) > limit {
			paths = paths[:limit]
		}
		var b strings.Builder
		for _, p := range paths {
			n := ix.Notes[p]
			fmt.Fprintf(&b, "%s  %s", n.MTime.Format("2006-01-02 15:04"), p)
			for _, t := range n.Tags {
				b.WriteString("  #" + t)
			}
			b.WriteString("\n")
		}
		text := fmt.Sprintf("%d of %d shared notes, newest first; last sync %s", len(paths), total, ix.SyncedAt.Format(time.RFC3339))
		if total == 0 {
			text = fmt.Sprintf("no shared notes match (the vault shares %d)", len(ix.Notes))
		}
		return connectors.Result{Text: text, Data: b.String(), From: from}, nil
	case "note":
		p, n, err := find(ix, args["path"])
		if err != nil {
			return connectors.Result{}, err
		}
		text, err := st.readNote(p)
		if err != nil {
			return connectors.Result{}, connectors.ErrNotFound
		}
		var b strings.Builder
		fmt.Fprintf(&b, "path: %s\nmodified: %s\n", p, n.MTime.Format(time.RFC3339))
		if len(n.Tags) > 0 {
			b.WriteString("tags: #" + strings.Join(n.Tags, " #") + "\n")
		}
		if out := ix.shared(n.Links); len(out) > 0 {
			b.WriteString("links to: " + strings.Join(out, ", ") + "\n")
		}
		if back := ix.backlinks(p); len(back) > 0 {
			b.WriteString("linked from: " + strings.Join(back, ", ") + "\n")
		}
		b.WriteString("\n")
		b.WriteString(text)
		return connectors.Result{Text: "the note as it is in the vault", Data: b.String(), From: from}, nil
	case "links":
		p, n, err := find(ix, args["path"])
		if err != nil {
			return connectors.Result{}, err
		}
		var b strings.Builder
		b.WriteString("links to:\n")
		for _, l := range ix.shared(n.Links) {
			b.WriteString("  " + l + "\n")
		}
		b.WriteString("linked from:\n")
		for _, l := range ix.backlinks(p) {
			b.WriteString("  " + l + "\n")
		}
		return connectors.Result{Text: "links among the shared notes only", Data: b.String(), From: from}, nil
	case "tags":
		count := map[string]int{}
		for _, n := range ix.Notes {
			for _, t := range n.Tags {
				count[t]++
			}
		}
		tags := make([]string, 0, len(count))
		for t := range count {
			tags = append(tags, t)
		}
		sort.Slice(tags, func(i, j int) bool {
			if count[tags[i]] != count[tags[j]] {
				return count[tags[i]] > count[tags[j]]
			}
			return tags[i] < tags[j]
		})
		var b strings.Builder
		for _, t := range tags {
			fmt.Fprintf(&b, "#%s  %d\n", t, count[t])
		}
		return connectors.Result{Text: fmt.Sprintf("%d tags", len(tags)), Data: b.String(), From: from}, nil
	case "search":
		q := strings.ToLower(strings.TrimSpace(args["query"]))
		words := strings.Fields(q)
		if len(words) == 0 {
			return connectors.Result{}, fmt.Errorf("%w: query is required", connectors.ErrBadArgs)
		}
		limit := limitOf(args, 50, 500)
		paths := make([]string, 0, len(ix.Notes))
		for p := range ix.Notes {
			paths = append(paths, p)
		}
		slices.Sort(paths)
		var b strings.Builder
		found := 0
		for _, p := range paths {
			if found >= limit {
				break
			}
			if all(strings.ToLower(p+" "+ix.Notes[p].Title), words) {
				fmt.Fprintf(&b, "%s: (name)\n", p)
				found++
			}
			text, err := st.readNote(p)
			if err != nil {
				continue
			}
			for i, line := range strings.Split(text, "\n") {
				if found >= limit {
					break
				}
				if all(strings.ToLower(line), words) {
					fmt.Fprintf(&b, "%s:%d: %s\n", p, i+1, short(line, 300))
					found++
				}
			}
		}
		if found == 0 {
			return connectors.Result{Text: "nothing found in the shared notes"}, nil
		}
		return connectors.Result{Text: fmt.Sprintf("%d matches", found), Data: b.String(), From: from}, nil
	case "changes":
		since := now().Add(-24 * time.Hour)
		if v := strings.TrimSpace(args["since"]); v != "" {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				since = now().Add(-d)
			} else if t, err := time.Parse(time.RFC3339, v); err == nil {
				since = t
			} else {
				return connectors.Result{}, fmt.Errorf("%w: since is a time or a duration like 24h", connectors.ErrBadArgs)
			}
		}
		limit := limitOf(args, 100, 2000)
		var list []change
		for _, c := range st.changes() {
			if !c.At.Before(since) {
				list = append(list, c)
			}
		}
		if len(list) > limit {
			list = list[len(list)-limit:]
		}
		if len(list) == 0 {
			return connectors.Result{Text: "no changes since " + since.UTC().Format(time.RFC3339)}, nil
		}
		var b strings.Builder
		for _, c := range list {
			fmt.Fprintf(&b, "%s  %-8s  %s\n", c.At.Format(time.RFC3339), c.Change, c.Path)
		}
		return connectors.Result{Text: fmt.Sprintf("%d changes, oldest first", len(list)), Data: b.String(), From: from}, nil
	}
	return connectors.Result{}, connectors.ErrUnknownTool
}

// find is a shared note by its path; a path without «.md» is taken too.
func find(ix *index, raw string) (string, *note, error) {
	p := strings.TrimPrefix(strings.TrimSpace(raw), "/")
	if p == "" {
		return "", nil, fmt.Errorf("%w: path is required", connectors.ErrBadArgs)
	}
	if n := ix.Notes[p]; n != nil {
		return p, n, nil
	}
	if n := ix.Notes[p+".md"]; n != nil {
		return p + ".md", n, nil
	}
	return "", nil, fmt.Errorf("%w: no shared note %s (it may be private in Obsidian)", connectors.ErrNotFound, p)
}

// shared keeps the links to notes the vault shares now: a note may link to one that has since become private.
func (ix *index) shared(links []string) []string {
	var out []string
	for _, l := range links {
		if ix.Notes[l] != nil {
			out = append(out, l)
		}
	}
	return out
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) || strings.HasPrefix(strings.ToLower(t), strings.ToLower(tag)+"/") {
			return true
		}
	}
	return false
}

func all(s string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}
