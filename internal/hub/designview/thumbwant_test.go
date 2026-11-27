package designview

import (
	"context"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/designaccess"
)

// headAt is a room whose project changed last at a given moment.
type headAt struct {
	*memRoom
	at int64
}

func (r headAt) HistoryUnder(context.Context, string, int) ([]filestore.Entry, error) {
	return []filestore.Entry{{Rev: 9, At: r.at}}, nil
}

// a board's picture is drawn again when it is missing or older than the project's last change.
func TestThumbWantFollowsTheProject(t *testing.T) {
	ctx := context.Background()
	at := time.Now().Add(-time.Hour).UnixMilli()
	room := headAt{newMemRoom(), at}
	v := New(Options{
		Rooms:  func(context.Context, string) (Room, error) { return room, nil },
		Thumbs: pictureRoom(t, filestore.KindThumbs, map[string]string{filestore.ThumbPath("shop", "Home.dc.html"): "thumb-home"}),
		Dir:    t.TempDir(),
	})
	rev, _, _, want, err := v.ThumbWant(ctx, "s1", "shop", "Home.dc.html")
	if err != nil || want || rev != 9 {
		t.Fatalf("a fresh picture: rev %d want %v %v", rev, want, err)
	}
	if _, _, _, want, _ := v.ThumbWant(ctx, "s1", "shop", "About.dc.html"); !want {
		t.Fatal("a board with no picture is not drawn")
	}
	room.at = time.Now().Add(time.Hour).UnixMilli()
	v.o.Rooms = func(context.Context, string) (Room, error) { return room, nil }
	if _, _, _, want, _ := v.ThumbWant(ctx, "s1", "shop", "Home.dc.html"); !want {
		t.Fatal("a picture older than the project's last change is not drawn again")
	}
	if _, _, _, want, _ := v.ThumbWant(ctx, "../x", "shop", "Home.dc.html"); want {
		t.Fatal("a bad space")
	}
}

// the first board is drawn as the cover too when the project's cover is missing or older than the project.
func TestCoverWantFollowsTheFirstBoard(t *testing.T) {
	ctx := context.Background()
	room := headAt{newMemRoom(), time.Now().Add(-time.Hour).UnixMilli()}
	v := New(Options{
		Rooms:  func(context.Context, string) (Room, error) { return room, nil },
		Covers: pictureRoom(t, filestore.KindCovers, map[string]string{filestore.CoverPath("shop"): "cover-shop"}),
		Dir:    t.TempDir(),
	})
	if _, want, _ := v.CoverWant(ctx, "s1", "shop", "Home.dc.html"); want {
		t.Fatal("a project not listed yet has no first board")
	}
	boards := []designaccess.LibraryBoard{{Path: "Home.dc.html"}, {Path: "About.dc.html"}}
	v.mu.Lock()
	v.listing = map[string]listedProject{}
	v.listing["s1/shop"] = listedProject{item: designaccess.LibraryItem{Boards: boards}}
	v.listing["s1/blog"] = listedProject{item: designaccess.LibraryItem{Boards: boards}}
	v.mu.Unlock()
	if rev, want, err := v.CoverWant(ctx, "s1", "shop", "Home.dc.html"); err != nil || want || rev != 9 {
		t.Fatalf("a fresh cover: rev %d want %v %v", rev, want, err)
	}
	if _, want, _ := v.CoverWant(ctx, "s1", "blog", "Home.dc.html"); !want {
		t.Fatal("a project with no cover gets none")
	}
	if _, want, _ := v.CoverWant(ctx, "s1", "blog", "About.dc.html"); want {
		t.Fatal("a board other than the first is drawn as the cover")
	}
	room.at = time.Now().Add(time.Hour).UnixMilli()
	if _, want, _ := v.CoverWant(ctx, "s1", "shop", "Home.dc.html"); !want {
		t.Fatal("a cover older than the project's last change is not drawn again")
	}
}
