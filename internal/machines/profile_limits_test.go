package machines

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotListsProfileLimits(t *testing.T) {
	dir := t.TempDir()
	file := `{"ts":1000,"entries":{
	 "glm":{"profile":"glm","account":"","ts":1000,"source":"zai-quota","five_hour":{"used_percentage":25,"resets_at":5000},"seven_day":null,"month":{"used_percentage":3,"resets_at":90000},"plan":{"name":"GLM Pro"},"error":{"message":"HTTP 500","at":1000}},
	 "kimi":{"profile":"kimi","account":"","ts":1000,"balance":{"amount":49.58,"currency":"USD"}},
	 "odd":{"ts":1000}}}`
	if err := os.WriteFile(filepath.Join(dir, "provider-limits.json"), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	asked := 0
	s := NewSnapshot(SnapshotConfig{DataDir: dir, Now: time.Now, LimitsAsked: func() { asked++ }})
	l := s.Limits(2000)
	if asked != 1 || len(l.Profiles) != 2 {
		t.Fatalf("asked %d, profiles %v", asked, l.Profiles)
	}
	glm := l.Profiles["glm"]
	if glm.FiveHour == nil || float64(glm.FiveHour.UsedPct) != 25 || glm.SevenDay != nil || glm.Month == nil || *glm.Plan != "GLM Pro" || *glm.Error != "HTTP 500" {
		t.Fatalf("glm: %+v", glm)
	}
	if b := l.Profiles["kimi"].Balance; b == nil || float64(b.Amount) != 49.58 || b.Currency != "USD" {
		t.Fatalf("kimi: %+v", l.Profiles["kimi"])
	}
	raw, _ := json.Marshal(MachineLimits{})
	if string(raw) != `{"claude":null,"codex":null}` {
		t.Fatalf("a machine without profiles changed its answer: %s", raw)
	}
}
