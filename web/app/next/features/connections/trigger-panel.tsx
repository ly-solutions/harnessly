// «Может быть триггером» on the page of an MCP connection: the tools the runtime's classifier finds fit
// (read-only, they list things), each a chip with a person's name of what it catches. A chip opens the form of a
// trigger: the tool's arguments, a draft of the recipe from one call of the tool, the paths of a thing's fields, how
// often, a filter and the mode of the catalog. «Проверить» shows the newest things as they will be events; «Сохранить»
// makes the trigger (a connection of the connector mcp-recipe) and links to /events → Triggers.
import { useEffect, useId, useState } from 'react';
import type { ICandidate, IRecipe, ISample, ITriggerApi } from '@/next/features/connections/triggers-api';
import { FormError, SetButton } from '@/next/features/settings/rows';
import { locale, t } from '@/next/i18n';

const EVERY = ['5m', '10m', '30m', '1h', '6h'] as const;
const MODES = ['digest', 'notify', 'agent'] as const;
type TMode = (typeof MODES)[number];
const PATHS = ['items', 'id', 'at', 'title', 'actor', 'url', 'thread', 'cursor'] as const;
type TPath = (typeof PATHS)[number];

export const labelOf = (c: ICandidate): string =>
  locale() === 'ru' ? c.labelRu || c.label : c.label || c.labelRu;

interface IFormProps {
  candidate: ICandidate;
  api: ITriggerApi;
  onClose: () => void;
}

const SampleList = ({ samples }: { samples: readonly ISample[] }) =>
  samples.length === 0 ? (
    <p className="hx-trg__note">{t('next.connections.trigger.none')}</p>
  ) : (
    <ul className="hx-trg__samples" aria-label={t('next.connections.trigger.samples')}>
      {samples.map((s) => (
        <li key={s.id} className="hx-trg__sample">
          <span className="hx-trg__sample-title">{s.title || s.id}</span>
          <span className="hx-trg__sample-meta">
            {[s.at ? new Date(s.at).toLocaleString(locale()) : '', s.actor].filter(Boolean).join(' · ')}
          </span>
        </li>
      ))}
    </ul>
  );

export const TriggerForm = ({ candidate, api, onClose }: IFormProps) => {
  const uid = useId();
  const cursorArg = candidate.args.find((a) => a.cursor) ?? null;
  const [args, setArgs] = useState<Record<string, string>>({});
  const [paths, setPaths] = useState<Record<TPath, string>>({
    items: '',
    id: '',
    at: '',
    title: '',
    actor: '',
    url: '',
    thread: '',
    cursor: '',
  });
  const [trigger, setTrigger] = useState('');
  const [every, setEvery] = useState<string>('10m');
  const [filterPath, setFilterPath] = useState('');
  const [filterText, setFilterText] = useState('');
  const [mode, setMode] = useState<TMode>('digest');
  const [name, setName] = useState(labelOf(candidate));
  const [unmarked, setUnmarked] = useState(false);
  const [busy, setBusy] = useState<'draft' | 'test' | 'save' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [samples, setSamples] = useState<readonly ISample[] | null>(null);
  const [saved, setSaved] = useState(false);
  const [baseArgs, setBaseArgs] = useState<Record<string, unknown>>({});
  // Recipe v2: watched fields, facts for rules, further pages, a transform script; an API's GET.
  const [watch, setWatch] = useState('');
  const [facts, setFacts] = useState('');
  const [nextPath, setNextPath] = useState('');
  const [nextArg, setNextArg] = useState('');
  const [script, setScript] = useState('');
  const [apiPath, setApiPath] = useState('');
  const [apiQuery, setApiQuery] = useState('');
  const isApi = candidate.tool === '';

  const recipe = (): IRecipe => {
    const out: IRecipe = { trigger, tool: candidate.tool, id: paths.id, every, mode };
    const list = (v: string) =>
      v
        .split(',')
        .map((x) => x.trim())
        .filter(Boolean);
    if (list(watch).length > 0) out.watch = list(watch);
    if (list(facts).length > 0) out.facts = list(facts).slice(0, 8);
    if (nextPath.trim() !== '' && nextArg.trim() !== '')
      out.next = { path: nextPath.trim(), arg: nextArg.trim() };
    if (script.trim() !== '') out.script = script;
    if (isApi) {
      delete (out as { tool?: string }).tool;
      out.path = apiPath.trim();
      const q: Record<string, string> = {};
      for (const line of apiQuery.split('\n')) {
        const [k, ...v] = line.split('=');
        if (k.trim() !== '') q[k.trim()] = v.join('=').trim();
      }
      if (Object.keys(q).length > 0) out.query = q;
    }
    const a: Record<string, unknown> = { ...baseArgs };
    for (const [k, v] of Object.entries(args)) if (v.trim() !== '') a[k] = v.trim();
    if (Object.keys(a).length > 0) out.args = a;
    for (const p of PATHS) if (p !== 'id' && paths[p] !== '') out[p] = paths[p];
    if (candidate.gateway) out.via = candidate.gateway;
    if (filterPath.trim() !== '' && filterText.trim() !== '')
      out.filter = { [filterPath.trim()]: filterText.trim() };
    return out;
  };

  const missing = candidate.args.filter((a) => a.required && !a.cursor && (args[a.name] ?? '').trim() === '');
  const ready = trigger !== '' && paths.id !== '' && (!isApi || apiPath.trim().startsWith('/'));

  const draft = async () => {
    setBusy('draft');
    setError(null);
    const res = await api.draft({
      tool: candidate.tool,
      description: candidate.description,
      via: candidate.gateway,
      args: Object.fromEntries(Object.entries(args).filter(([, v]) => v.trim() !== '')),
      cursorArg: cursorArg?.name ?? '',
      cursorNumber: cursorArg !== null && (cursorArg.type === 'integer' || cursorArg.type === 'number'),
      unmarked,
    });
    setBusy(null);
    if (!res.ok) {
      setError(res.error);
      return;
    }
    const r = res.value.recipe;
    setTrigger(r.trigger);
    setBaseArgs(r.args ?? {});
    setPaths({
      items: r.items ?? '',
      id: r.id ?? '',
      at: r.at ?? '',
      title: r.title ?? '',
      actor: r.actor ?? '',
      url: r.url ?? '',
      thread: r.thread ?? '',
      cursor: r.cursor ?? '',
    });
    setSamples(res.value.samples);
  };

  const test = async () => {
    setBusy('test');
    setError(null);
    const res = await api.test(recipe(), unmarked);
    setBusy(null);
    if (!res.ok) setError(res.error);
    else setSamples(res.value);
  };

  const save = async () => {
    setBusy('save');
    setError(null);
    const res = await api.save(recipe(), unmarked, name);
    setBusy(null);
    if (!res.ok) setError(res.error);
    else setSaved(true);
  };

  if (saved)
    return (
      <div className="hx-trg__form" role="status">
        <p className="hx-trg__ok">{t('next.connections.trigger.saved', { name })}</p>
        {api.eventsUrl && (
          <a className="hx-mcp-row__link" href={api.eventsUrl} target="_blank" rel="noreferrer">
            {t('next.connections.trigger.events')}
          </a>
        )}
        <SetButton tone="ghost" onClick={onClose}>
          {t('next.connections.trigger.close')}
        </SetButton>
      </div>
    );

  return (
    <form
      className="hx-trg__form"
      aria-label={t('next.connections.trigger.form')}
      onSubmit={(e) => e.preventDefault()}
    >
      {isApi && (
        <>
          <label className="hx-trg__field">
            <span className="hx-conn-form__label">{t('next.connections.trigger.apiPath')}</span>
            <input
              className="hx-set__input"
              value={apiPath}
              onChange={(e) => setApiPath(e.target.value)}
              placeholder="/v1/items"
            />
          </label>
          <label className="hx-trg__field">
            <span className="hx-conn-form__label">{t('next.connections.trigger.apiQuery')}</span>
            <textarea
              className="hx-set__input"
              rows={2}
              value={apiQuery}
              onChange={(e) => setApiQuery(e.target.value)}
              placeholder={'since={{cursor}}\nstatus=done'}
            />
          </label>
        </>
      )}
      <p className="hx-trg__tool" hidden={isApi}>
        {t('next.connections.trigger.tool')}: <code>{candidate.tool}</code>
        {candidate.gateway && ` · ${t('next.connections.trigger.via', { call: candidate.gateway.call })}`}
      </p>
      {candidate.args
        .filter((a) => !a.cursor)
        .map((a) => (
          <label key={a.name} className="hx-trg__field">
            <span className="hx-conn-form__label">
              {a.name}
              {a.required ? ' *' : ''}
            </span>
            <input
              className="hx-set__input"
              value={args[a.name] ?? ''}
              placeholder={a.description}
              onChange={(e) => setArgs({ ...args, [a.name]: e.target.value })}
            />
          </label>
        ))}
      {cursorArg && (
        <p className="hx-trg__note">{t('next.connections.trigger.cursor', { arg: cursorArg.name })}</p>
      )}
      {!candidate.readOnly && (
        <label className="hx-trg__check">
          <input type="checkbox" checked={unmarked} onChange={(e) => setUnmarked(e.target.checked)} />
          {t('next.connections.trigger.unmarked')}
        </label>
      )}
      <div className="hx-svc-manage__buttons" hidden={isApi}>
        <SetButton
          tone="ghost"
          busy={busy === 'draft'}
          disabled={missing.length > 0 || busy !== null}
          onClick={() => void draft()}
        >
          {t('next.connections.trigger.draft')}
        </SetButton>
      </div>
      <fieldset className="hx-trg__paths">
        <legend className="hx-conn-form__label">{t('next.connections.trigger.paths')}</legend>
        <label className="hx-trg__field">
          <span className="hx-conn-form__label">trigger</span>
          <input
            className="hx-set__input"
            value={trigger}
            onChange={(e) => setTrigger(e.target.value)}
            placeholder="message.new"
          />
        </label>
        {PATHS.map((p) => (
          <label key={p} className="hx-trg__field">
            <span className="hx-conn-form__label">{p}</span>
            <input
              className="hx-set__input"
              value={paths[p]}
              onChange={(e) => setPaths({ ...paths, [p]: e.target.value })}
            />
          </label>
        ))}
      </fieldset>
      <label className="hx-trg__field">
        <span className="hx-conn-form__label">{t('next.connections.trigger.every')}</span>
        <select className="hx-set__input" value={every} onChange={(e) => setEvery(e.target.value)}>
          {EVERY.map((v) => (
            <option key={v} value={v}>
              {v}
            </option>
          ))}
        </select>
      </label>
      <div className="hx-trg__filter">
        <label className="hx-trg__field">
          <span className="hx-conn-form__label">{t('next.connections.trigger.filterPath')}</span>
          <input
            className="hx-set__input"
            value={filterPath}
            onChange={(e) => setFilterPath(e.target.value)}
            placeholder="message"
          />
        </label>
        <label className="hx-trg__field">
          <span className="hx-conn-form__label">{t('next.connections.trigger.filterText')}</span>
          <input
            className="hx-set__input"
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
          />
        </label>
      </div>
      <div className="hx-trg__filter">
        <label className="hx-trg__field">
          <span className="hx-conn-form__label">{t('next.connections.trigger.watch')}</span>
          <input
            className="hx-set__input"
            value={watch}
            onChange={(e) => setWatch(e.target.value)}
            placeholder="status"
          />
        </label>
        <label className="hx-trg__field">
          <span className="hx-conn-form__label">{t('next.connections.trigger.facts')}</span>
          <input
            className="hx-set__input"
            value={facts}
            onChange={(e) => setFacts(e.target.value)}
            placeholder="status, model"
          />
        </label>
      </div>
      <div className="hx-trg__filter">
        <label className="hx-trg__field">
          <span className="hx-conn-form__label">{t('next.connections.trigger.nextPath')}</span>
          <input
            className="hx-set__input"
            value={nextPath}
            onChange={(e) => setNextPath(e.target.value)}
            placeholder="next_cursor"
          />
        </label>
        <label className="hx-trg__field">
          <span className="hx-conn-form__label">{t('next.connections.trigger.nextArg')}</span>
          <input
            className="hx-set__input"
            value={nextArg}
            onChange={(e) => setNextArg(e.target.value)}
            placeholder="cursor"
          />
        </label>
      </div>
      <label className="hx-trg__field">
        <span className="hx-conn-form__label">{t('next.connections.trigger.script')}</span>
        <textarea
          className="hx-set__input hx-trg__script"
          rows={4}
          spellCheck={false}
          value={script}
          onChange={(e) => setScript(e.target.value)}
          placeholder="(answer, memo) => ({ items: answer.data, memo })"
        />
        <span className="hx-trg__note">{t('next.connections.trigger.scriptHint')}</span>
      </label>
      <div className="hx-trg__modes" role="radiogroup" aria-labelledby={`${uid}-mode`}>
        <span id={`${uid}-mode`} className="hx-conn-form__label">
          {t('next.connections.trigger.mode')}
        </span>
        {MODES.map((m) => (
          <label key={m} className="hx-trg__mode">
            <input type="radio" name={`${uid}-mode`} checked={mode === m} onChange={() => setMode(m)} />
            {t(`next.connections.trigger.mode.${m}`)}
          </label>
        ))}
      </div>
      <label className="hx-trg__field">
        <span className="hx-conn-form__label">{t('next.connections.trigger.name')}</span>
        <input className="hx-set__input" value={name} onChange={(e) => setName(e.target.value)} />
      </label>
      {error !== null && <FormError>{error}</FormError>}
      {samples !== null && <SampleList samples={samples} />}
      <div className="hx-svc-manage__buttons">
        <SetButton
          tone="ghost"
          busy={busy === 'test'}
          disabled={!ready || busy !== null}
          onClick={() => void test()}
        >
          {t('next.connections.trigger.test')}
        </SetButton>
        <SetButton
          tone="primary"
          busy={busy === 'save'}
          disabled={!ready || busy !== null}
          onClick={() => void save()}
        >
          {t('next.connections.trigger.save')}
        </SetButton>
        <SetButton tone="ghost" onClick={onClose}>
          {t('next.connections.trigger.close')}
        </SetButton>
      </div>
    </form>
  );
};

/** An API connection's trigger has no tool: the form asks a GET path. */
export const API_CANDIDATE: ICandidate = {
  tool: '',
  gateway: null,
  description: '',
  score: 0,
  fits: true,
  excluded: false,
  plus: [],
  minus: [],
  label: 'New things of the API',
  labelRu: 'Новое в API',
  readOnly: true,
  args: [],
};

/** The chips of the tools that can be a trigger, and the form of the one chosen; an API's form at once. */
const TriggerPanel = ({ api, isApi = false }: { api: ITriggerApi; isApi?: boolean }) => {
  const [cands, setCands] = useState<readonly ICandidate[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  useEffect(() => {
    if (isApi) return;
    let live = true;
    void api.load().then((res) => {
      if (!live) return;
      if (res.ok) setCands(res.value.filter((c) => c.fits));
      else setError(res.error);
    });
    return () => {
      live = false;
    };
  }, [api, isApi]);
  if (isApi)
    return (
      <section className="hx-svc-sec hx-trg" aria-label={t('next.connections.trigger.title')}>
        {open === null ? (
          <SetButton tone="primary" onClick={() => setOpen('api')}>
            {t('next.connections.trigger.make')}
          </SetButton>
        ) : (
          <TriggerForm candidate={API_CANDIDATE} api={api} onClose={() => setOpen(null)} />
        )}
      </section>
    );
  if (error !== null || cands === null || cands.length === 0) return null;
  const chosen = cands.find((c) => c.tool === open) ?? null;
  return (
    <section className="hx-svc-sec hx-trg" aria-label={t('next.connections.trigger.title')}>
      <div className="hx-trg__chips">
        <span className="hx-trg__lead">{t('next.connections.trigger.can')}</span>
        {cands.map((c) => (
          <button
            key={`${c.gateway?.server ?? ''}/${c.tool}`}
            type="button"
            className="hx-trg__chip"
            aria-pressed={open === c.tool}
            title={[c.tool, c.description].filter(Boolean).join(' — ')}
            onClick={() => setOpen(open === c.tool ? null : c.tool)}
          >
            {labelOf(c)}
          </button>
        ))}
      </div>
      {chosen === null ? (
        <SetButton tone="primary" onClick={() => setOpen(cands[0].tool)}>
          {t('next.connections.trigger.make')}
        </SetButton>
      ) : (
        <TriggerForm key={chosen.tool} candidate={chosen} api={api} onClose={() => setOpen(null)} />
      )}
    </section>
  );
};

export default TriggerPanel;
