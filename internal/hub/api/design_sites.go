package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/ly-solutions/harnessly/internal/hub/designaccess"
	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/sites"
)

// designSites publishes a Sinopia project for a machine that is off: the site is the machine's, as the one
// its runtime makes through its publishing session (machine:<id>, bound to the project by its origin), and the person
// who asked is its author.
type designSites struct{ h *Hub }

var _ designaccess.SitePublisher = designSites{}

func (d designSites) session(ctx context.Context, m store.Machine, u store.User) (sites.MachineSession, error) {
	sp, err := d.h.db.S().SpaceByID(ctx, m.SpaceID)
	if err != nil {
		return sites.MachineSession{}, err
	}
	name := u.Name
	if name == "" {
		name = u.Email
	}
	return sites.MachineSession{Space: sp.Slug, MachineID: m.ID, PersonID: u.ID, Name: name}, nil
}

func (d designSites) PublishSite(ctx context.Context, m store.Machine, u store.User, p designaccess.SitePublish) (designaccess.PublishedSite, error) {
	ms, err := d.session(ctx, m, u)
	if err != nil {
		return designaccess.PublishedSite{}, err
	}
	svc := d.h.Sites
	site, err := svc.Site(ctx, ms.Space, p.Name)
	switch {
	case err == nil:
		// Only the machine's own site made from this project is published onto.
		if site.Origin != p.Origin || site.CreatedByID != ms.ID() {
			return designaccess.PublishedSite{}, designaccess.ErrSiteNameTaken
		}
	case errors.Is(err, httpx.ErrNotFound):
		site, err = svc.CreateSite(ctx, ms.Space, p.Name, p.Access, nil, ms.Name, ms.ID(), p.Origin)
		if err != nil {
			return designaccess.PublishedSite{}, refusal(err)
		}
	default:
		return designaccess.PublishedSite{}, err
	}
	if site.Access != p.Access {
		access := p.Access
		if site, err = svc.Update(ctx, site, &access, nil, ms.Name); err != nil {
			return designaccess.PublishedSite{}, refusal(err)
		}
	}
	manifest := sites.Manifest{}
	data := map[string][]byte{}
	for _, f := range p.Files {
		sum := sha256.Sum256(f.Data)
		hash := hex.EncodeToString(sum[:])
		manifest[f.Path] = sites.File{Hash: hash, Size: int64(len(f.Data))}
		data[hash] = f.Data
	}
	pending, missing, err := svc.Begin(ctx, site, manifest, "sinopia", ms.Name, false)
	if err != nil {
		return designaccess.PublishedSite{}, refusal(err)
	}
	for _, hash := range missing {
		if err := svc.PutBlob(ctx, site, pending.ID, hash, bytes.NewReader(data[hash])); err != nil {
			return designaccess.PublishedSite{}, refusal(err)
		}
	}
	done, err := svc.Commit(ctx, site, pending.ID, ms.Name, true)
	if err != nil {
		return designaccess.PublishedSite{}, refusal(err)
	}
	return designaccess.PublishedSite{URL: svc.SiteURL(site), Name: site.Name, Access: site.Access, Version: done.Version}, nil
}

func (d designSites) FindSite(ctx context.Context, m store.Machine, origin string) (*designaccess.PublishedSite, error) {
	sp, err := d.h.db.S().SpaceByID(ctx, m.SpaceID)
	if err != nil {
		return nil, err
	}
	list, err := d.h.Sites.Sites(ctx, sp.Slug)
	if err != nil {
		return nil, err
	}
	own := sites.MachineSession{MachineID: m.ID}.ID()
	for _, site := range list {
		if site.Origin == origin && site.CreatedByID == own {
			return &designaccess.PublishedSite{URL: d.h.Sites.SiteURL(site), Name: site.Name, Access: site.Access, Version: site.CurrentVersion}, nil
		}
	}
	return nil, nil
}

// refusal turns what the sites module said no with into words for the person; a name that is taken is told apart.
func refusal(err error) error {
	var he *httpx.Error
	if !errors.As(err, &he) || he.Status >= 500 {
		return err
	}
	if he.Status == 409 && he.Details["reason"] == "name_taken" {
		return designaccess.ErrSiteNameTaken
	}
	return &designaccess.SiteRefusal{Message: he.Message}
}
