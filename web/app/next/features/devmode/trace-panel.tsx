// The developer mode's panel at the bottom of Settings → Connections: the trace of the connectors as the
// machine keeps it — each check and poll with its time, how many things came, the new ids, the cursor before and after,
// a refusal of the recipe's guard with its reason, each event's raw envelope and where it went (the hub's journal, the
// inbox, a rule), and what the MCP synchronization saw. It folds to its head line; a line unfolds to its raw JSON.
// Drawn only when the mode is on and never on a phone (DevTracePanel).
import '@/next/styles/dev-trace.css';

import { useEffect, useMemo, useState } from 'react';
import { followTrace, loadTrace, useDevMode, type ITraceEntry } from '@/next/features/devmode/api';
import { t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';
import Icon from '@/next/ui/icon';
import { usePhone } from '@/next/ui/use-media';

/** Lines kept in the panel. */
const KEEP = 500;
const OPEN_KEY = 'hx.devTrace.open';

export interface ITraceConn {
  id: string;
  name: string;
}

interface ITracePanelProps {
  machine: MachineId;
  /** The connections the filter offers, with their names; the trace's other keys are listed by id. */
  conns: readonly ITraceConn[];
}

const readOpen = (): boolean => {
  try {
    return window.localStorage.getItem(OPEN_KEY) === '1';
  } catch {
    return false;
  }
};

const keepOpen = (open: boolean) => {
  try {
    window.localStorage.setItem(OPEN_KEY, open ? '1' : '0');
  } catch {
    // a private window keeps nothing
  }
};

const timeOf = (at: string) => {
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return at;
  const pad = (n: number, w = 2) => String(n).padStart(w, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
};

const TraceRow = ({ entry, name }: { entry: ITraceEntry; name: string }) => {
  const [open, setOpen] = useState(false);
  const hasData = entry.data !== null && entry.data !== undefined;
  return (
    <li className="hx-dtr__row" data-level={entry.level}>
      <button
        type="button"
        className="hx-dtr__line"
        aria-expanded={hasData ? open : undefined}
        disabled={!hasData}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="hx-dtr__time">{timeOf(entry.at)}</span>
        <span className="hx-dtr__level">{entry.level}</span>
        <span className="hx-dtr__conn">{name}</span>
        <span className="hx-dtr__part">{entry.part}</span>
        <span className="hx-dtr__msg">{entry.message}</span>
      </button>
      {open && hasData && <pre className="hx-dtr__json">{JSON.stringify(entry.data, null, 2)}</pre>}
    </li>
  );
};

export const TracePanel = ({ machine, conns }: ITracePanelProps) => {
  const [open, setOpen] = useState(readOpen);
  const [conn, setConn] = useState('');
  const [entries, setEntries] = useState<readonly ITraceEntry[]>([]);
  const [keys, setKeys] = useState<readonly string[]>([]);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!open) return;
    let live = true;
    void loadTrace(machine, conn).then((res) => {
      if (!live) return;
      setFailed(!('ok' in res));
      if (!('ok' in res)) return;
      setKeys(res.value.conns);
      // What the stream brought while the list was read stays after it; lines of another filter go.
      setEntries((cur) => {
        const last = res.value.entries.at(-1)?.seq ?? 0;
        const later = cur.filter((e) => e.seq > last && (conn === '' || e.conn === conn));
        return [...res.value.entries, ...later].slice(-KEEP);
      });
    });
    const stop = followTrace(machine, conn, (e) => {
      if (!live) return;
      setEntries((cur) => (cur.some((x) => x.seq === e.seq) ? cur : [...cur, e].slice(-KEEP)));
      setKeys((cur) => (cur.includes(e.conn) ? cur : [...cur, e.conn]));
    });
    return () => {
      live = false;
      stop();
    };
  }, [machine, conn, open]);

  const names = useMemo(() => new Map(conns.map((c) => [c.id, c.name])), [conns]);
  const nameOf = (id: string) =>
    id === 'mcp-sync' ? t('next.devmode.sync') : (names.get(id) ?? (id || t('next.devmode.new')));
  const options = useMemo(() => {
    const ids = [...new Set([...conns.map((c) => c.id), ...keys])];
    return ids;
  }, [conns, keys]);

  const toggle = () => {
    setOpen((v) => {
      keepOpen(!v);
      return !v;
    });
  };

  return (
    <section className="hx-dtr" data-open={open || undefined} aria-label={t('next.devmode.panel')}>
      <div className="hx-dtr__head">
        <button type="button" className="hx-dtr__fold" aria-expanded={open} onClick={toggle}>
          <Icon name={open ? 'chevron-down' : 'chevron-up'} size="sm" />
          <span>{t('next.devmode.panel')}</span>
          {entries.length > 0 && <span className="hx-dtr__count">{entries.length}</span>}
        </button>
        {open && (
          <>
            <select
              className="hx-dtr__filter"
              aria-label={t('next.devmode.filter')}
              value={conn}
              onChange={(e) => {
                setEntries([]);
                setConn(e.target.value);
              }}
            >
              <option value="">{t('next.devmode.all')}</option>
              {options.map((id) => (
                <option key={id} value={id}>
                  {nameOf(id)}
                </option>
              ))}
            </select>
            <button type="button" className="hx-dtr__clear" onClick={() => setEntries([])}>
              {t('next.devmode.clear')}
            </button>
          </>
        )}
      </div>
      {open && (
        <div className="hx-dtr__body">
          {failed && <p className="hx-dtr__empty">{t('next.devmode.failed')}</p>}
          {!failed && entries.length === 0 && <p className="hx-dtr__empty">{t('next.devmode.empty')}</p>}
          <ol className="hx-dtr__list">
            {entries.map((e) => (
              <TraceRow key={e.seq} entry={e} name={nameOf(e.conn)} />
            ))}
          </ol>
        </div>
      )}
    </section>
  );
};

/** The panel, only when the machine's developer mode is on and the screen is not a phone's. */
const DevTracePanel = (props: ITracePanelProps) => {
  const phone = usePhone();
  const on = useDevMode(props.machine);
  if (phone || on !== true) return null;
  return <TracePanel {...props} />;
};

export default DevTracePanel;
