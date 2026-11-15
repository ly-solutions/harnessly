package envsetup

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hubproto"
)

type fakeProviders struct {
	kept map[string]string // item → source
	// waiting are the names the registry would wait for: those the safe of the test does not hold.
	waiting func() []string
	shared  []string
	pruned  [][]string
	err     error
}

func (p *fakeProviders) Apply(itemID, name string, source json.RawMessage) (bool, []string, []string, error) {
	if p.err != nil {
		return false, nil, nil, p.err
	}
	changed := p.kept[itemID] != string(source)
	p.kept[itemID] = string(source)
	var waiting []string
	if p.waiting != nil {
		waiting = p.waiting()
	}
	return changed, waiting, p.shared, nil
}
func (p *fakeProviders) Has(itemID string) bool { _, ok := p.kept[itemID]; return ok }
func (p *fakeProviders) Prune(keep []string) ([]string, error) {
	p.pruned = append(p.pruned, keep)
	return nil, nil
}

func providerItem() hubproto.EnvItem {
	return hubproto.EnvItem{ItemID: "p1", Kind: hubproto.EnvKindProvider, Name: "Шлюз", Layer: "org", Pinned: true,
		Source: json.RawMessage(`{"api":"anthropic","baseUrl":"https://llm.corp.example/anthropic","key":"gateway-key"}`)}
}

func TestAProviderOfTheListIsKeptAndWaitsForItsSecret(t *testing.T) {
	f := newFixture(t)
	prov := &fakeProviders{kept: map[string]string{}}
	prov.waiting = func() []string {
		if f.safe.Has("gateway-key") {
			return nil
		}
		return []string{"gateway-key"}
	}
	f.r.Providers = prov
	f.setList(providerItem())
	f.r.Tick(context.Background())
	// It is in the registry already, without the key: the key is the person's to put and to allow for this server.
	if s := f.step(f.onlyRun(), "p1"); s.Status != hubproto.EnvPending || s.Message != "подключён без секрета, ждёт вашего разрешения в сейфе: gateway-key" {
		t.Fatalf("%+v", s)
	}
	if !prov.Has("p1") {
		t.Fatalf("kept %v", prov.kept)
	}
	if len(prov.pruned) != 1 || len(prov.pruned[0]) != 1 || prov.pruned[0][0] != "p1" {
		t.Fatalf("pruned: %v", prov.pruned)
	}

	// The secret arrived: the list is worked on again and the step is done.
	f.safe.put("gateway-key", "value")
	f.r.Tick(context.Background())
	runs := f.runsAll()
	if s := f.step(runs[len(runs)-1], "p1"); s.Status != hubproto.EnvSkipped || s.Message != "уже подключён" {
		t.Fatalf("after the secret: %+v", s)
	}

	// A look changes nothing and does not prune.
	f.request("look-run-01", hubproto.RunCheck)
	f.r.Tick(context.Background())
	if s := f.step(f.runFile("look-run-01"), "p1"); s.Status != hubproto.EnvSkipped {
		t.Fatalf("look: %+v", s)
	}
	if len(prov.pruned) != 2 {
		t.Fatalf("a look pruned: %v", prov.pruned)
	}
}

func TestAnOrganizationsSecretThatCannotBeReadIsSaidAtTheSetup(t *testing.T) {
	f := newFixture(t)
	f.r.Providers = &fakeProviders{kept: map[string]string{}, shared: []string{"vault:shared/llm/gateway#key"}}
	f.setList(providerItem())
	f.r.Tick(context.Background())
	if s := f.step(f.onlyRun(), "p1"); s.Status != hubproto.EnvError || s.Message != "подключён, но секрет организации не читается: vault:shared/llm/gateway#key" {
		t.Fatalf("%+v", s)
	}
	if len(f.safe.asked) != 0 {
		t.Fatalf("the person was asked for the organization's secret: %v", f.safe.asked)
	}
	f.safe.put("vault:shared/llm/gateway#key", "value")
	f.request("update-run-01", hubproto.RunUpdate)
	f.r.Tick(context.Background())
	if s := f.step(f.runFile("update-run-01"), "p1"); s.Status != hubproto.EnvSkipped {
		t.Fatalf("readable now: %+v", s)
	}
}

func TestAProviderThatIsRefusedOrAMachineWithoutProvidersSaysSo(t *testing.T) {
	f := newFixture(t)
	f.setList(providerItem())
	f.r.Tick(context.Background())
	if s := f.step(f.onlyRun(), "p1"); s.Status != hubproto.EnvError || s.Message != "эта машина не держит провайдеров моделей из окружения" {
		t.Fatalf("%+v", s)
	}

	g := newFixture(t)
	g.r.Providers = &fakeProviders{kept: map[string]string{}, err: errors.New("адрес не похож на адрес")}
	g.setList(providerItem())
	g.r.Tick(context.Background())
	if s := g.step(g.onlyRun(), "p1"); s.Status != hubproto.EnvError || s.Message != "провайдер не подключён: адрес не похож на адрес" {
		t.Fatalf("%+v", s)
	}
}

func TestAProviderLeavesWhenTheListIsEmptied(t *testing.T) {
	f := newFixture(t)
	prov := &fakeProviders{kept: map[string]string{}}
	f.r.Providers = prov
	f.setList(providerItem())
	f.r.Tick(context.Background())
	before := len(prov.pruned)
	// The organization took its only item back: no run is due for an empty list, and the provider still has to leave.
	f.setList()
	f.r.Tick(context.Background())
	if len(prov.pruned) != before+1 || len(prov.pruned[before]) != 0 {
		t.Fatalf("pruned %v", prov.pruned)
	}
}
