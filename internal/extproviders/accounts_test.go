package extproviders

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeveralAccountsOfOneProvider(t *testing.T) {
	svc, sec := newService(t)
	add := func(name, key string) View {
		t.Helper()
		v, err := svc.Create(Input{Name: name, BaseURL: "https://api.deepseek.com/v1", API: APIOpenAI, Preset: "deepseek", Key: key})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	first, second, third := add("DeepSeek", "key-one"), add("DeepSeek", "key-two"), add("deepseek", "key-three")
	if first.Name != "DeepSeek" || second.Name != "DeepSeek 2" || third.Name != "deepseek 3" {
		t.Fatalf("names: %q %q %q", first.Name, second.Name, third.Name)
	}
	if first.Group != second.Group || !first.Default || second.Default || third.Default {
		t.Fatalf("groups and defaults: %+v %+v %+v", first, second, third)
	}
	// Every account has a key of its own in the safe.
	if first.ID == second.ID || sec["provider-"+first.ID+"-key"] != "key-one" || sec["provider-"+second.ID+"-key"] != "key-two" {
		t.Fatalf("keys: %v", sec)
	}

	// A name of its own, which no other account has.
	work := "Work"
	if v, err := svc.Update(second.ID, Patch{Name: &work}); err != nil || v.Name != "Work" {
		t.Fatalf("rename: %+v %v", v, err)
	}
	taken := " work "
	if _, err := svc.Update(third.ID, Patch{Name: &taken}); ReasonOf(err) != ReasonNameTaken {
		t.Fatalf("a taken name: %v", err)
	}
	if _, err := svc.Update(second.ID, Patch{Name: &taken}); err != nil {
		t.Fatalf("its own name again: %v", err)
	}

	// One default of the provider: making another the default takes it from the first.
	yes := true
	if v, err := svc.Update(second.ID, Patch{Default: &yes}); err != nil || !v.Default {
		t.Fatalf("default: %+v %v", v, err)
	}
	list, _ := svc.List()
	if list[0].Default || !list[1].Default || list[2].Default {
		t.Fatalf("defaults after the change: %+v", list)
	}

	// The default account removed: the one added first among the rest takes its place, and only its key is gone.
	gone, _ := svc.Get(second.ID)
	if err := svc.Remove(second.ID); err != nil {
		t.Fatal(err)
	}
	next, ok := svc.DefaultOf(gone)
	if !ok || next.ID != first.ID {
		t.Fatalf("the default after the removal: %+v %v", next, ok)
	}
	if _, has := sec["provider-"+second.ID+"-key"]; has || sec["provider-"+first.ID+"-key"] != "key-one" {
		t.Fatalf("keys after the removal: %v", sec)
	}
}

func TestAnotherProviderIsAnotherGroup(t *testing.T) {
	svc, _ := newService(t)
	a, _ := svc.Create(Input{Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", API: APIOpenAI, Preset: "deepseek"})
	b, _ := svc.Create(Input{Name: "Groq", BaseURL: "https://api.groq.com/openai/v1", API: APIOpenAI, Preset: "groq"})
	c, _ := svc.Create(Input{Name: "Local", BaseURL: "http://localhost:11434/v1", API: APIOpenAI, Preset: PresetCustom})
	d, _ := svc.Create(Input{Name: "Local too", BaseURL: "http://LOCALHOST:11434/v1", API: APIOpenAI, Preset: PresetCustom})
	if a.Group == b.Group || !a.Default || !b.Default {
		t.Fatalf("two providers: %+v %+v", a, b)
	}
	if c.Group != d.Group || !c.Default || d.Default {
		t.Fatalf("two accounts of one address: %+v %+v", c, d)
	}
}

func TestARegistryWrittenBeforeAccountsKeepsItsProviders(t *testing.T) {
	svc, sec := newService(t)
	sec["provider-deepseek-key"] = testKey
	old := `{"providers":[{"id":"deepseek","name":"DeepSeek","baseUrl":"https://api.deepseek.com/v1","api":"openai","preset":"deepseek","keyRef":"provider-deepseek-key","models":[{"id":"deepseek-chat"}],"createdAt":1,"updatedAt":1}]}`
	if err := os.WriteFile(filepath.Join(svc.Store.Dir, "providers.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if got := list[0]; got.ID != "deepseek" || !got.Default || !got.HasKey || len(got.Models) != 1 {
		t.Fatalf("the provider as the only account: %+v", got)
	}
	p, _ := svc.Get("deepseek")
	if key, _ := svc.Key(p); key != testKey {
		t.Fatalf("the key: %q", key)
	}
}
