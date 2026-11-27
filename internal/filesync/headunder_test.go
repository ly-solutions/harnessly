package filesync

import "testing"

// the newest revision the agent holds under a folder, deleted files included, and nothing of a folder whose
// name only starts the same.
func TestStateHeadUnder(t *testing.T) {
	st, err := openState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.db.Close()
	for p, k := range map[string]known{
		"site/index.html": {Rev: 3},
		"site/a/b.css":    {Rev: 9, Deleted: true},
		"site2/x.html":    {Rev: 20},
		"other/y.html":    {Rev: 15},
	} {
		if err := st.put(p, k); err != nil {
			t.Fatal(err)
		}
	}
	for prefix, want := range map[string]int64{"site": 9, "site2": 20, "none": 0} {
		if got, err := st.headUnder(prefix); err != nil || got != want {
			t.Fatalf("%s: %d %v, want %d", prefix, got, err, want)
		}
	}
}
