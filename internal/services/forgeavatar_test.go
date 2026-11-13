package services

import "testing"

func TestAvatarLink(t *testing.T) {
	const route = "/api/platform/link-meta/forge-avatar"
	if got, want := AvatarLink(route, "https://gitlab.example.com/uploads/-/system/user/avatar/7/a.png"),
		route+"?src=https%3A%2F%2Fgitlab.example.com%2Fuploads%2F-%2Fsystem%2Fuser%2Favatar%2F7%2Fa.png"; got != want {
		t.Fatalf("link = %q, want %q", got, want)
	}
	for _, src := range []string{"", "http://gitlab.example.com/uploads/a.png", "/uploads/a.png", "javascript:alert(1)"} {
		if got := AvatarLink(route, src); got != "" {
			t.Errorf("AvatarLink(%q) = %q, want none", src, got)
		}
	}
	if got := AvatarLink("", "https://gitlab.example.com/uploads/a.png"); got != "" {
		t.Errorf("no route: %q", got)
	}
}

func TestForgeAvatarHeader(t *testing.T) {
	t.Setenv("GITLAB_SELF_TOKEN", "self-secret")
	t.Setenv("GITLAB_TOKEN", "public-secret")
	s := New(Config{Home: t.TempDir(), GitLabHosts: []string{"gitlab.example.com"}, GitHubHost: "ghe.example.com"})
	extra := []string{"git.own.example"}
	cases := []struct {
		src   string
		ok    bool
		token bool
	}{
		{"https://gitlab.example.com/uploads/-/system/user/avatar/7/a.png", true, true},
		// The token goes only to the deployment's GitLab and only for an avatar it keeps.
		{"https://gitlab.example.com/uploads/other/a.png", true, false},
		{"https://gitlab.com/uploads/-/system/user/avatar/7/a.png", true, false},
		{"https://avatars.githubusercontent.com/u/1?v=4", true, false},
		{"https://secure.gravatar.com/avatar/abc?s=80", true, false},
		{"https://ghe.example.com/avatars/u/3", true, false},
		{"https://git.own.example/uploads/-/system/user/avatar/2/a.png", true, false},
		// Only the paths avatars live at, and never another host, port, scheme or a name with credentials.
		{"https://gitlab.example.com/api/v4/user", false, false},
		{"https://git.own.example/admin", false, false},
		{"https://evil.example.com/uploads/a.png", false, false},
		{"https://gitlab.example.com:8443/uploads/a.png", false, false},
		{"http://gitlab.example.com/uploads/a.png", false, false},
		{"https://user:pw@gitlab.example.com/uploads/a.png", false, false},
		{"https://169.254.169.254/uploads/a.png", false, false},
	}
	for _, c := range cases {
		u, ok := parseWebURL(c.src, nil)
		if !ok {
			t.Fatalf("%s does not parse", c.src)
		}
		header, ok := s.forgeAvatarHeader(u, extra)
		if ok != c.ok {
			t.Errorf("%s: served = %v, want %v", c.src, ok, c.ok)
			continue
		}
		if got := header.Get("PRIVATE-TOKEN") != ""; got != c.token {
			t.Errorf("%s: token sent = %v, want %v", c.src, got, c.token)
		}
	}
}
