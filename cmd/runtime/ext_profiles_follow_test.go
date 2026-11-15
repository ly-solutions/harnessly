package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ly-solutions/harnessly/internal/extproviders"
	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// followSetup is a machine whose profiles follow a registry, with what its pages were told.
func followSetup(t *testing.T) (*extproviders.Service, *liveMachine, *[]wire.MachineStatusEv) {
	t.Helper()
	data := t.TempDir()
	sec := mapSecrets{}
	svc := &extproviders.Service{Store: &extproviders.Store{Dir: filepath.Join(data, "v2", "ext-providers")}, Secrets: sec,
		OnChange: tellExtProviderHearers(data)}
	store := &providers.Store{Dir: filepath.Join(data, "providers"), Safe: sec.Reveal}
	t.Cleanup(func() {
		providers.SetCustom(nil)
		providers.SetBacked(nil)
		extProvidersMu.Lock()
		delete(extProviderHearers, data)
		extProvidersMu.Unlock()
	})
	m := newLiveMachine("m", "v", nil, t.TempDir(), nil, nil, nil)
	m.offers = profileOffers(store)
	m.machine.Agents = m.withOffers([]wire.MachineAgent{{Provider: wire.ProviderClaude}})
	heard := &[]wire.MachineStatusEv{}
	m.subs[1] = func(e wire.Event) { *heard = append(*heard, e.(wire.MachineStatusEv)) }
	followExtProviders(data, svc, store, m.republishProfiles)
	return svc, m, heard
}

// offeredTo is the profiles with a key that the pages were last told of.
func offeredTo(heard []wire.MachineStatusEv) []string {
	if len(heard) == 0 {
		return nil
	}
	var ids []string
	for _, p := range heard[len(heard)-1].Agents[0].Profiles {
		if p.Configured {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// An account the person adds is in the picker of the next chat at once, and one they remove leaves it: the pages are
// told with the change, not when the runtime starts again.
func TestAnAddedAccountIsOfferedAtOnce(t *testing.T) {
	svc, _, heard := followSetup(t)

	made, err := svc.Create(extproviders.Input{Name: "Рабочий", BaseURL: "https://api.example.com/anthropic", API: extproviders.APIAnthropic,
		Key: vendorKey, Models: []extproviders.Model{{ID: "big-model"}}})
	if err != nil {
		t.Fatal(err)
	}
	own := extProfileID(made.ID)
	if !slices.Contains(offeredTo(*heard), own) {
		t.Fatalf("the pages were not told of the new account: %v", offeredTo(*heard))
	}

	if err := svc.Remove(made.ID); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(offeredTo(*heard), own) {
		t.Fatalf("the removed account is still offered: %v", offeredTo(*heard))
	}
}

// A provider saved without models is no profile yet; the check that brings its models makes it one at once.
func TestACheckThatBringsModelsMakesTheProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"big-model","display_name":"Big","type":"model"}],"has_more":false}`))
	}))
	defer srv.Close()
	svc, _, heard := followSetup(t)

	made, err := svc.Create(extproviders.Input{Name: "Рабочий", BaseURL: srv.URL, API: extproviders.APIAnthropic, Key: vendorKey})
	if err != nil {
		t.Fatal(err)
	}
	own := extProfileID(made.ID)
	if slices.Contains(offeredTo(*heard), own) {
		t.Fatalf("a provider without models is offered: %v", offeredTo(*heard))
	}

	got, err := svc.Check(t.Context(), made.ID)
	if err != nil || !got.OK {
		t.Fatalf("check: %+v %v", got, err)
	}
	if !slices.Contains(offeredTo(*heard), own) {
		t.Fatalf("the checked provider is not offered: %v", offeredTo(*heard))
	}
	told := len(*heard)
	if _, err := svc.Check(t.Context(), made.ID); err != nil || len(*heard) != told {
		t.Fatalf("a check that brought the same models told the pages again: %v, %d then %d", err, told, len(*heard))
	}
}
