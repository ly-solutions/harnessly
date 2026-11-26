// The settings a board is seen with, the same for everybody: what a person picks in «Макет → Настройки» is
// sent to the page at once and kept in the project (meta/props.json); every viewer reads it every few seconds and puts
// on their page what differs, and the agent reads and sets the same file (`harnessly design props`).
import { useCallback, useEffect, useRef, useState } from 'react';
import type { MachineId } from '@/next/protocol/v2';
import type { ITweak, TTweakValue } from '@/next/features/designly/page-bridge';
import { fetchSiteProps, putSiteProps, type TSiteProps } from '@/next/features/designly/panel/api';
import { tweakChanged } from '@/next/features/designly/tweaks';
import { toast } from '@/next/features/designly/sdk/toast';
import { t } from '@/next/i18n';

const POLL_MS = 5000;

/** A value the board's setting takes: one of its options, else of its kind. */
const fits = (c: ITweak, v: TTweakValue) => {
  if (c.options.length > 0) return c.options.some((o) => o.value === v);
  if (c.editor === 'boolean') return typeof v === 'boolean';
  if (c.editor === 'number') return typeof v === 'number';
  return typeof v === typeof (c.value ?? c.def ?? v);
};

/** What a board's settings are away from the board's own: the values kept for everybody. */
export const keptValues = (controls: readonly ITweak[]): Record<string, TTweakValue> => {
  const out: Record<string, TTweakValue> = {};
  for (const c of controls) if (tweakChanged(c) && c.value !== null) out[c.name] = c.value;
  return out;
};

/** The changes to send the page so it stands as kept: the kept values it does not stand at yet. */
export const toApply = (controls: readonly ITweak[], kept: Record<string, TTweakValue> | undefined) => {
  const out: { name: string; value: TTweakValue }[] = [];
  for (const c of controls) {
    const want = kept && Object.hasOwn(kept, c.name) ? kept[c.name] : c.def;
    if (want === null || want === undefined || want === c.value || !fits(c, want)) continue;
    out.push({ name: c.name, value: want });
  }
  return out;
};

export interface ISiteProps {
  change: (name: string, value: TTweakValue) => void;
  reset: () => void;
}

export const useSiteProps = (
  machine: MachineId,
  project: string,
  board: string | null,
  /** The settings the page offers now; null while it has not said them. */
  controls: readonly ITweak[] | null,
  send: (name: string, value: TTweakValue) => void,
): ISiteProps => {
  const [props, setProps] = useState<TSiteProps | null>(null);
  // A change of this person waits for its write: the poll does not take the old value back over it meanwhile.
  const writing = useRef(0);
  useEffect(() => {
    const now = new AbortController();
    const read = () =>
      fetchSiteProps(machine, project, now.signal).then(
        (p) => {
          if (writing.current === 0) setProps(p);
        },
        () => undefined,
      );
    void read();
    const timer = setInterval(() => {
      if (!document.hidden) void read();
    }, POLL_MS);
    return () => {
      now.abort();
      clearInterval(timer);
    };
  }, [machine, project]);

  // The kept values go on the page; each change once per what the page said (a page that refuses one is not pressed).
  const sent = useRef(new Set<string>());
  const latestSend = useRef(send);
  useEffect(() => {
    latestSend.current = send;
  });
  const offered = controls !== null;
  useEffect(() => {
    sent.current.clear();
  }, [board, offered]);
  useEffect(() => {
    if (props === null || controls === null || board === null) return;
    // Nothing kept for a board nobody touched: the page stays as it is drawn (and as an agent's command left it).
    const kept = props[board];
    if (kept === undefined) return;
    for (const { name, value } of toApply(controls, kept)) {
      const key = `${board}|${name}|${JSON.stringify(value)}`;
      if (sent.current.has(key)) continue;
      sent.current.add(key);
      latestSend.current(name, value);
    }
  }, [props, controls, board]);

  const write = useCallback(
    (values: Record<string, TTweakValue>) => {
      if (board === null) return;
      writing.current += 1;
      setProps((p) => ({ ...(p ?? {}), [board]: values }));
      putSiteProps(machine, project, board, values)
        .then(setProps, (e: unknown) =>
          toast(t('next.ds.settings.failed'), {
            tone: 'error',
            description: e instanceof Error ? e.message : undefined,
          }),
        )
        .finally(() => {
          writing.current -= 1;
        });
    },
    [machine, project, board],
  );

  return {
    change: (name, value) => {
      send(name, value);
      if (controls === null) return;
      const next = controls.map((c) => (c.name === name ? { ...c, value } : c));
      const key = `${board}|${name}|${JSON.stringify(value)}`;
      sent.current.add(key);
      write(keptValues(next));
    },
    reset: () => {
      if (controls === null) return;
      for (const c of controls) if (tweakChanged(c) && c.def !== null) send(c.name, c.def);
      write({});
    },
  };
};
