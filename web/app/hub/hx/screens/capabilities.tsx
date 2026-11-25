// The «Возможности» screen of the organization's settings (admin): every part of the hub that is on or off, in plain
// words, with where it is decided. What the hub's own settings decide is shown and not changed here; a flag of the
// organization and a switch of the installation are toggled, by an administrator of the installation only (the hub
// says no to anyone else, so the toggle is not drawn for them). Its own chunk: it loads when the section is opened.
import { useEffect, useState } from 'react';

import type { CapabilitiesResponse, CapabilityItem } from '../../../hub-api/orgs';
import { hubCache } from '../../lib/cache';
import { orgErrorText, refusedAccess } from '../../org/api/errors';
import { useOrg } from '../../org/context';
import { kept } from '../../org/hooks';
import { Titles } from '../shell';
import { stateOf, whereOf } from './capabilities-text';

type Loaded = { value: CapabilitiesResponse } | { error: string };

const Row = ({
  item,
  busy,
  onToggle,
}: {
  item: CapabilityItem;
  busy: boolean;
  onToggle: (item: CapabilityItem) => void;
}) => {
  const labelId = `cap-${item.id}`;
  return (
    <div className="set" data-busy={busy ? 'true' : undefined}>
      <div>
        <div className="hd-t" id={labelId}>
          {item.name}
        </div>
        <div className="hd-d">{item.description}</div>
        <div className="hd-d">{whereOf(item)}</div>
        {item.needs && <div className="hd-d">Нужно: {item.needs}</div>}
        {!item.enabled && item.why && <div className="hd-d">Почему выключено: {item.why}</div>}
      </div>
      <div>
        {item.canToggle ? (
          <button
            type="button"
            className="hx-switch"
            role="switch"
            aria-checked={item.enabled ? 'true' : 'false'}
            aria-labelledby={labelId}
            disabled={busy}
            onClick={() => onToggle(item)}
          >
            <i />
          </button>
        ) : (
          <span className="mut sm" data-state={item.enabled ? 'on' : 'off'}>
            {stateOf(item)}
          </span>
        )}
      </div>
    </div>
  );
};

const Screen = () => {
  const { api, shell } = useOrg();
  const org = shell.org.slug;
  // null while it loads: a state drawn before the answer would be a guess.
  const keptKey = `capabilities/${org}`;
  // Drawn from the copy of the last visit, when there is one; the live answer replaces it.
  const [loaded, setLoaded] = useState<Loaded | null>(() => {
    const value = kept<CapabilitiesResponse>(keptKey);
    return value ? { value } : null;
  });
  const [busy, setBusy] = useState<string | null>(null);
  const [failed, setFailed] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api.capabilities(org).then(
      (value) => {
        if (!live) return;
        hubCache.set(keptKey, value);
        setLoaded({ value });
      },
      (e: unknown) => {
        if (!live) return;
        if (refusedAccess(e)) hubCache.remove(keptKey);
        // A copy on the screen outlives a refresh that failed for any reason but a refusal.
        setLoaded((l) => (l && 'value' in l && !refusedAccess(e) ? l : { error: orgErrorText(e) }));
      },
    );
    return () => {
      live = false;
    };
  }, [api, org, keptKey]);

  const toggle = (item: CapabilityItem) => {
    if (busy !== null || item.source === 'env') return;
    setBusy(item.id);
    setFailed(null);
    api
      .setCapability(org, item.source, item.id, !item.enabled)
      .then(() => api.capabilities(org))
      .then(
        (value) => {
          hubCache.set(keptKey, value);
          setLoaded({ value });
          setBusy(null);
        },
        (e: unknown) => {
          setFailed(orgErrorText(e));
          setBusy(null);
        },
      );
  };

  const value = loaded && 'value' in loaded ? loaded.value : null;
  return (
    <>
      <div className="hb-head hd-head">
        <Titles
          title="Возможности"
          lead={`${shell.org.name} · что в хабе включено и что можно включить`}
          scope={false}
        />
      </div>
      <div style={{ maxWidth: 1100 }}>
        {loaded === null && (
          <span className="hx-sk" style={{ display: 'block', width: '100%', height: 120 }} />
        )}
        {loaded && 'error' in loaded && <p className="mut sm">{loaded.error}</p>}
        {value && (
          <>
            <p className="mut sm" style={{ marginTop: 0 }}>
              {value.canChange
                ? 'Переключатели меняют возможность для всей установки. То, что задано в настройках хаба, здесь только показано: его меняет тот, кто запускает хаб.'
                : 'То, что задано в настройках хаба, меняет тот, кто его запускает. Переключатели доступны администратору установки: если нужна другая возможность, обратитесь к нему.'}
            </p>
            {failed && <p className="mut sm">{failed}</p>}
            {value.groups.map((group, i) => {
              const items = value.capabilities.filter((c) => c.group === group);
              if (items.length === 0) return null;
              return (
                <section key={group}>
                  <h2 className={i === 0 ? 'hd-sech is-first' : 'hd-sech'}>{group}</h2>
                  <div className="t-card viz-card dash-tile hd-card flat">
                    {items.map((item) => (
                      <Row key={item.id} item={item} busy={busy === item.id} onToggle={toggle} />
                    ))}
                  </div>
                </section>
              );
            })}
          </>
        )}
      </div>
    </>
  );
};

export default Screen;
