// The edit lease of a project (designaccess lease.go): the page of an editor in the editing mode takes it and
// renews it while it is open, and gives it back on leaving; every other page asks now and then who edits, for the header.
// The lease is advisory: a save is still guarded by its base revision in the hub's store.

import { useEffect, useState } from 'react';
import { gatewayPath } from '@/next/core/net/machine';
import { hubJson } from '@/next/core/net/runtime-json';

export interface IEditLease {
  person: string;
  name: string;
  machine: string;
  machineName: string;
  until: number;
  /** The person holds it on this machine: this page. */
  mine: boolean;
}

/** How often the editing page renews the lease (the hub keeps it 90 s) and how often the others look. */
export const LEASE_RENEW_MS = 30_000;
export const LEASE_LOOK_MS = 20_000;

const leasePath = (id: string) => `/design/store/${encodeURIComponent(id)}/lease` as const;

/** Who edits project `id` now, null when nobody does or the hub does not say; `on` false asks nothing. */
export const useEditLease = (id: string, editing: boolean, on: boolean): IEditLease | null => {
  const [lease, setLease] = useState<{ id: string; lease: IEditLease | null }>({ id, lease: null });
  useEffect(() => {
    if (!on) return;
    let stopped = false;
    let timer = 0;
    const ask = () => {
      hubJson<{ lease: IEditLease | null }>(leasePath(id), editing ? { method: 'POST' } : undefined).then(
        (got) => {
          if (!stopped) setLease({ id, lease: got.lease });
        },
        () => undefined,
      );
      timer = window.setTimeout(ask, editing ? LEASE_RENEW_MS : LEASE_LOOK_MS);
    };
    ask();
    return () => {
      stopped = true;
      window.clearTimeout(timer);
      // Leaving the editing mode (or the page) gives the lease back at once; it would run out by itself anyway.
      if (editing)
        void fetch(gatewayPath(leasePath(id)), { method: 'DELETE', keepalive: true }).catch(() => undefined);
    };
  }, [id, editing, on]);
  return lease.id === id ? lease.lease : null;
};

/** The lease of someone else, the one the header names; the page's own is not shown. */
export const othersLease = (lease: IEditLease | null, now = Date.now()): IEditLease | null =>
  lease !== null && !lease.mine && lease.until > now ? lease : null;
