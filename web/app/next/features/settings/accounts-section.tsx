// The agents' sign-in. «Провайдеры» lists the agents (a row leads to the agent's page); the agent's own page has
// «Подключение»: how it is signed in on the machine and where its requests go. Everything is entered there: a subscription is signed in through a login the machine runs (its link and code are on the card), a
// key goes into a write-only field. The page never holds a saved key, only its last digits; the address of the requests
// is a setting of the CLI's file and is changed here.
import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import LoginPanel from '@/next/features/accounts/login-panel';
import SeenAccounts from '@/next/features/accounts/seen-accounts';
import {
  METHODS_OF,
  type ICredentialActions,
  type ISavedKey,
  type LoginMethod,
  type LoginVia,
  type LoginState,
} from '@/next/features/accounts/model';
import KeyPanel from '@/next/features/providers/key-panel';
import {
  vendorsOf,
  type IPresetActions,
  type IPresetState,
  type PresetId,
} from '@/next/features/providers/model';
import GptSwitch from '@/next/features/providers/gpt-switch';
import SpeechKeyCards, { type SpeechCards } from '@/next/features/providers/speech-cards';
import VendorCard from '@/next/features/providers/vendor-card';
import LimitsLine from '@/next/features/limits/limits-line';
import type { MachineLimits } from '@/next/features/limits/model';
import { optionValue } from '@/next/features/settings/options';
import { FormError, Group, SetButton } from '@/next/features/settings/rows';
import { t } from '@/next/i18n';
import type { AgentSettings, Provider } from '@/next/protocol/v2';
import Icon from '@/next/ui/icon';
import Segmented from '@/next/ui/segmented';
import LoadFailed, { LOAD_FAILED_SIZE } from '@/next/ui/load-failed';
import { Skeleton } from '@/next/ui/skeleton';

/** Where each CLI keeps the address its requests go to. */
const BASE_URL_AT: Readonly<Record<Provider, string>> = {
  claude: 'ANTHROPIC_BASE_URL',
  codex: 'openai_base_url',
};

/** An address the CLI can send requests to: http(s) with a host. */
export const isProxyUrl = (value: string): boolean => {
  try {
    const url = new URL(value);
    return (url.protocol === 'https:' || url.protocol === 'http:') && url.host !== '' && !url.username;
  } catch {
    return false;
  }
};

/** Where a card's requests go. */
const WHERE = { direct: 'direct', proxy: 'proxy' } as const;

export type AccountCard =
  | { provider: Provider; k: 'loading' }
  | { provider: Provider; k: 'failed'; text: string }
  | { provider: Provider; k: 'ready'; settings: AgentSettings };

/** What the card of an agent takes and does with its credentials. */
export interface IAgentCredentials extends ICredentialActions {
  login: LoginState;
  key: ISavedKey;
  /** The accounts the cluster saw this agent signed in with on this machine, newest first; undefined while unread. */
  seen: readonly string[] | undefined;
}

/** The vendors' cards (Kimi, GLM, Grok): the presets as the runtime lists them, or why it has none to show. */
export type VendorCards =
  | { k: 'loading' }
  | { k: 'failed'; text: string; onRetry: () => void }
  | ({
      k: 'ready';
      presets: readonly IPresetState[];
      /** Turns a switchable profile on or off (GPT in Claude Code); resolves null when the machine did it. */
      onSwitch: (preset: PresetId, on: boolean) => Promise<string | null>;
    } & IPresetActions);

export interface IAccountsSectionProps {
  /** The agents of the machine: how each is signed in. Signing in is on the agent's own page. */
  cards: readonly AccountCard[];
  /** The keys of the cloud recognition models. */
  speech: SpeechCards;
  /** Opens the agent's page, where it is signed in. */
  onOpen: (provider: Provider) => void;
  /** What is left of each account's subscription, as the machine says it; absent or null while it has not. */
  limits?: MachineLimits | null;
}

/** «Подключение» on an agent's own page: how it is signed in and where its requests go. */
export interface IAgentConnectionProps {
  settings: AgentSettings;
  credentials: IAgentCredentials;
  /** The address of the requests is being written. */
  busy: boolean;
  /** The `gpt` profile and its switch, for Codex; null on the other pages. */
  gpt: { preset: IPresetState | undefined; onSwitch: (on: boolean) => Promise<string | null> } | null;
  /** Resolves true when the machine took the new address (null: the requests go direct). */
  onBaseUrl: (provider: Provider, url: string | null) => Promise<boolean>;
}

const statusText = (settings: AgentSettings): string => {
  const { account } = settings;
  if (!account.signedIn || !account.method) return t('next.settings.accounts.off');
  const how =
    account.method === 'subscription'
      ? t('next.settings.accounts.onSubscription', { plan: account.plan ?? '' }).trim()
      : t('next.settings.accounts.onKey');
  return account.who ? `${how} · ${account.who}` : how;
};

const SignIn = ({ settings, credentials }: { settings: AgentSettings; credentials: IAgentCredentials }) => {
  const { account, provider } = settings;
  const methods = METHODS_OF[provider];
  const [method, setMethod] = useState<LoginMethod>(
    account.method === 'key' && methods.some((m) => m === 'key') ? 'key' : 'subscription',
  );
  const name = t(`next.settings.accounts.name.${provider}`);
  const open = (value: LoginMethod) => ({ 'data-open': method === value }) as const;
  const inert = (value: LoginMethod) => (method !== value ? true : undefined);
  const panel = (via: LoginVia) => (
    <LoginPanel
      via={via}
      account={account}
      login={credentials.login}
      onLogin={credentials.onLogin}
      onSubmitCode={credentials.onSubmitCode}
      onCancel={credentials.onCancelLogin}
      onLogout={credentials.onLogout}
    />
  );
  return (
    <>
      <SeenAccounts seen={credentials.seen} />
      <div className="hx-acct__label">{t('next.settings.accounts.how')}</div>
      <div className="hx-acct__how">
        <Segmented
          label={t('next.settings.accounts.how')}
          value={method}
          options={methods.map((value) => ({
            value,
            label: t(`next.settings.accounts.method.${value}`),
          }))}
          onChange={setMethod}
        />
        <div className="hx-reveal" {...open('subscription')}>
          <div className="hx-reveal__in" inert={inert('subscription')}>
            {panel('subscription')}
          </div>
        </div>
        {provider === 'claude' && (
          <div className="hx-reveal" {...open('console')}>
            <div className="hx-reveal__in" inert={inert('console')}>
              {panel('console')}
            </div>
          </div>
        )}
        {provider === 'codex' && (
          <div className="hx-reveal" {...open('key')}>
            <div className="hx-reveal__in" inert={inert('key')}>
              <KeyPanel
                hint={t('next.settings.accounts.keyHint.codex')}
                label={t('next.providers.keyAria', { vendor: name })}
                saved={credentials.key}
                onSave={credentials.onSaveKey}
                onRemove={credentials.onLogout}
              />
            </div>
          </div>
        )}
      </div>
    </>
  );
};

interface IProxyProps {
  settings: AgentSettings;
  busy: boolean;
  onBaseUrl: IAgentConnectionProps['onBaseUrl'];
}

const Proxy = ({ settings, busy, onBaseUrl }: IProxyProps) => {
  const saved = optionValue(settings, 'baseUrl');
  const url = typeof saved === 'string' ? saved : null;
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [error, setError] = useState<string | null>(null);
  const on = url !== null || editing;
  const field = useRef<HTMLInputElement>(null);

  // The address field takes the cursor when it opens.
  useEffect(() => {
    if (editing) field.current?.focus();
  }, [editing]);

  const edit = () => {
    setDraft(url ?? '');
    setError(null);
    setEditing(true);
  };
  const direct = async () => {
    setEditing(false);
    setError(null);
    if (url !== null) await onBaseUrl(settings.provider, null);
  };
  const save = async () => {
    const next = draft.trim();
    if (!isProxyUrl(next)) {
      setError(t('next.settings.accounts.proxyBad'));
      return;
    }
    setError(null);
    if (await onBaseUrl(settings.provider, next)) setEditing(false);
  };
  const onKey = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') void save();
    if (event.key === 'Escape') setEditing(false);
  };

  return (
    <>
      <div className="hx-acct__label">{t('next.settings.accounts.where')}</div>
      <div className="hx-proxy" data-on={on}>
        <Segmented
          label={t('next.settings.accounts.where')}
          value={on ? WHERE.proxy : WHERE.direct}
          options={[
            { value: WHERE.direct, label: t('next.settings.accounts.direct') },
            { value: WHERE.proxy, label: t('next.settings.accounts.viaProxy') },
          ]}
          onChange={(value) => {
            if (value === WHERE.direct) void direct();
            else if (!on) edit();
          }}
        />
        <div className="hx-proxy__body">
          <div>
            {editing && (
              <>
                <div className="hx-proxy__edit">
                  <input
                    className="hx-set__input hx-set__input--mono"
                    type="url"
                    value={draft}
                    autoComplete="off"
                    spellCheck={false}
                    placeholder="https://llm.example.ru"
                    aria-label={t('next.settings.accounts.proxyAria')}
                    ref={field}
                    onChange={(event) => setDraft(event.target.value)}
                    onKeyDown={onKey}
                  />
                  <SetButton tone="ghost" onClick={() => setEditing(false)}>
                    {t('next.settings.cancel')}
                  </SetButton>
                  <SetButton tone="primary" busy={busy} onClick={() => void save()}>
                    {t('next.settings.save')}
                  </SetButton>
                </div>
                {error && <FormError>{error}</FormError>}
              </>
            )}
            {!editing && url !== null && (
              <div className="hx-proxy__url">
                <Icon name="globe" size="sm" />
                <code className="hx-trunc">{url}</code>
                <button type="button" className="hx-src__act" onClick={edit}>
                  {t('next.settings.accounts.change')}
                </button>
              </div>
            )}
            <div className="hx-proxy__env">
              <span>{t('next.settings.accounts.gets')}</span>
              <code className="hx-trunc">
                {BASE_URL_AT[settings.provider]}
                {settings.provider === 'claude' ? '=' : ' = '}
                {url ?? (draft || '…')}
              </code>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};

export const AgentConnection = ({ settings, credentials, busy, gpt, onBaseUrl }: IAgentConnectionProps) => (
  <Group
    title={t('next.settings.accounts.connection')}
    foot={
      settings.provider === 'claude'
        ? t('next.settings.accounts.connectionFootClaude')
        : t('next.settings.accounts.connectionFoot')
    }
  >
    <div className="hx-acct hx-acct--page" data-open data-on={settings.account.signedIn}>
      <div className="hx-acct__body">
        <div className="hx-acct__inner">
          <SignIn settings={settings} credentials={credentials} />
          <Proxy key={settings.provider} settings={settings} busy={busy} onBaseUrl={onBaseUrl} />
          {gpt && (
            <GptSwitch
              preset={gpt.preset}
              ready={settings.account.signedIn && settings.account.method === 'subscription'}
              onSwitch={gpt.onSwitch}
            />
          )}
        </div>
      </div>
    </div>
  </Group>
);

/** The `gpt` profile of the runtime's list and its switch, for the Codex page. */
export const gptOf = (vendors: Extract<VendorCards, { k: 'ready' }>) => {
  const preset = vendors.presets.find((p) => p.vendor === 'gpt');
  return {
    preset,
    onSwitch: (on: boolean) => (preset ? vendors.onSwitch(preset.id, on) : Promise.resolve(null)),
  };
};

/** The vendor profiles the auto profile routes between: it is offered once one of them has a key. */
const AUTO_TARGETS: readonly string[] = ['kimi-code', 'glm', 'glm-cn'];

/** «Авто» under the vendor cards: the switch of the `auto` profile; nothing from a runtime that has none. */
const AutoCard = ({ vendors }: { vendors: Extract<VendorCards, { k: 'ready' }> }) => {
  const preset = vendors.presets.find((p) => p.vendor === 'auto');
  if (!preset) return null;
  const ready = vendors.presets.some((p) => AUTO_TARGETS.includes(p.id) && p.key.configured);
  return (
    <div className="hx-acct hx-acct--solo hx-acct--switch">
      <GptSwitch
        kind="auto"
        preset={preset}
        ready={ready}
        onSwitch={(on) => vendors.onSwitch(preset.id, on)}
      />
    </div>
  );
};

/** The vendors whose models run in Claude Code in chats of their own (Kimi, GLM, Grok), on the Claude Code page. */
export const VendorKeys = ({ vendors }: { vendors: VendorCards }) => {
  const [open, setOpen] = useState<string | null>(null);
  return (
    <Group title={t('next.settings.accounts.vendors')} foot={t('next.settings.accounts.vendorsFoot')}>
      {vendors.k === 'ready' &&
        vendorsOf(vendors.presets).map((vendor) => (
          <VendorCard
            key={vendor}
            vendor={vendor}
            states={vendors.presets}
            open={open === vendor}
            onToggle={(card) => setOpen((now) => (now === card ? null : card))}
            onSave={vendors.onSave}
            onRemove={vendors.onRemove}
            onTest={vendors.onTest}
          />
        ))}
      {vendors.k === 'ready' && <AutoCard vendors={vendors} />}
      {vendors.k === 'loading' && <Skeleton className="hx-set__sk" />}
      {vendors.k === 'failed' && (
        <div className="hx-acct hx-acct--solo">
          <LoadFailed
            size={LOAD_FAILED_SIZE.inline}
            title={t('next.providers.loadFailed')}
            reason={vendors.text}
            onRetry={vendors.onRetry}
          />
        </div>
      )}
    </Group>
  );
};

const agentStatus = (card: AccountCard): string => {
  if (card.k === 'failed') return card.text;
  if (card.k === 'loading') return t('next.settings.loading');
  return statusText(card.settings);
};

/** «Провайдеры», its agents: who is signed in and how; a row leads to the agent's page, where it is signed in. */
const AccountsSection = ({ cards, speech, limits, onOpen }: IAccountsSectionProps) => (
  <>
    <Group title={t('next.settings.accounts.agents')} foot={t('next.settings.accounts.agentsFoot')}>
      {cards.map((card) => {
        const offered = card.k === 'ready' && !card.settings.account.signedIn;
        return (
          <button
            key={card.provider}
            type="button"
            className="hx-set__row hx-acct-row"
            onClick={() => onOpen(card.provider)}
          >
            <span className="hx-acct__logo">
              <i className="hx-agent" data-agent={card.provider} aria-hidden="true" />
            </span>
            <span className="hx-set__row-text">
              <span className="hx-set__row-label">{t(`next.settings.section.${card.provider}`)}</span>
              <span className="hx-set__row-hint">{agentStatus(card)}</span>
            </span>
            <LimitsLine limits={limits?.[card.provider] ?? null} />
            {offered ? (
              <span className="hx-set__btn" data-tone="ghost">
                {t('next.settings.accounts.connect')}
              </span>
            ) : (
              <Icon name="chevron-right" size="sm" className="hx-acct-row__go" />
            )}
          </button>
        );
      })}
    </Group>
    <Group title={t('next.settings.accounts.speech')}>
      <SpeechKeyCards cards={speech} />
    </Group>
  </>
);

export default AccountsSection;
