package apps

import "testing"

func TestDecide(t *testing.T) {
	cashback := &Manifest{ID: "acme.cashback", Agent: &Agent{Tools: &ToolPolicy{
		Allow:   []string{"bank-cashback.*", "app.*", "Read"},
		Deny:    []string{"Bash", "WebFetch", "mcp:*"},
		Confirm: []string{"bank-cashback.create"},
	}}}
	review := &Manifest{ID: "acme.mr-review", Agent: &Agent{Tools: &ToolPolicy{
		Deny: []string{"Edit", "Write", "Bash(git push*)"},
	}}}
	bash := func(cmd string) Call { return Call{Tool: "Bash", Input: map[string]any{"command": cmd}} }

	cases := []struct {
		name string
		apps []*Manifest
		call Call
		want Decision
		app  string
	}{
		{"own server allowed over mcp:*", []*Manifest{cashback}, Call{Tool: "mcp__plugin_acme-cashback_bank-cashback__list"}, DecideAllow, "acme.cashback"},
		{"own server by bare name", []*Manifest{cashback}, Call{Tool: "mcp__bank-cashback__list"}, DecideAllow, "acme.cashback"},
		{"exact confirm beats the mask", []*Manifest{cashback}, Call{Tool: "mcp__plugin_acme-cashback_bank-cashback__create"}, DecideConfirm, "acme.cashback"},
		{"other MCP denied", []*Manifest{cashback}, Call{Tool: "mcp__github__create_issue"}, DecideDeny, "acme.cashback"},
		{"host tools", []*Manifest{cashback}, Call{Tool: "mcp__harnessly-app__flow_go"}, DecideAllow, "acme.cashback"},
		{"built-in denied", []*Manifest{cashback}, bash("ls"), DecideDeny, "acme.cashback"},
		{"built-in allowed", []*Manifest{cashback}, Call{Tool: "Read"}, DecideAllow, "acme.cashback"},
		{"no rule", []*Manifest{review}, Call{Tool: "Read"}, DecideNone, ""},
		{"bash rule matches", []*Manifest{review}, bash("git push origin main"), DecideDeny, "acme.mr-review"},
		{"bash rule misses", []*Manifest{review}, bash("git status"), DecideNone, ""},
		{"claude's colon form", []*Manifest{{ID: "x.y", Agent: &Agent{Tools: &ToolPolicy{Deny: []string{"Bash(rm:*)"}}}}}, bash("rm -rf /"), DecideDeny, "x.y"},
		{"allow of one app", []*Manifest{cashback, review}, Call{Tool: "Read"}, DecideAllow, "acme.cashback"},
		{"deny of the other app", []*Manifest{cashback, review}, Call{Tool: "Edit"}, DecideDeny, "acme.mr-review"},
		{"no apps", nil, bash("ls"), DecideNone, ""},
		{"push after another command", []*Manifest{review}, bash("git status && git push"), DecideDeny, "acme.mr-review"},
		{"push with a path and a variable", []*Manifest{review}, bash("GIT_TRACE=1 /usr/bin/git push"), DecideDeny, "acme.mr-review"},
		{"push in a subshell", []*Manifest{review}, bash("(cd x; git push)"), DecideDeny, "acme.mr-review"},
		{"a line it cannot read", []*Manifest{review}, bash("eval \"$CMD\""), DecideConfirm, "acme.mr-review"},
		{"a wrapper's flags", []*Manifest{review}, bash("nice -n 10 git push"), DecideConfirm, "acme.mr-review"},
		{"a wrapper's arguments", []*Manifest{review}, bash("timeout 5 git push"), DecideConfirm, "acme.mr-review"},
		{"a wrapper by its path", []*Manifest{review}, bash("/usr/bin/timeout 5 git push"), DecideConfirm, "acme.mr-review"},
		{"a program under a variable's folder", []*Manifest{review}, bash("$D/git push"), DecideDeny, "acme.mr-review"},
		{"a program in a variable", []*Manifest{review}, bash("G=git; $G push"), DecideConfirm, "acme.mr-review"},
		{"a quoted program", []*Manifest{review}, bash(`"git" push`), DecideConfirm, "acme.mr-review"},
		{"command substitution", []*Manifest{review}, bash("echo $(git push)"), DecideDeny, "acme.mr-review"},
		{"another plugin's server", []*Manifest{cashback}, Call{Tool: "mcp__plugin_evil-app_bank-cashback__list"}, DecideDeny, "acme.cashback"},
		{"host tools only unprefixed", []*Manifest{cashback}, Call{Tool: "mcp__plugin_evil-app_harnessly-app__flow_go"}, DecideDeny, "acme.cashback"},
	}
	for _, c := range cases {
		got := Decide(c.apps, c.call)
		if got.Decision != c.want || got.App != c.app {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

func TestGlob(t *testing.T) {
	for _, c := range []struct {
		p, s string
		want bool
	}{
		{"git push*", "git push --force", true},
		{"git push*", "git pull", false},
		{"*secret*", "cat my-secret.txt", true},
		{"a*b*c", "a-x-b-y-c", true},
		{"a*b*c", "a-x-c", false},
		{"exact", "exact", true},
	} {
		if got := glob(c.p, c.s); got != c.want {
			t.Errorf("glob(%q, %q) = %v", c.p, c.s, got)
		}
	}
}

func TestSnapshotKeepsTheAppsOn(t *testing.T) {
	s := Snapshot{Apps: []SnapshotApp{{ID: "a.b", Name: "AB"}}, Protected: []string{"/data/apps", "/home/me/dev-app"}}
	for _, c := range []struct {
		call Call
		want Decision
	}{
		{Call{Tool: "Bash", Input: map[string]any{"command": "harnessly app disable a.b"}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": "ls; harnessly app remove a.b"}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": "harnessly app list"}}, DecideNone},
		{Call{Tool: "Bash", Input: map[string]any{"command": "harnessly app off a.b"}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": `harnessly -w ws1 apps "of"f a.b`}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": `"harness"ly ap\p disable a.b`}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": "sh -c 'harnessly app off a.b'"}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": "/usr/bin/setsid /opt/bin/harnessly app off a.b"}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": "harnessly app $'\\x6f'ff a.b"}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": "echo x > /data/apps/packages/a.b/harnessly/app.json"}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": "harnessly --json app rm a.b"}}, DecideDeny},
		{Call{Tool: "Bash", Input: map[string]any{"command": "/usr/local/bin/harnessly app uninstall a.b"}}, DecideDeny},
		{Call{Tool: "Edit", Input: map[string]any{"file_path": "/home/me/dev-app/harnessly/app.json"}}, DecideDeny},
		{Call{Tool: "Edit", Input: map[string]any{"file_path": "/home/me/dev-app/../dev-app/x"}}, DecideDeny},
		{Call{Tool: "Edit", Input: map[string]any{"file_path": "/home/me/dev-application/x"}}, DecideNone},
	} {
		if v, _ := s.Decide(c.call); v.Decision != c.want {
			t.Errorf("%+v: %s", c.call, v.Decision)
		}
	}
}
