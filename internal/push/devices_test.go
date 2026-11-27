package push

import (
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/jsvalue"
)

func TestDeviceName(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1": "iPhone · Safari",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36":                   "Android · Chrome",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36":                   "Mac · Chrome",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0":                                                        "Windows · Firefox",
		"": "",
	} {
		if got := DeviceName(ua); got != want {
			t.Errorf("%q: %q, want %q", ua, got, want)
		}
	}
}

// A device subscribing again keeps when it was first stored and last active; Pick and the approval targets follow the
// person's last use, and a device that shows the chat with the card holds the push back.
func TestDevicesPickAndApprovalTargets(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	st := &Store{Dir: t.TempDir(), Now: func() time.Time { return now }}
	keys := jsvalue.NewObject("p256dh", "P", "auth", "A")
	yes, no := true, false
	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1"
	mac := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	if err := st.AddDevice("https://push.example/phone", keys, "phone", &yes, DeviceInfo{UserAgent: iphone, Via: "m1"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err := st.AddDevice("https://push.example/mac", keys, "mac", &no, DeviceInfo{UserAgent: mac}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Minute)
	st.MarkLooking("mac", "tab-x")
	devs := st.Devices()
	if len(devs) != 2 || devs[0].ID != "mac" || !devs[0].Last || !devs[0].Visible || devs[1].Name != "iPhone · Safari" || !devs[1].Mobile {
		t.Fatalf("%+v", devs)
	}
	if subs, _ := st.Pick("last"); len(subs) != 1 || subs[0].DeviceID != "mac" {
		t.Fatalf("last: %+v", subs)
	}
	if subs, _ := st.Pick("mobile"); len(subs) != 1 || subs[0].DeviceID != "phone" || subs[0].Via != "m1" {
		t.Fatalf("mobile: %+v", subs)
	}
	if _, err := st.Pick("nope"); err != ErrNoDevice {
		t.Fatal(err)
	}
	if subs := st.ApprovalTargets("tab-y"); len(subs) != 1 || subs[0].DeviceID != "phone" {
		t.Fatalf("targets: %+v", subs)
	}
	if subs := st.ApprovalTargets("tab-x"); len(subs) != 0 {
		t.Fatalf("the card is on screen: %+v", subs)
	}
	// The phone registers again: it keeps its first time and its last activity.
	now = now.Add(2 * time.Hour)
	if err := st.AddDevice("https://push.example/phone2", keys, "phone", &yes, DeviceInfo{UserAgent: iphone}); err != nil {
		t.Fatal(err)
	}
	for _, s := range st.Subscriptions() {
		if s.DeviceID == "phone" && (s.CreatedAt != "2026-11-27T10:00:00Z" || s.Endpoint != "https://push.example/phone2") {
			t.Fatalf("%+v", s)
		}
		if s.DeviceID == "mac" && s.LastActive != "2026-11-27T10:06:00Z" {
			t.Fatalf("lastActive not kept: %+v", s)
		}
	}
}
