package main

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ly-solutions/harnessly/internal/extproviders"
	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// vendorKey is a key as a person types one for a vendor: joined from parts so that no scanner takes it for a key.
const vendorKey = "kc-" + "first-0000-abcdef"
const otherKey = "kc-" + "second-1111-fedcba"

func vendorSetup(t *testing.T) (*extproviders.Service, *providers.Store, mapSecrets) {
	t.Helper()
	data := t.TempDir()
	sec := mapSecrets{}
	svc := &extproviders.Service{Store: &extproviders.Store{Dir: filepath.Join(data, "v2", "ext-providers")}, Secrets: sec}
	store := &providers.Store{Dir: filepath.Join(data, "providers"), Safe: sec.Reveal}
	t.Cleanup(func() { providers.SetCustom(nil); providers.SetBacked(nil) })
	return svc, store, sec
}

func offered(store *providers.Store) []string {
	var ids []string
	for _, o := range profileOffers(store)("claude") {
		if o.Configured {
			ids = append(ids, o.ID)
		}
	}
	return ids
}

// The key a person typed for Kimi Code before the vendors became accounts is their first account afterwards: the same
// key, kept in the safe and not typed again, and the profile the chats run on keeps its id and goes on with it.
func TestAKeyTypedForAVendorBecomesItsFirstAccount(t *testing.T) {
	svc, store, sec := vendorSetup(t)
	if _, err := store.Set("kimi-code", vendorKey); err != nil {
		t.Fatal(err)
	}

	moveVendorKeys(svc, store)
	syncExtProfiles(svc, store)

	list, err := svc.Store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("accounts: %v %v", list, err)
	}
	first := list[0]
	if first.Preset != "kimi-code" || !first.Default || first.KeyRef == "" || sec[first.KeyRef] != vendorKey {
		t.Fatalf("the account does not hold the typed key: %+v", first)
	}
	if got, err := store.Key("kimi-code"); err != nil || got != vendorKey {
		t.Fatalf("the vendor's profile lost its key: %q %v", got, err)
	}
	if p, _ := providers.Find("kimi-code"); !p.Backed || p.Label != first.Name {
		t.Fatalf("the vendor's profile is not the account: %+v", p)
	}
	if got := offered(store); !slices.Equal(got, []string{"kimi-code"}) {
		t.Fatalf("a new chat is offered %v", got)
	}
	if _, err := store.Set("kimi-code", otherKey); err == nil {
		t.Fatal("a key was typed for the profile past its account")
	}

	// A second start moves nothing again.
	moveVendorKeys(svc, store)
	if list, _ := svc.Store.List(); len(list) != 1 {
		t.Fatalf("the key was moved twice: %d accounts", len(list))
	}
}

// A second account of the vendor is a profile of its own with the vendor's models; making it the default hands it the
// vendor's profile, and the chats started on either account's own profile go on with that account's key.
func TestAccountsOfAVendorKeepTheirProfilesWhenTheDefaultChanges(t *testing.T) {
	svc, store, _ := vendorSetup(t)
	if _, err := store.Set("kimi-code", vendorKey); err != nil {
		t.Fatal(err)
	}
	moveVendorKeys(svc, store)
	list, _ := svc.Store.List()
	first := list[0]
	second, err := svc.Create(extproviders.Input{Name: "Kimi рабочий", BaseURL: first.BaseURL, API: first.API, Preset: "kimi-code", Key: otherKey})
	if err != nil {
		t.Fatal(err)
	}
	syncExtProfiles(svc, store)

	own := extProfileID(second.ID)
	vendor, _ := providers.Find("kimi-code")
	if p, ok := providers.Find(own); !ok || len(p.Models) != len(vendor.Models) || p.Small != vendor.Small {
		t.Fatalf("the second account has no profile with the vendor's models: %+v", p)
	}
	if got, _ := store.Key(own); got != otherKey {
		t.Fatalf("the second account's profile has key %q", got)
	}
	if got := offered(store); !slices.Equal(got, []string{"kimi-code", own}) {
		t.Fatalf("a new chat is offered %v", got)
	}

	yes := true
	if _, err := svc.Update(second.ID, extproviders.Patch{Default: &yes}); err != nil {
		t.Fatal(err)
	}
	syncExtProfiles(svc, store)
	if got, _ := store.Key("kimi-code"); got != otherKey {
		t.Fatalf("the vendor's profile did not follow the default account: %q", got)
	}
	if got, _ := store.Key(own); got != otherKey {
		t.Fatalf("a chat on the second account lost its key: %q", got)
	}
	if got, _ := store.Key(extProfileID(first.ID)); got != vendorKey {
		t.Fatalf("a chat on the first account lost its key: %q", got)
	}
	if got := offered(store); !slices.Equal(got, []string{"kimi-code", extProfileID(first.ID)}) {
		t.Fatalf("after the change a new chat is offered %v", got)
	}
}

// Removing the vendor's only account takes the key away from the vendor's profile: nothing typed before comes back.
func TestRemovingTheAccountLeavesTheVendorWithoutAKey(t *testing.T) {
	svc, store, _ := vendorSetup(t)
	if _, err := store.Set("glm", vendorKey); err != nil {
		t.Fatal(err)
	}
	moveVendorKeys(svc, store)
	syncExtProfiles(svc, store)
	list, _ := svc.Store.List()
	if len(list) != 1 || list[0].Preset != "glm" {
		t.Fatalf("accounts: %+v", list)
	}
	if err := svc.Remove(list[0].ID); err != nil {
		t.Fatal(err)
	}
	syncExtProfiles(svc, store)
	if _, err := store.Key("glm"); err == nil {
		t.Fatal("the vendor's profile kept a key after its account was removed")
	}
	if got := offered(store); len(got) != 0 {
		t.Fatalf("a new chat is offered %v", got)
	}
}

// A machine nobody typed a vendor's key on moves nothing, and an account added later is the vendor's profile.
func TestAnAccountAddedOnAFreshMachineIsTheVendorsProfile(t *testing.T) {
	svc, store, _ := vendorSetup(t)
	moveVendorKeys(svc, store)
	if list, _ := svc.Store.List(); len(list) != 0 {
		t.Fatalf("accounts out of nothing: %+v", list)
	}
	k := slices.IndexFunc(extproviders.Presets(), func(p extproviders.Preset) bool { return p.ID == "glm" })
	tpl := extproviders.Presets()[k]
	if _, err := svc.Create(extproviders.Input{Name: tpl.Name, BaseURL: tpl.BaseURL, API: tpl.API, Preset: tpl.ID, Key: vendorKey}); err != nil {
		t.Fatal(err)
	}
	syncExtProfiles(svc, store)
	if got, _ := store.Key("glm"); got != vendorKey {
		t.Fatalf("the vendor's profile has key %q", got)
	}
	if got := offered(store); !slices.Equal(got, []string{"glm"}) {
		t.Fatalf("a new chat is offered %v", got)
	}
}

// failingSafe is a safe that does not take a secret while down is set.
type failingSafe struct {
	mapSecrets
	down *bool
}

func (f failingSafe) Put(name, value string) error {
	if *f.down {
		return errors.New("the safe does not answer")
	}
	return f.mapSecrets.Put(name, value)
}

// While the safe does not take the key, nothing is lost and nothing is locked: the vendor's profile goes on with the
// typed key, the page is told the key is still the profile's own (so its card stays, the way to change the key), and
// the key can be typed anew. When the safe answers again, the next start moves the key the profile has then.
func TestAKeyThatCouldNotBeMovedStaysTypedAndChangeable(t *testing.T) {
	data := t.TempDir()
	down := true
	sec := failingSafe{mapSecrets: mapSecrets{}, down: &down}
	svc := &extproviders.Service{Store: &extproviders.Store{Dir: filepath.Join(data, "v2", "ext-providers")}, Secrets: sec}
	store := &providers.Store{Dir: filepath.Join(data, "providers"), Safe: sec.Reveal}
	t.Cleanup(func() { providers.SetCustom(nil); providers.SetBacked(nil) })
	if _, err := store.Set("kimi-code", vendorKey); err != nil {
		t.Fatal(err)
	}

	moveVendorKeys(svc, store)
	syncExtProfiles(svc, store)

	if vendorKeysMoved(svc.Store.Dir) {
		t.Fatal("the move is marked done though the safe took nothing")
	}
	if list, _ := svc.Store.List(); len(list) != 0 {
		t.Fatalf("an account without its key was made: %+v", list)
	}
	if got, err := store.Key("kimi-code"); err != nil || got != vendorKey {
		t.Fatalf("a chat on the profile lost its key: %q %v", got, err)
	}
	if got := offered(store); !slices.Equal(got, []string{"kimi-code"}) {
		t.Fatalf("a new chat is offered %v", got)
	}
	listed := func() wire.ProfileInfo {
		all, err := (&providers.Source{Store: store}).Profiles(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		k := slices.IndexFunc(all.Profiles, func(p wire.ProfileInfo) bool { return p.ID == "kimi-code" })
		if k < 0 {
			t.Fatal("the profile is not listed")
		}
		return all.Profiles[k]
	}
	if p := listed(); p.Account || !p.Key.Configured {
		t.Fatalf("the page is not shown the profile's own key: %+v", p)
	}
	// The way to change the key is not lost.
	if _, err := store.Set("kimi-code", otherKey); err != nil {
		t.Fatalf("the key cannot be typed anew: %v", err)
	}

	down = false
	moveVendorKeys(svc, store)
	syncExtProfiles(svc, store)
	list, _ := svc.Store.List()
	if len(list) != 1 || sec.mapSecrets[list[0].KeyRef] != otherKey || !vendorKeysMoved(svc.Store.Dir) {
		t.Fatalf("after the safe came back: %+v", list)
	}
	if got, _ := store.Key("kimi-code"); got != otherKey {
		t.Fatalf("the profile has key %q", got)
	}
	if p := listed(); !p.Account {
		t.Fatalf("the page is not told the key is an account now: %+v", p)
	}
}
