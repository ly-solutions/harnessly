// A hash is a name, not a right. A caller that sees only some paths of a room gets contents only through a
// path it sees, or after it has sent those contents itself: knowing a hash (from a log, or from a project it saw once)
// neither reads the file nor puts it into another project.

package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"time"
)

// proofTTL is how long contents a caller has sent stay its own before a commit names them: an upload is committed
// within the same pass, and one that is late sends the contents again.
const proofTTL = 24 * time.Hour

// holder is whose contents and uploads these are: Holder, or Member where the authorizer names nothing finer.
func (a Access) holder() string {
	if a.Holder != "" {
		return a.Holder
	}
	return a.Member
}

// holds tells whether the caller may have the contents with this sha256: it sees the whole room, it sent them, or a
// revision of a path it sees has them.
func (c caller) holds(ctx context.Context, sha string) (bool, error) {
	if c.acc.Sees == nil {
		return true, nil
	}
	if !ValidSha(sha) {
		return false, nil
	}
	ok, err := c.store.meta.Proven(ctx, c.acc.holder(), sha, time.Now().Add(-proofTTL).UnixMilli())
	if err != nil || ok {
		return ok, err
	}
	paths, err := c.store.meta.PathsOf(ctx, sha)
	if err != nil {
		return false, err
	}
	for _, p := range paths {
		if c.acc.sees(p) {
			return true, nil
		}
	}
	return false, nil
}

// prove records that the caller sent the contents, for a caller that does not see the whole room.
func (c caller) prove(ctx context.Context, sha string) error {
	if c.acc.Sees == nil {
		return nil
	}
	now := time.Now()
	return c.store.meta.Prove(ctx, c.acc.holder(), sha, now.UnixMilli(), now.Add(-proofTTL).UnixMilli())
}

// checked reads a body and hashes it: a store that holds the contents already does not look at what is sent again, so
// whoever is to be believed to have them is checked here.
type checked struct {
	r io.Reader
	h hash.Hash
}

func newChecked(r io.Reader) *checked { return &checked{r: r, h: sha256.New()} }

func (c *checked) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.h.Write(p[:n])
	return n, err
}

func (c *checked) is(sha string) error {
	if hex.EncodeToString(c.h.Sum(nil)) != sha {
		return fmt.Errorf("%w: the contents do not hash to %s", ErrBadRequest, sha)
	}
	return nil
}

// andNotHeld adds to the missing contents those the room holds and the caller may not have.
func (c caller) andNotHeld(ctx context.Context, asked, missing []string) ([]string, error) {
	if c.acc.Sees == nil {
		return missing, nil
	}
	done := make(map[string]bool, len(asked))
	for _, sha := range missing {
		done[sha] = true
	}
	for _, sha := range asked {
		if done[sha] {
			continue
		}
		done[sha] = true
		held, err := c.holds(ctx, sha)
		if err != nil {
			return nil, err
		}
		if !held {
			missing = append(missing, sha)
		}
	}
	return missing, nil
}
