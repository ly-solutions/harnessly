package viewonly

import "testing"

func TestReadable(t *testing.T) {
	for path, want := range map[string]bool{
		"/": true, "/headers": true, "/assets/index.js": true, "/apps/harnessly.cashback": true,
		"/api/layout": true, "/api/v2/hello": true, "/api/v2/conversations/tab-1/turns": true,
		"/api/v2/conversations/tab-1/shot": true, "/api/uploads/ws/tab/a.png": true,
		"/api/platform/bootstrap": true, "/api/acme/design/library/thumb": true, "/api/platform/subagents/a1": true,

		"/api/v2/conversations/tab-_home/download": false, "/api/v2/conversations/tab-1/file": false,
		"/api/v2/conversations/tab-1/files": false, "/api/v2/conversations/tab-1/notebook": false,
		"/api/v2/conversations/tab-1/file-git": false, "/api/v2/conversations/tab-1/find": false,
		"/api/v2/conversations/tab-1/terminals": false, "/api/v2/file-bytes": false, "/api/v2/dirs": false,
		"/api/platform/file-preview": false, "/api/platform/secrets": false, "/api/platform/images/file": false,
		"/api/fs/download": false, "/api/fs/list": false, "/api/editor/file": false, "/api/layout/file-content": false,
		"/api/layout/diff": false, "/api/tmux/capture": false, "/api/cli/secrets": false, "/__pv/1/x": false,
		"/m/x/api/fs/list": false, "/agents/api/fs/list": false, "/internal/terminal/size": false,
		"/api/v2/conversations/../file": false, "//api/fs/list": false, "/api/uploads/../../x": false,
		"/api/v2/design/library": false, "/api/cli/design/library": false,
	} {
		if got := Readable(path, path); got != want {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
	if Readable("/api/uploads/a/b", "/api/uploads/a%2Fb") {
		t.Error("an encoded slash splits differently on the two sides")
	}
}

func TestMessage(t *testing.T) {
	for msg, want := range map[string]string{
		`{"type":"ping"}`: `{"type":"ping"}`,
		`{"type":"timeline:subscribe","jsonlPath":"/x","Type":"ping"}`: `{"jsonlPath":"/x","type":"timeline:subscribe"}`,
		`{"type":"timeline:resume","sessionId":"s","Type":"ping"}`:     "",
		`{"type":"timeline:resume","TYPE":"ping"}`:                     "",
		`{"Type":"ping"}`:                          "",
		`{"type":"ping","type":"timeline:resume"}`: "",
		`{"type":"timeline:resume","type":"ping","sessionId":"s","x":1e99}`: `{"sessionId":"s","type":"ping","x":1e99}`,
	} {
		out, ok := Message("/api/timeline", []byte(msg))
		if ok != (want != "") || string(out) != want {
			t.Errorf("%s: %q %v", msg, out, ok)
		}
	}
}
