package designaccess

import "testing"

// the motion kit and the video runtime are a snapshot's own files; the library's other routes are not.
func TestServedSegment(t *testing.T) {
	for first, want := range map[string]bool{
		"Board.dc.html": true, "records": true, "__guest": true, "__vendor": true, "__lib": true,
		"__rev": false, "__share": false, "__media": false, "__state": false,
	} {
		if got := servedSegment(first); got != want {
			t.Errorf("%s: %v want %v", first, got, want)
		}
	}
}
