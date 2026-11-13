// The extra model vendors on made-up data, for the eye and for screenshots: ?dev=providers. Never a real key: the keys
// typed here go nowhere and only their last digits are kept. ?scene=settings|accounts|picker|chat picks the page, ?state=empty|kimi|
// all the keys on the machine, ?check=ok|auth|network|limit what the check of a connection finds,
// ?theme=light|dark&lang=en|ru the look. Never part of a production path.
import '@/next/styles/tokens.css';
import '@/next/styles/base.css';
import '@/next/styles/shell.css';
import '@/next/styles/ui.css';
import '@/next/styles/icons.css';
import '@/next/styles/chat.css';
import '@/next/styles/settings.css';
import '@/next/styles/ext-providers.css';
import '@/next/dev/providers-gallery.css';

import { useEffect, useRef, useState } from 'react';
import Composer from '@/next/features/composer/composer';
import { pickerAgents, type IModelChoice } from '@/next/features/composer/agents';
import type { IPresetState, PresetId, TestState } from '@/next/features/providers/model';
import {
  limitsOfOwner,
  limitsOwnerOf,
  otherLimitsOf,
  readLimitsOf,
  type LimitsView,
} from '@/next/features/limits/model';
import ContinueSheet from '@/next/features/chat/continue-sheet';
import { EXT_API, type IExtPreset, type IExtProvider } from '@/next/features/settings/ext-providers-api';
import ExtProvidersSection from '@/next/features/settings/ext-providers-section';
import AccountsSection, {
  AgentConnection,
  VendorKeys,
  gptOf,
  type IAgentCredentials,
  type VendorCards,
} from '@/next/features/settings/accounts-section';
import LoginPanel from '@/next/features/accounts/login-panel';
import type { LoginState } from '@/next/features/accounts/model';
import CodexLoginsGroup, { CodexLoginCard } from '@/next/features/settings/codex-logins-group';
import { pinLocale } from '@/next/i18n';
import type { AgentSettings, MachineAgent, Provider } from '@/next/protocol/v2';
import { PortalContext } from '@/next/ui/portal';
import { useRouter } from '@/platform/router';

/** What is left of the made-up subscriptions: Codex on a plan with the week alone, the vendors as they answer. */
const inHours = (h: number): number => Math.round(Date.now() / 1000 + h * 3600);
const LIMITS = readLimitsOf({
  claude: {
    fiveHour: { usedPct: 6, resetsAt: inHours(2.2) },
    sevenDay: { usedPct: 19, resetsAt: inHours(100) },
    plan: 'Claude Max 20x',
    updatedAt: Date.now() / 1000,
  },
  codex: { fiveHour: null, sevenDay: { usedPct: 11, resetsAt: inHours(155) }, plan: 'ChatGPT Prolite' },
  profiles: {
    'kimi-code': {
      profile: 'kimi-code',
      account: '',
      fiveHour: { usedPct: 9.5, resetsAt: inHours(3.4) },
      sevenDay: { usedPct: 18, resetsAt: inHours(70) },
      updatedAt: Date.now() / 1000,
    },
    glm: {
      profile: 'glm',
      account: '',
      fiveHour: { usedPct: 81, resetsAt: inHours(0.7) },
      sevenDay: { usedPct: 41, resetsAt: inHours(120) },
      plan: 'GLM Pro',
    },
    xai: {
      profile: 'xai',
      account: '',
      sevenDay: { usedPct: 90, resetsAt: inHours(30) },
      unofficial: true,
      updatedAt: Date.now() / 1000,
    },
    kimi: { profile: 'kimi', account: '', balance: { amount: 49.58, currency: 'USD' } },
  },
});

const limitsViewOf = (choice: IModelChoice): LimitsView | null => {
  const owner = limitsOwnerOf(choice);
  if (!LIMITS || !owner) return null;
  const limits = limitsOfOwner(LIMITS, owner);
  if (owner.profile !== null && !limits) return null;
  return {
    state: 'ready',
    provider: owner.provider,
    ...(owner.profile === null ? {} : { profile: owner.profile }),
    limits,
    spend: null,
    others: otherLimitsOf(LIMITS, owner),
  };
};

const levels = ['low', 'medium', 'high', 'xhigh', 'max'] as const;

const MACHINE_AGENTS = [
  {
    provider: 'claude',
    version: '2.1.310',
    status: 'ready',
    canSwitchModel: true,
    models: [
      { id: 'opus', label: 'Opus 5.5', hint: 'Самая сильная', efforts: [...levels], defaultEffort: 'high' },
      {
        id: 'sonnet',
        label: 'Sonnet 5.5',
        hint: 'Быстрая и точная',
        efforts: [...levels],
        defaultEffort: 'medium',
      },
    ],
    profiles: [
      {
        id: 'kimi',
        label: 'Kimi',
        vendor: 'kimi',
        configured: true,
        models: [
          { id: 'kimi-k3.1-pro', label: 'K3.1 Pro', hint: '1M контекста', efforts: [], defaultEffort: null },
          {
            id: 'kimi-k2.7-code',
            label: 'K2.7 Code',
            hint: 'Быстрее и дешевле',
            efforts: [],
            defaultEffort: null,
          },
        ],
      },
      {
        // A second account of the vendor: a profile of its own under the name the person gave it.
        id: 'x-kimi-rabochiy',
        label: 'Kimi рабочий',
        vendor: 'kimi',
        configured: true,
        models: [
          { id: 'kimi-k3.1-pro', label: 'K3.1 Pro', hint: '1M контекста', efforts: [], defaultEffort: null },
        ],
      },
      {
        id: 'glm',
        label: 'GLM',
        vendor: 'glm',
        configured: true,
        models: [
          { id: 'glm-5.3', label: 'GLM 5.3', hint: 'Только текст', efforts: [], defaultEffort: null },
          { id: 'glm-5.3-flash', label: 'GLM 5.3 Flash', hint: '', efforts: [], defaultEffort: null },
        ],
      },
    ],
  },
  {
    provider: 'codex',
    version: '0.152.0',
    status: 'ready',
    canSwitchModel: true,
    models: [
      { id: 'gpt-6', label: 'GPT-6', hint: '', efforts: ['low', 'medium', 'high'], defaultEffort: 'medium' },
    ],
    profiles: [
      {
        id: 'xai',
        label: 'Grok',
        vendor: 'grok',
        configured: true,
        models: [
          { id: 'grok-build-0.1', label: 'Grok Build', hint: 'Окно 256k', efforts: [], defaultEffort: null },
          { id: 'grok-4.7', label: 'Grok 4.7', hint: 'Окно 500k', efforts: [], defaultEffort: null },
        ],
      },
    ],
  },
] as unknown as MachineAgent[];

const model = (id: string, label: string) => ({ id, label, hint: '', efforts: [], defaultEffort: null });
const PRESETS: readonly IPresetState[] = [
  {
    id: 'kimi',
    label: 'Kimi · платформа',
    vendor: 'kimi',
    via: 'claude',
    host: 'api.moonshot.ai',
    hint: 'Ключ с platform.kimi.ai, оплата за токены',
    key: { configured: false, tail: null },
    models: [model('kimi-k3.1-pro', 'K3.1 Pro')],
  },
  {
    id: 'kimi-code',
    label: 'Kimi · Kimi Code',
    vendor: 'kimi',
    via: 'claude',
    host: 'api.kimi.com',
    hint: 'Ключ подписки Kimi Code',
    key: { configured: false, tail: null },
    models: [model('kimi-for-coding', 'Kimi for coding')],
  },
  {
    id: 'glm',
    label: 'GLM · Z.ai',
    vendor: 'glm',
    via: 'claude',
    host: 'api.z.ai',
    hint: 'Ключ с z.ai: тарифа Coding Plan или обычный ключ API. Они не взаимозаменяемы',
    key: { configured: false, tail: null },
    models: [model('glm-5.3', 'GLM 5.3')],
  },
  {
    id: 'glm-cn',
    label: 'GLM · bigmodel.cn',
    vendor: 'glm',
    via: 'claude',
    host: 'open.bigmodel.cn',
    hint: 'Ключ с open.bigmodel.cn, для китайского сервиса',
    key: { configured: false, tail: null },
    models: [model('glm-5.3', 'GLM 5.3')],
  },
  {
    id: 'xai',
    label: 'Grok',
    vendor: 'grok',
    via: 'codex',
    host: 'api.x.ai',
    hint: 'Ключ API с console.x.ai',
    key: { configured: false, tail: null },
    models: [model('grok-build-0.1', 'Grok Build')],
  },
  {
    id: 'gpt',
    label: 'GPT · подписка ChatGPT',
    vendor: 'gpt',
    via: 'claude',
    host: 'chatgpt.com',
    hint: '',
    key: { configured: false, tail: null },
    models: [model('gpt-6.1-sol', 'GPT-6.1 Sol'), model('gpt-6-luna', 'GPT-6 Luna')],
    login: { account: 'sergey@example.test' },
    switch: { on: false },
  },
  {
    id: 'auto',
    label: 'Авто · GLM + Kimi',
    vendor: 'auto',
    via: 'claude',
    host: 'api.kimi.com',
    hint: '',
    key: { configured: false, tail: null },
    models: [model('auto', 'Авто'), model('auto-fast', 'Авто · быстро')],
    switch: { on: false },
  },
];

const startStates = (state: string | undefined): IPresetState[] =>
  PRESETS.map((p) => {
    const on = state === 'all' || (state === 'kimi' && p.id === 'kimi');
    return on ? { ...p, key: { configured: true, tail: '…3456' } } : { ...p };
  });

const tailOf = (key: string) => `…${key.slice(-4)}`;

const LOGINS: Record<string, LoginState> = {
  idle: { k: 'idle' },
  starting: { k: 'starting' },
  device: { k: 'waiting', url: 'https://example.test/device', userCode: 'WDJB-MJHT', asksCode: false },
  code: { k: 'waiting', url: 'https://example.test/oauth', userCode: null, asksCode: true },
  failed: { k: 'failed', reason: 'Вход не завершился за пять минут. Попробуйте ещё раз' },
};

const settingsOf = (provider: Provider, signedIn: boolean): AgentSettings => ({
  provider,
  path: provider === 'claude' ? '~/.claude/settings.json' : '~/.codex/config.toml',
  rev: 'r1',
  account: {
    signedIn,
    method: signedIn ? 'subscription' : null,
    plan: signedIn ? 'Max' : null,
    who: signedIn ? 'sergey@example.test' : null,
  },
  options: [],
  rules: null,
  env: [],
  mcp: [],
});

const later = <T,>(value: T, ms = 600) => new Promise<T>((resolve) => setTimeout(() => resolve(value), ms));

const SettingsScene = ({ state, check, login }: { state?: string; check?: string; login?: string }) => {
  const [states, setStates] = useState(() => startStates(state));
  const [loginState, setLoginState] = useState<LoginState>(LOGINS[login ?? 'idle'] ?? LOGINS.idle);
  const [codexKey, setCodexKey] = useState({ configured: login === 'key', tail: null });
  const change = (id: PresetId, patch: Partial<IPresetState>) =>
    setStates((have) => have.map((p) => (p.id === id ? { ...p, ...patch } : p)));
  const result = (): TestState => {
    if (check === 'auth') return { k: 'failed', reason: 'unauthorized' };
    if (check === 'network') return { k: 'failed', reason: 'unreachable' };
    if (check === 'limit') return { k: 'failed', reason: 'rate-limited' };
    return { k: 'ok' };
  };
  const credentials = (key: { configured: boolean; tail: null }): IAgentCredentials => ({
    login: loginState,
    seen: ['person@example.com', 'second@example.com'],
    key,
    onLogin: async () => {
      setLoginState({ k: 'starting' });
      await later(null, 900);
      setLoginState(LOGINS.code);
      return null;
    },
    onSubmitCode: async () => later(null, 700),
    onCancelLogin: async () => setLoginState(LOGINS.idle),
    onLogout: async () => {
      await later(null, 400);
      setCodexKey({ configured: false, tail: null });
      return null;
    },
    onSaveKey: async () => {
      await later(null, 500);
      setCodexKey({ configured: true, tail: null });
      return null;
    },
  });
  const vendors: Extract<VendorCards, { k: 'ready' }> = {
    k: 'ready',
    presets: states,
    onSave: async (id, key) => {
      await later(null, 500);
      change(id, { key: { configured: true, tail: tailOf(key) } });
      return null;
    },
    onRemove: async (id) => {
      await later(null, 400);
      change(id, { key: { configured: false, tail: null } });
      return null;
    },
    onTest: async () => later(result(), 900),
    // ?gpt=fail: the proxy does not start.
    onSwitch: async (id, on) => {
      await later(null, 1400);
      if (on && new URLSearchParams(window.location.search).get('gpt') === 'fail')
        return 'Прокси не запустился';
      change(id, { switch: { on } });
      return null;
    },
  };
  return (
    <div className="hx-root hx-providers-set">
      <div className="hx-set__page-in">
        <AccountsSection
          limits={LIMITS}
          cards={[
            { provider: 'claude', k: 'ready', settings: settingsOf('claude', login === 'connected') },
            { provider: 'codex', k: 'ready', settings: settingsOf('codex', login === 'connected') },
          ]}
          speech={{
            k: 'ready',
            keys: {
              openai: { configured: false, tail: null },
              yandex: { configured: false, tail: null },
              'yandex-folder': { configured: false, tail: null },
            },
            onSave: async () => null,
            onRemove: async () => null,
          }}
          onOpen={() => undefined}
        />
        <AgentConnection
          settings={settingsOf('claude', login === 'connected')}
          credentials={credentials({ configured: false, tail: null })}
          busy={false}
          gpt={null}
          onBaseUrl={async () => true}
        />
        <AgentConnection
          settings={settingsOf('codex', login === 'connected')}
          credentials={credentials(codexKey)}
          busy={false}
          gpt={gptOf(vendors)}
          onBaseUrl={async () => true}
        />
        <CodexLoginsGroup
          logins={[
            { id: 'ab12', name: 'Рабочий', profile: 'cx-ab12', account: 'work@example.com', main: true },
            { id: 'cd34', name: 'Личный', profile: 'cx-cd34', account: '', main: false },
          ]}
          full={false}
          onAdd={async () => null}
          renderLogin={(one) => (
            <CodexLoginCard
              key={one.id}
              login={one}
              onRename={async () => null}
              onRemove={async () => null}
              onMain={async () => null}
            >
              <LoginPanel
                via="subscription"
                account={
                  one.account === ''
                    ? { signedIn: false, method: null, plan: null, who: null }
                    : { signedIn: true, method: 'subscription', plan: null, who: one.account }
                }
                login={{ k: 'idle' }}
                onLogin={async () => null}
                onSubmitCode={async () => null}
                onCancel={async () => undefined}
                onLogout={async () => null}
              />
            </CodexLoginCard>
          )}
        />
        <VendorKeys vendors={vendors} />
      </div>
    </div>
  );
};

const PickerScene = ({ locked = false }: { locked?: boolean }) => {
  const portalRef = useRef<HTMLDivElement>(null);
  const all = pickerAgents(MACHINE_AGENTS);
  // A created chat shows only its own group.
  const agents = locked ? all.filter((a) => a.profile === 'kimi') : all;
  const [choice, setChoice] = useState<IModelChoice>({
    provider: 'claude',
    profile: 'kimi',
    model: 'kimi-k3.1-pro',
    effort: null,
    ultracode: false,
  });
  const [draft, setDraft] = useState('');
  return (
    <PortalContext value={portalRef}>
      <div className="hx-root hx-providers-dock hx-dock">
        <Composer
          value={draft}
          onChange={setDraft}
          onSubmit={() => setDraft('')}
          mode="new"
          onStop={() => Promise.resolve()}
          picker={{ value: choice, agents, locked, onChange: setChoice }}
          attachments={[]}
          onFiles={() => undefined}
          onRemoveAttachment={() => undefined}
          onRetryAttachment={() => undefined}
          recording={null}
          onCancelRecording={() => undefined}
          onFinishRecording={() => undefined}
          maxText={20000}
          limits={limitsViewOf(choice)}
        />
        <div ref={portalRef} className="hx-portal" />
      </div>
    </PortalContext>
  );
};

const ContinueScene = () => {
  const portalRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(true);
  const others = pickerAgents(MACHINE_AGENTS).filter((a) => a.available && a.profile !== 'kimi');
  return (
    <PortalContext value={portalRef}>
      <div className="hx-root hx-providers-dock">
        <ContinueSheet
          open={open}
          onClose={() => setOpen(false)}
          agents={others}
          onPick={() => setOpen(false)}
          onConnect={() => setOpen(false)}
        />
        <div ref={portalRef} className="hx-portal" />
      </div>
    </PortalContext>
  );
};

const EXT_PRESETS: readonly IExtPreset[] = [
  {
    id: 'deepseek',
    name: 'DeepSeek',
    baseUrl: 'https://api.deepseek.com/v1',
    api: EXT_API.openai,
    keyUrl: '',
  },
  {
    id: 'anthropic',
    name: 'Anthropic API',
    baseUrl: 'https://api.anthropic.com',
    api: EXT_API.anthropic,
    keyUrl: 'https://console.anthropic.com/settings/keys',
  },
  {
    id: 'kimi-code',
    name: 'Kimi Code',
    baseUrl: 'https://api.kimi.com/coding',
    api: EXT_API.anthropic,
    keyUrl: 'https://www.kimi.com/code',
  },
  {
    id: 'glm',
    name: 'GLM · Z.ai',
    baseUrl: 'https://api.z.ai/api/anthropic',
    api: EXT_API.anthropic,
    keyUrl: 'https://z.ai/manage-apikey/apikey-list',
  },
  {
    id: 'glm-cn',
    name: 'GLM · bigmodel.cn',
    baseUrl: 'https://open.bigmodel.cn/api/anthropic',
    api: EXT_API.anthropic,
    keyUrl: 'https://open.bigmodel.cn/usercenter/apikeys',
  },
  { id: 'custom', name: 'Свой адрес', baseUrl: '', api: EXT_API.openai, keyUrl: '' },
];

const kimiAccount = (id: string, name: string, isDefault: boolean): IExtProvider => ({
  id,
  name,
  baseUrl: 'https://api.kimi.com/coding',
  host: 'api.kimi.com',
  api: EXT_API.anthropic,
  preset: 'kimi-code',
  hasKey: true,
  models: [
    { id: 'k3.1-pro', label: 'K3.1 Pro' },
    { id: 'k3', label: 'K3' },
  ],
  group: 'kimi-code',
  isDefault,
  managed: false,
  env: [],
});

/** «Провайдеры по API»: two accounts of one vendor, and the form that adds another by a template. */
const AccountsScene = () => {
  const portalRef = useRef<HTMLDivElement>(null);
  const done = () => later<string | null>(null);
  return (
    <PortalContext value={portalRef}>
      <div className="hx-root hx-providers-set">
        <div className="hx-set__page-in">
          <ExtProvidersSection
            state={{
              k: 'ready',
              providers: [
                kimiAccount('kimi-code', 'Kimi Code', true),
                kimiAccount('kimi-rabochiy', 'Kimi рабочий', false),
              ],
              presets: EXT_PRESETS,
              accounts: true,
              launch: true,
            }}
            usage={{ 'kimi-code': [], 'kimi-rabochiy': [] }}
            onAdd={() => later({ id: 'new' })}
            onSave={done}
            onRemove={done}
            onDefault={done}
            onCheck={() => later({ ok: true, text: '' })}
            onRefresh={done}
          />
        </div>
        <div ref={portalRef} className="hx-portal" />
      </div>
    </PortalContext>
  );
};

const ProvidersGallery = () => {
  const { query } = useRouter();
  const theme = query.theme === 'dark' ? 'dark' : 'light';
  pinLocale(query.lang === 'en' ? 'en' : 'ru');
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = theme;
    root.classList.toggle('dark', theme === 'dark');
    root.style.colorScheme = theme;
  }, [theme]);
  if (query.scene === 'accounts') return <AccountsScene />;
  if (query.scene === 'continue') return <ContinueScene />;
  if (query.scene === 'picker') return <PickerScene />;
  if (query.scene === 'chat') return <PickerScene locked />;
  return (
    <SettingsScene
      state={String(query.state ?? '')}
      check={String(query.check ?? '')}
      login={String(query.login ?? '')}
    />
  );
};

export default ProvidersGallery;
