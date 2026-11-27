package designaccess

import "github.com/ly-solutions/harnessly/internal/hub/linkseal"

// A guest link's address is kept sealed (linkseal), so whoever runs the access can copy it again; only its hash opens
// the link. A hub whose key changed simply cannot open the older addresses: those links are reissued.

const sealTable = "design_share_links"

type sealer struct{ s *linkseal.Sealer }

// newSealer derives the links' key from the hub's master key; nil without one.
func newSealer(master []byte) *sealer {
	s := linkseal.New(master)
	if s == nil {
		return nil
	}
	return &sealer{s: s}
}

// seal is the token sealed for one link; "" without a key.
func (s *sealer) seal(linkID, token string) string {
	if s == nil {
		return ""
	}
	return s.s.Seal(sealTable, linkID, token)
}

// open is the token back; false for another key, another link or a damaged value.
func (s *sealer) open(linkID, sealed string) (string, bool) {
	if s == nil {
		return "", false
	}
	return s.s.Open(sealTable, linkID, sealed)
}
