package wire_test

import (
	"context"
	"testing"

	"github.com/ly-solutions/harnessly/internal/wire"
)

func TestMadeRoute(t *testing.T) {
	path := "/api/v2/conversations/" + openConv + "/made"
	if code, _ := meCall(t, &wire.Server{}, "GET", path, ""); code != 404 {
		t.Errorf("no source: %d", code)
	}
	srv := &wire.Server{NotFound: func(err error) bool { return err == errGone }}
	srv.Made = func(_ context.Context, conv string) (wire.ChatMade, error) {
		if conv != openConv {
			return wire.ChatMade{}, errGone
		}
		return wire.ChatMade{Complete: true}, nil
	}
	code, res := meCall(t, srv, "GET", path, "")
	if items, ok := res["items"].([]any); code != 200 || !ok || len(items) != 0 || res["complete"] != true {
		t.Errorf("empty: %d %v", code, res)
	}
	if code, _ := meCall(t, srv, "GET", "/api/v2/conversations/tab-2/made", ""); code != 404 {
		t.Errorf("unknown conversation: %d", code)
	}
	srv.Made = func(context.Context, string) (wire.ChatMade, error) {
		return wire.ChatMade{Items: []wire.MadeItem{{Kind: wire.MadeDesign, Ref: "d", Op: wire.OutputChanged}}}, nil
	}
	if code, _ := meCall(t, srv, "GET", path, ""); code != 500 {
		t.Errorf("a design without designKind must be out of contract: %d", code)
	}
}
