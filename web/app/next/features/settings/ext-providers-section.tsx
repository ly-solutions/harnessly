// Settings → «Провайдеры» → «По API»: the model providers a person connects with a key. One form adds one: a
// preset asks only for the key, «Свой адрес» also for the name, the address and the kind of API. The form checks the
// connection before it adds, and says the provider's refusal in words. A row opens the provider: its models with a
// way to ask for them again, the check, the fields to change and the removal. Every list and line here is what the
// machine answered; nothing moves before it does.
import { useState, type FormEvent } from 'react';
import EnvEditor, { envDirty, inputsOf, rowsOf, type IEnvRow } from '@/next/features/settings/ext-env-editor';
import {
  EXT_API,
  type IExtEnvInput,
  type IExtPreset,
  type IExtProvider,
  type TExtApi,
} from '@/next/features/settings/ext-providers-api';
import LimitsLine from '@/next/features/limits/limits-line';
import { API_PROFILE, profileLimitsOf, type MachineLimits } from '@/next/features/limits/model';
import { FormError, SegControl, SetButton } from '@/next/features/settings/rows';
import { plural, t } from '@/next/i18n';
import Icon from '@/next/ui/icon';
import LoadFailed, { LOAD_FAILED_SIZE } from '@/next/ui/load-failed';
import { usePresence } from '@/next/ui/presence';
import SecretField from '@/next/ui/secret-field';
import { Skeleton } from '@/next/ui/skeleton';

/** The preset of an address of the person's own (extproviders.PresetCustom). */
export const CUSTOM_PRESET = 'custom';

/** A card's exit, as long as hx-extp-out in ext-providers.css. */
const CARD_EXIT_MS = 180;

/** Every action resolves null when the machine took it, else the words of why not. */
type TDone = Promise<string | null>;

export interface IExtAdd {
  preset: IExtPreset;
  name: string;
  baseUrl: string;
  api: TExtApi;
  key: string;
}

export interface IExtEdit {
  name: string;
  /** Absent for a provider of a preset: its address and API are the preset's. */
  baseUrl?: string;
  api?: TExtApi;
  /** '' keeps the key the safe has. */
  key: string;
  /** The whole list of the launch's variables; absent when they were not touched. */
  env?: readonly IExtEnvInput[];
}

export type TExtProvidersState =
  | { k: 'loading' }
  | { k: 'failed'; text: string; onRetry: () => void }
  | {
      k: 'ready';
      providers: readonly IExtProvider[];
      presets: readonly IExtPreset[];
      /** The machine keeps several accounts of one provider. */
      accounts: boolean;
      /** The machine starts Claude Code chats on an Anthropic-compatible provider. */
      launch: boolean;
    };

interface IExtProvidersSectionProps {
  state: TExtProvidersState;
  /** The machine's limits: a provider whose vendor tells the balance of a key shows it in its row. */
  limits?: MachineLimits | null;
  /** What each provider does now, by its id: the titles of the features given to it. null: not known yet. */
  usage: Readonly<Record<string, readonly string[]>> | null;
  /** Checks the connection and adds the provider: its id, or the words of why not. */
  onAdd: (draft: IExtAdd) => Promise<{ id: string } | { error: string }>;
  onSave: (id: string, edit: IExtEdit) => TDone;
  onRemove: (id: string) => TDone;
  /** Makes the account the default one of its provider. */
  onDefault: (id: string) => TDone;
  /** The check of a kept provider: whether it answered, and what to say. */
  onCheck: (id: string) => Promise<{ ok: boolean; text: string }>;
  /** Asks the provider for its models again. */
  onRefresh: (id: string) => TDone;
}

const apiChoices = () => [
  { value: EXT_API.openai, label: t('next.extProviders.api.openai') },
  { value: EXT_API.anthropic, label: t('next.extProviders.api.anthropic') },
];

const presetName = (preset: IExtPreset): string =>
  preset.id === CUSTOM_PRESET ? t('next.extProviders.custom') : preset.name;

/** The line under a provider's name: how many models it has and what it does. */
const lineOf = (provider: IExtProvider, used: readonly string[] | null): string => {
  const models =
    provider.models.length === 0
      ? t('next.extProviders.noModels')
      : plural(provider.models.length, 'next.extProviders.models');
  if (used === null) return models;
  const what = used.length === 0 ? t('next.extProviders.unused') : used.join(', ').toLocaleLowerCase();
  return `${models} · ${what}`;
};

const hostOf = (address: string): string | null => {
  try {
    return new URL(address.trim()).host;
  } catch {
    return null;
  }
};

/** The provider a new account is added to: the form opens on it. */
interface IAddStart {
  preset: string;
  baseUrl: string;
  api: TExtApi;
}

interface IAddFormProps {
  presets: readonly IExtPreset[];
  /** The presets the person has an account of: a new one is told apart by its name. */
  connected: ReadonlySet<string>;
  start: IAddStart | null;
  closing: boolean;
  onAdd: (draft: IExtAdd) => TDone;
  onClose: () => void;
}

const AddForm = ({ presets, connected, start, closing, onAdd, onClose }: IAddFormProps) => {
  const [presetId, setPresetId] = useState(start?.preset ?? presets[0]?.id ?? CUSTOM_PRESET);
  const [name, setName] = useState('');
  const [baseUrl, setBaseUrl] = useState(start?.preset === CUSTOM_PRESET ? start.baseUrl : '');
  const [api, setApi] = useState<TExtApi>(start?.api ?? EXT_API.openai);
  const [key, setKey] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const preset = presets.find((p) => p.id === presetId) ?? presets[0];
  if (!preset) return null;
  const custom = preset.id === CUSTOM_PRESET;
  // One more account of a provider the person has: it may be given a name of its own (the machine numbers it if not).
  const another = !custom && connected.has(preset.id);
  const ready = key.trim() !== '' && (!custom || (name.trim() !== '' && baseUrl.trim() !== ''));

  const submit = async (event?: FormEvent) => {
    event?.preventDefault();
    if (!ready || busy) return;
    setBusy(true);
    setError(null);
    const refused = await onAdd({
      preset,
      name: custom || (another && name.trim() !== '') ? name.trim() : preset.name,
      baseUrl: custom ? baseUrl.trim() : preset.baseUrl,
      api: custom ? api : preset.api,
      key: key.trim(),
    });
    setBusy(false);
    setError(refused);
  };

  return (
    <form
      className="hx-extp__card"
      data-state={closing ? 'closing' : undefined}
      aria-busy={busy || undefined}
      onSubmit={(event) => void submit(event)}
    >
      <h3 className="hx-extp__h">
        {another
          ? t('next.extProviders.add.another', { name: preset.name })
          : t('next.extProviders.add.title')}
      </h3>
      <div className="hx-extp__presets" role="radiogroup" aria-label={t('next.extProviders.add.preset')}>
        {presets.map((p) => (
          <button
            key={p.id}
            type="button"
            className="hx-extp__preset"
            role="radio"
            aria-checked={p.id === preset.id}
            disabled={busy}
            onClick={() => {
              setPresetId(p.id);
              setError(null);
            }}
          >
            {presetName(p)}
          </button>
        ))}
      </div>
      {custom && (
        <>
          <div className="hx-extp__two">
            <label className="hx-extp__fld">
              <span className="hx-extp__label">{t('next.extProviders.name')}</span>
              <input
                className="hx-set__input"
                value={name}
                disabled={busy}
                maxLength={60}
                placeholder={t('next.extProviders.name.placeholder')}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label className="hx-extp__fld">
              <span className="hx-extp__label">{t('next.extProviders.address')}</span>
              <input
                className="hx-set__input hx-set__input--mono"
                value={baseUrl}
                disabled={busy}
                autoCapitalize="off"
                spellCheck={false}
                placeholder="https://llm.example.ru/v1"
                onChange={(event) => setBaseUrl(event.target.value)}
              />
            </label>
          </div>
          <div className="hx-extp__fld">
            <span className="hx-extp__label">{t('next.extProviders.api')}</span>
            <SegControl
              label={t('next.extProviders.api')}
              value={api}
              choices={apiChoices()}
              onPick={setApi}
              disabled={busy}
            />
            <small className="hx-extp__hint">{t('next.extProviders.api.hint')}</small>
          </div>
        </>
      )}
      {another && (
        <label className="hx-extp__fld">
          <span className="hx-extp__label">{t('next.extProviders.account')}</span>
          <input
            className="hx-set__input"
            value={name}
            disabled={busy}
            maxLength={60}
            placeholder={t('next.extProviders.account.placeholder')}
            onChange={(event) => setName(event.target.value)}
          />
          <small className="hx-extp__hint">{t('next.extProviders.account.hint')}</small>
        </label>
      )}
      <div className="hx-extp__fld">
        <span className="hx-extp__label">{t('next.extProviders.key')}</span>
        <SecretField
          value={key}
          onChange={setKey}
          label={t('next.extProviders.key')}
          placeholder={
            custom
              ? t('next.extProviders.key.placeholder')
              : t('next.extProviders.key.placeholderOf', { name: preset.name })
          }
          disabled={busy}
          invalid={error !== null}
          onSubmit={() => void submit()}
        />
        <small className="hx-extp__hint">
          {custom
            ? t('next.extProviders.key.hint')
            : t('next.extProviders.key.hintOf', { name: preset.name })}
          {!custom && preset.keyUrl !== '' && (
            <>
              {' '}
              <a className="hx-extp__link" href={preset.keyUrl} target="_blank" rel="noreferrer">
                {t('next.extProviders.key.where')}
              </a>
            </>
          )}
        </small>
      </div>
      {error && <FormError>{error}</FormError>}
      <div className="hx-extp__btns">
        {busy ? (
          <span className="hx-extp__checking" role="status">
            <Icon name="loader-circle" size="sm" className="hx-extp__spin" />
            {t('next.extProviders.add.checking')}
          </span>
        ) : (
          <>
            <SetButton tone="primary" disabled={!ready} onClick={() => void submit()}>
              {t('next.extProviders.add.submit')}
            </SetButton>
            <SetButton tone="ghost" onClick={onClose}>
              {t('next.extProviders.cancel')}
            </SetButton>
          </>
        )}
      </div>
    </form>
  );
};

const EDIT_BUSY = {
  save: 'save',
  check: 'check',
  refresh: 'refresh',
  remove: 'remove',
  primary: 'primary',
} as const;
type TEditBusy = (typeof EDIT_BUSY)[keyof typeof EDIT_BUSY];

interface IEditCardProps extends Pick<
  IExtProvidersSectionProps,
  'onSave' | 'onRemove' | 'onDefault' | 'onCheck' | 'onRefresh'
> {
  provider: IExtProvider;
  /** The provider has other accounts: this one may be made the default, and says so when it is. */
  several: boolean;
  /** The features that run on this account, in words; '' when none does. */
  used: string;
  closing: boolean;
  onClose: () => void;
  /** Opens the form of one more account of this provider; absent where the machine keeps one. */
  onAnother?: () => void;
  /** Claude Code chats start on this provider: the variables of the launch are edited here. */
  launch: boolean;
}

const EditCard = ({
  provider,
  several,
  used,
  closing,
  onSave,
  onRemove,
  onDefault,
  onCheck,
  onRefresh,
  onClose,
  onAnother,
  launch,
}: IEditCardProps) => {
  const custom = provider.preset === '' || provider.preset === CUSTOM_PRESET;
  const managed = provider.managed;
  const [name, setName] = useState(provider.name);
  const [baseUrl, setBaseUrl] = useState(provider.baseUrl);
  const [api, setApi] = useState<TExtApi>(provider.api);
  const [key, setKey] = useState('');
  const [busy, setBusy] = useState<TEditBusy | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [checked, setChecked] = useState<{ ok: boolean; text: string } | null>(null);
  const [sure, setSure] = useState(false);
  // The variables as they are being edited; null: as the machine keeps them.
  const [edited, setEdited] = useState<IEnvRow[] | null>(null);
  const env = edited ?? rowsOf(provider.env);
  const envChanged = edited !== null && envDirty(edited, provider.env);

  const moved = custom && hostOf(baseUrl) !== provider.host;
  const dirty =
    envChanged ||
    name.trim() !== provider.name ||
    key.trim() !== '' ||
    (custom && (baseUrl.trim() !== provider.baseUrl || api !== provider.api));

  const run = async (kind: TEditBusy, action: () => TDone) => {
    if (busy !== null) return;
    setBusy(kind);
    setError(null);
    setChecked(null);
    const refused = await action();
    setBusy(null);
    setError(refused);
    return refused;
  };
  const save = async (event?: FormEvent) => {
    event?.preventDefault();
    if (!dirty) return;
    const refused = await run(EDIT_BUSY.save, () =>
      onSave(provider.id, {
        name: name.trim(),
        key: key.trim(),
        ...(custom ? { baseUrl: baseUrl.trim(), api } : {}),
        ...(envChanged ? { env: inputsOf(env) } : {}),
      }),
    );
    if (refused === null) {
      setKey('');
      // What was typed for a secret goes with the form: the machine has it now.
      setEdited(null);
    }
  };
  const check = async () => {
    if (busy !== null) return;
    setBusy(EDIT_BUSY.check);
    setError(null);
    setChecked(null);
    const found = await onCheck(provider.id);
    setBusy(null);
    setChecked(found);
  };

  let keyHint = t('next.extProviders.key.kept');
  if (!provider.hasKey) keyHint = t('next.extProviders.key.none');
  if (moved) keyHint = t('next.extProviders.key.moved');

  return (
    <form
      className="hx-extp__card"
      data-state={closing ? 'closing' : undefined}
      aria-busy={busy !== null || undefined}
      onSubmit={(event) => void save(event)}
    >
      <div className="hx-extp__head">
        <h3 className="hx-extp__h">{provider.name}</h3>
        <button
          type="button"
          className="hx-head-btn"
          aria-label={t('next.extProviders.close')}
          onClick={onClose}
        >
          <Icon name="x" size="sm" />
        </button>
      </div>
      {managed && (
        <small className="hx-extp__hint">{t('next.extProviders.managedHint', { host: provider.host })}</small>
      )}
      {!managed && (
        <>
          <div className={custom ? 'hx-extp__two' : undefined}>
            <label className="hx-extp__fld">
              <span className="hx-extp__label">{t('next.extProviders.name')}</span>
              <input
                className="hx-set__input"
                value={name}
                maxLength={60}
                disabled={busy !== null}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            {custom && (
              <label className="hx-extp__fld">
                <span className="hx-extp__label">{t('next.extProviders.address')}</span>
                <input
                  className="hx-set__input hx-set__input--mono"
                  value={baseUrl}
                  autoCapitalize="off"
                  spellCheck={false}
                  disabled={busy !== null}
                  onChange={(event) => setBaseUrl(event.target.value)}
                />
              </label>
            )}
          </div>
          {custom && (
            <div className="hx-extp__fld">
              <span className="hx-extp__label">{t('next.extProviders.api')}</span>
              <SegControl
                label={t('next.extProviders.api')}
                value={api}
                choices={apiChoices()}
                onPick={setApi}
                disabled={busy !== null}
              />
            </div>
          )}
          <div className="hx-extp__fld">
            <span className="hx-extp__label">{t('next.extProviders.key')}</span>
            <SecretField
              value={key}
              onChange={setKey}
              label={t('next.extProviders.key')}
              placeholder={
                provider.hasKey ? t('next.extProviders.key.replace') : t('next.extProviders.key.placeholder')
              }
              disabled={busy !== null}
              onSubmit={() => void save()}
            />
            <small className="hx-extp__hint" data-warn={moved || !provider.hasKey || undefined}>
              {keyHint}
            </small>
          </div>
        </>
      )}
      <div className="hx-extp__fld">
        <span className="hx-extp__label">{t('next.extProviders.modelsTitle')}</span>
        {provider.models.length === 0 ? (
          <small className="hx-extp__hint">{t('next.extProviders.modelsEmpty')}</small>
        ) : (
          <ul className="hx-extp__models">
            {provider.models.map((model) => (
              <li key={model.id} className="hx-extp__model">
                {model.label}
              </li>
            ))}
          </ul>
        )}
      </div>
      {launch && !managed && <EnvEditor rows={env} disabled={busy !== null} onChange={setEdited} />}
      {sure && used !== '' && (
        <small className="hx-extp__hint" data-warn>
          {t(several ? 'next.extProviders.removeUsedSeveral' : 'next.extProviders.removeUsed', { used })}
        </small>
      )}
      {error && <FormError>{error}</FormError>}
      {checked && (
        <p className="hx-extp__checked" data-ok={checked.ok} role="status">
          <Icon name={checked.ok ? 'circle-check' : 'triangle-alert'} size="sm" />
          {checked.text}
        </p>
      )}
      <div className="hx-extp__btns">
        {!managed && (
          <SetButton
            tone="primary"
            busy={busy === EDIT_BUSY.save}
            disabled={!dirty || busy !== null}
            onClick={() => void save()}
          >
            {t('next.extProviders.save')}
          </SetButton>
        )}
        <SetButton
          tone="ghost"
          busy={busy === EDIT_BUSY.check}
          disabled={busy !== null || !provider.hasKey}
          onClick={() => void check()}
        >
          {t('next.extProviders.check')}
        </SetButton>
        <SetButton
          tone="ghost"
          icon="rotate-ccw"
          busy={busy === EDIT_BUSY.refresh}
          disabled={busy !== null || !provider.hasKey}
          onClick={() => void run(EDIT_BUSY.refresh, () => onRefresh(provider.id))}
        >
          {t('next.extProviders.refresh')}
        </SetButton>
        {several && !provider.isDefault && (
          <SetButton
            tone="ghost"
            busy={busy === EDIT_BUSY.primary}
            disabled={busy !== null}
            onClick={() => void run(EDIT_BUSY.primary, () => onDefault(provider.id))}
          >
            {t('next.extProviders.makeDefault')}
          </SetButton>
        )}
        {onAnother && (
          <SetButton tone="ghost" icon="plus" disabled={busy !== null} onClick={onAnother}>
            {t('next.extProviders.another')}
          </SetButton>
        )}
        <span className="hx-extp__gap" />
        {!managed && (
          <SetButton
            tone="danger"
            busy={busy === EDIT_BUSY.remove}
            disabled={busy !== null}
            onClick={() => {
              if (!sure) {
                setSure(true);
                return;
              }
              void run(EDIT_BUSY.remove, () => onRemove(provider.id));
            }}
          >
            {sure ? t('next.extProviders.removeSure') : t('next.extProviders.remove')}
          </SetButton>
        )}
      </div>
    </form>
  );
};

const ExtProvidersSection = ({
  state,
  limits,
  usage,
  onAdd,
  onSave,
  onRemove,
  onDefault,
  onCheck,
  onRefresh,
}: IExtProvidersSectionProps) => {
  // What is open under the list: the form of a new provider (or of one more account of one), or a provider by its id.
  const [open, setOpen] = useState<{ k: 'add'; start: IAddStart | null } | { k: 'edit'; id: string } | null>(
    null,
  );
  // The provider the form has just added: its row comes in marked, once.
  const [fresh, setFresh] = useState<string | null>(null);
  const providers = state.k === 'ready' ? state.providers : [];
  const edited = open?.k === 'edit' ? (providers.find((p) => p.id === open.id) ?? null) : null;
  const adding = usePresence(open?.k === 'add' ? open : null, CARD_EXIT_MS);
  const editing = usePresence(edited, CARD_EXIT_MS);

  const foot = t('next.extProviders.foot');
  if (state.k === 'loading')
    return (
      <div className="hx-set__group">
        <div className="hx-set__group-title">{t('next.extProviders.title')}</div>
        <Skeleton className="hx-set__sk" />
      </div>
    );
  if (state.k === 'failed')
    return (
      <div className="hx-set__group">
        <div className="hx-set__group-title">{t('next.extProviders.title')}</div>
        <div className="hx-set__card hx-extp__failed">
          <LoadFailed
            size={LOAD_FAILED_SIZE.inline}
            title={t('next.extProviders.loadFailed')}
            reason={state.text}
            onRetry={state.onRetry}
          />
        </div>
      </div>
    );

  // A machine that keeps one account of a provider is not offered a connected preset again; an address of the
  // person's own always is.
  const taken = new Set(providers.map((p) => p.preset));
  const presets = state.accounts
    ? state.presets
    : state.presets.filter((p) => p.id === CUSTOM_PRESET || !taken.has(p.id));
  const accountsOf = (group: string) => providers.filter((p) => p.group === group).length;
  const severalOf = (provider: IExtProvider) => state.accounts && accountsOf(provider.group) > 1;
  const usedBy = (provider: IExtProvider) => (usage?.[provider.id] ?? []).join(', ').toLocaleLowerCase();

  return (
    <div className="hx-set__group">
      <div className="hx-set__group-title">{t('next.extProviders.title')}</div>
      {providers.length === 0 ? (
        <div className="hx-set__card">
          <div className="hx-set__empty">{t('next.extProviders.empty')}</div>
        </div>
      ) : (
        <div className="hx-set__card">
          {providers.map((provider) => (
            <button
              key={provider.id}
              type="button"
              className="hx-set__row hx-extp__row"
              data-fresh={provider.id === fresh || undefined}
              aria-expanded={edited?.id === provider.id}
              onClick={() =>
                setOpen((now) =>
                  now?.k === 'edit' && now.id === provider.id ? null : { k: 'edit', id: provider.id },
                )
              }
            >
              <span className="hx-extp__logo" aria-hidden="true">
                {provider.name.slice(0, 1).toLocaleUpperCase()}
              </span>
              <span className="hx-set__row-text">
                <span className="hx-set__row-label">
                  {provider.name}
                  {provider.managed && <span className="hx-extp__tag">{t('next.extProviders.managed')}</span>}
                  {severalOf(provider) && provider.isDefault && (
                    <span className="hx-extp__tag">{t('next.extProviders.default')}</span>
                  )}
                </span>
                <span className="hx-set__row-hint">
                  {provider.hasKey
                    ? lineOf(provider, usage === null ? null : (usage[provider.id] ?? []))
                    : t('next.extProviders.noKey')}
                </span>
              </span>
              <LimitsLine
                limits={
                  limits && provider.preset
                    ? profileLimitsOf(limits, `${API_PROFILE}${provider.preset}`, provider.id)
                    : null
                }
              />
              <Icon name="chevron-right" size="sm" className="hx-extp__go" />
            </button>
          ))}
        </div>
      )}
      {editing.value && (
        <EditCard
          key={editing.value.id}
          provider={editing.value}
          launch={state.launch && editing.value.api === EXT_API.anthropic}
          several={severalOf(editing.value)}
          used={usedBy(editing.value)}
          closing={editing.state === 'closing'}
          onAnother={
            state.accounts
              ? () => {
                  const from = editing.value;
                  if (!from) return;
                  setOpen({
                    k: 'add',
                    start: { preset: from.preset || CUSTOM_PRESET, baseUrl: from.baseUrl, api: from.api },
                  });
                }
              : undefined
          }
          onDefault={onDefault}
          onSave={onSave}
          onCheck={onCheck}
          onRefresh={onRefresh}
          onRemove={onRemove}
          onClose={() => setOpen(null)}
        />
      )}
      {adding.value && (
        <AddForm
          presets={presets}
          connected={state.accounts ? taken : new Set()}
          start={adding.value.start}
          closing={adding.state === 'closing'}
          onAdd={async (draft) => {
            const made = await onAdd(draft);
            if ('error' in made) return made.error;
            setFresh(made.id);
            setOpen(null);
            return null;
          }}
          onClose={() => setOpen(null)}
        />
      )}
      {open === null && !adding.value && !editing.value && (
        <div className="hx-extp__add">
          <SetButton tone="ghost" icon="plus" onClick={() => setOpen({ k: 'add', start: null })}>
            {t('next.extProviders.add')}
          </SetButton>
        </div>
      )}
      <div className="hx-set__foot">{foot}</div>
    </div>
  );
};

export default ExtProvidersSection;
