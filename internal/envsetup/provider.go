package envsetup

import (
	"strings"

	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// provider puts the item's model provider into the machine's registry as the environment's: the person sees it beside
// their own accounts and cannot change or remove it. The source names the key and the secret variables. A secret of
// the person's safe is used only once the person has put it or allowed it for the provider's server (the registry asks
// them, and keeps the provider without it till then); a reference to the organization's secret is read once to say
// now, not at the first launch, that it cannot be read.
func (ex *execution) provider(i int) {
	it := ex.items[i]
	if ex.r.Providers == nil {
		ex.fail(i, "эта машина не держит провайдеров моделей из окружения")
		return
	}
	if ex.sandbox {
		ex.done(i, hubproto.EnvSkipped, "в песочнице провайдеры не подключаются")
		return
	}
	if !ex.modifies {
		if ex.r.Providers.Has(it.ItemID) {
			ex.done(i, hubproto.EnvSkipped, "уже подключён")
		} else {
			ex.pend(i, "не подключён — подключится при обновлении")
		}
		return
	}
	changed, waiting, shared, err := ex.r.Providers.Apply(it.ItemID, it.Name, it.Source)
	if err != nil {
		ex.fail(i, "провайдер не подключён: "+err.Error())
		return
	}
	var unread []string
	for _, ref := range shared {
		if ex.r.Secrets == nil {
			unread = append(unread, ref)
		} else if _, err := ex.r.Secrets.Reveal([]string{ref}); err != nil {
			unread = append(unread, ref)
		}
	}
	for _, name := range waiting {
		// A secret that is not in the safe yet wakes the setup when it arrives; one that waits for the person's
		// permission is looked at again at the next retry.
		if ex.r.Secrets != nil && !ex.r.Secrets.Has(name) && !contains(ex.waits, name) {
			ex.waits = append(ex.waits, name)
		}
	}
	switch {
	case len(unread) > 0:
		ex.fail(i, "подключён, но секрет организации не читается: "+strings.Join(unread, ", "))
	case len(waiting) > 0:
		ex.pend(i, "подключён без секрета, ждёт вашего разрешения в сейфе: "+strings.Join(waiting, ", "))
	case changed:
		ex.done(i, hubproto.EnvOK, "подключён")
	default:
		ex.done(i, hubproto.EnvSkipped, "уже подключён")
	}
}

// providerItems are the provider items of the list.
func (ex *execution) providerItems() []string {
	keep := []string{}
	for _, it := range ex.items {
		if it.Kind == hubproto.EnvKindProvider {
			keep = append(keep, it.ItemID)
		}
	}
	return keep
}

// pruneProviders takes off the environment's providers whose items left the list, after a full run on the machine.
func (ex *execution) pruneProviders() {
	if ex.r.Providers == nil {
		return
	}
	gone, err := ex.r.Providers.Prune(ex.providerItems())
	if err != nil {
		ex.r.logf("warn", "envsetup: the environment's providers not pruned", "error", err.Error())
		return
	}
	for _, id := range gone {
		ex.r.logf("info", "envsetup: the environment's provider taken off", "item", id)
	}
}
