package services

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/files"
)

// The avatars of a code host's people (the author of a merge request, of a pull request). The page never loads them
// from the host: a GitLab or a GitHub Enterprise of the person's own may be closed by sign-in or by the network, so this
// runtime fetches the picture, with the host's own token where it has one, and serves it from its route.

const (
	maxForgeAvatars = 500
	maxAvatarSource = 2000
)

// publicAvatarHosts serve avatars to anyone: GitHub's own and Gravatar, which GitLab falls back to.
var publicAvatarHosts = map[string]bool{
	"avatars.githubusercontent.com": true,
	"secure.gravatar.com":           true,
	"www.gravatar.com":              true,
	"gravatar.com":                  true,
}

type forgeAvatars struct {
	mu   sync.Mutex
	seen map[string]*avatar
}

// AvatarLink is the address the page loads a code host's avatar by: route with the picture's own address in ?src=.
// Empty for an address that is not an https one.
func AvatarLink(route, src string) string {
	if route == "" || len(src) > maxAvatarSource {
		return ""
	}
	u, ok := parseWebURL(src, nil)
	if !ok || u.Protocol() != "https:" {
		return ""
	}
	return route + "?src=" + files.EncodeURIComponent(u.Href())
}

func (s *Service) forgeAvatarLink(src string) *string {
	if link := AvatarLink(s.cfg.ForgeAvatarPath, src); link != "" {
		return &link
	}
	return nil
}

// glAvatarPrefix is where a GitLab keeps the avatars it serves itself.
const glAvatarPrefix = "/uploads/-/system/"

// forgeAvatarHeader says whether src is an avatar this runtime fetches, and with which header: a public avatar host,
// a GitLab of this deployment, gitlab.com, the deployment's GitHub, or a host of the person's connections (extra), all
// but the first only under the paths avatars live at. A token goes to nobody but a GitLab the deployment names, and
// there only with a request for an avatar it keeps.
func (s *Service) forgeAvatarHeader(u *webURL, extra []string) (http.Header, bool) {
	host := u.Host
	if u.Protocol() != "https:" || u.Username != "" || u.Password != "" || u.Port != "" || host == "" {
		return nil, false
	}
	if publicAvatarHosts[host] {
		return http.Header{}, true
	}
	path := u.Pathname()
	if !strings.HasPrefix(path, "/uploads/") && !strings.HasPrefix(path, "/avatars/") {
		return nil, false
	}
	if s.isGitlabHost(host) {
		header := http.Header{}
		if token := s.gitlabToken(host); token != "" && strings.HasPrefix(path, glAvatarPrefix) {
			header.Set("PRIVATE-TOKEN", token)
		}
		return header, true
	}
	if host == "gitlab.com" || host == s.cfg.GitHubHost {
		return http.Header{}, true
	}
	for _, h := range extra {
		if h == host {
			return http.Header{}, true
		}
	}
	return nil, false
}

// ForgeAvatar is the picture at src, an avatar address a code host gave; nil when it is not one this runtime fetches
// or the host gave no picture. extra are the hosts of the person's code host connections.
func (s *Service) ForgeAvatar(ctx context.Context, src string, extra []string) (*Image, error) {
	if len(src) > maxAvatarSource {
		return nil, nil
	}
	u, ok := parseWebURL(src, nil)
	if !ok {
		return nil, nil
	}
	header, ok := s.forgeAvatarHeader(u, extra)
	if !ok {
		return nil, nil
	}
	key := u.Href()
	st := s.forge
	st.mu.Lock()
	if c := st.seen[key]; c != nil {
		fresh := metaMissFresh
		if c.image != nil {
			fresh = avatarFresh
		}
		if time.Since(c.at) < fresh {
			st.mu.Unlock()
			return c.image, nil
		}
	}
	st.mu.Unlock()
	var image *Image
	resp, err := s.fetch(ctx, key, header, refuse, serviceTimeout, maxAvatar)
	if resp != nil && err == nil && resp.ok() && !resp.tooBig && len(resp.body) > 0 &&
		avatarType.MatchString(resp.header.Get("Content-Type")) {
		image = &Image{Type: resp.header.Get("Content-Type"), Body: resp.body}
	}
	st.mu.Lock()
	if len(st.seen) >= maxForgeAvatars {
		st.seen = map[string]*avatar{}
	}
	st.seen[key] = &avatar{image: image, at: time.Now()}
	st.mu.Unlock()
	return image, nil
}
