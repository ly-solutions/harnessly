package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
)

// The limits of a secret a server keeps (the projects whose chats may use it, the sites it may be typed into) lie
// beside its value, so that every machine of the person applies them. The ledger of a machine holds a copy,
// and alone holds the limits where the server cannot: a hub of before takes one value only.

// LimitsKey is the key a secret's limits are kept under at its path, MaxLimits their size.
const (
	LimitsKey = "harnessly-limits"
	MaxLimits = 16 << 10
)

type limits struct {
	Projects []string `json:"projects,omitempty"`
	Origins  []string `json:"origins,omitempty"`
}

// limitsOf is what data says of the limits; false when it says nothing. Limits that cannot be read allow no project.
func limitsOf(data map[string]string) (limits, bool) {
	raw, ok := data[LimitsKey]
	if !ok {
		return limits{}, false
	}
	var l limits
	if json.Unmarshal([]byte(raw), &l) != nil {
		return limits{Projects: []string{""}}, true
	}
	return l, true
}

func (l limits) apply(v *View) {
	v.Projects, v.Origins = slices.Clone(l.Projects), slices.Clone(l.Origins)
}

// storeLimits writes v's limits beside the value in data, as read from name's path. A secret kept under another key
// than ValueKey (one another tool wrote) is left as it is, and a server that does not take the limits keeps the value
// alone: the ledger still applies them here.
func (r *Remote) storeLimits(ctx context.Context, name string, data map[string]string, v View) error {
	if _, ok := data[ValueKey]; !ok {
		return nil
	}
	raw, err := json.Marshal(limits{Projects: v.Projects, Origins: v.Origins})
	if err != nil {
		return err
	}
	out := map[string]string{}
	for k, x := range data {
		out[k] = x
	}
	out[LimitsKey] = string(raw)
	err = r.KV.Write(ctx, r.path(name), out)
	if ReasonOf(err) == ReasonEmpty {
		return nil
	}
	return err
}

// syncLimits takes the server's limits of name into the ledger and returns data as read; ok false when the server did
// not answer, and the ledger's copy stands.
func (r *Remote) syncLimits(name string) (map[string]string, bool) {
	ctx, cancel := ctxCall()
	defer cancel()
	data, err := r.KV.Read(ctx, r.path(name))
	if err != nil {
		return nil, false
	}
	if l, ok := limitsOf(data); ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		led := r.readLedger()
		e := led.Secrets[name]
		if e == nil {
			e = &View{Name: name, UsedBy: []Use{}}
			led.Secrets[name] = e
		}
		l.apply(e)
		r.writeLedger(led)
	}
	return data, true
}

// pushLimits writes the ledger's limits of name to the server, over the value it keeps now.
func (r *Remote) pushLimits(name string, v View) error {
	ctx, cancel := ctxCall()
	defer cancel()
	data, err := r.KV.Read(ctx, r.path(name))
	if err != nil {
		return err
	}
	if data == nil {
		return refuse(ReasonNotFound, errors.New(name))
	}
	return r.storeLimits(ctx, name, data, v)
}
