// Settings → «Провайдеры» → «По API» on the machine the person has open: the registry is read when the page
// opens and changes only from the machine's answers. Adding a provider first checks its address and key, and the
// models the check found are kept with it.
import { useMachineLimits } from '@/next/bindings/use-limits';
import { useAccountSpend } from '@/next/bindings/use-account-spend';
import { useCallback, useEffect, useState } from 'react';
import { refusalOf } from '@/next/bindings/settings-changes';
import {
  checkExtDraft,
  checkExtProvider,
  createExtProvider,
  discoverLocal,
  LOCAL_PRESET,
  loadExtProviders,
  patchExtProvider,
  refreshExtModels,
  removeExtProvider,
  type IExtModel,
  type IExtPreset,
  type IExtProvider,
  type ILocalServer,
} from '@/next/features/settings/ext-providers-api';
import ExtProvidersSection, {
  type IExtAdd,
  type IExtEdit,
  type TExtProvidersState,
} from '@/next/features/settings/ext-providers-section';
import LocalModelCard, { connectedTo, type TLocalState } from '@/next/features/settings/local-model-card';
import { isRefused } from '@/next/features/settings/platform-answer';
import { loadSmart } from '@/next/features/settings/smart-api';
import { t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';

type TLoad =
  | { k: 'loading' }
  | { k: 'failed'; text: string }
  | {
      k: 'ready';
      providers: readonly IExtProvider[];
      presets: readonly IExtPreset[];
      accounts: boolean;
      launch: boolean;
    };

/** What each provider does: the titles of the features given to it. null: the machine did not say. */
const useUsage = (machine: MachineId, stamp: number): Readonly<Record<string, readonly string[]>> | null => {
  const [read, setRead] = useState<{ machine: MachineId; usage: Record<string, string[]> } | null>(null);
  useEffect(() => {
    let live = true;
    void loadSmart(machine).then((res) => {
      if (!live || !('ok' in res) || isRefused(res.value)) return;
      const usage: Record<string, string[]> = {};
      for (const feature of res.value.features)
        if (feature.choice) (usage[feature.choice.provider] ??= []).push(feature.title);
      setRead({ machine, usage });
    });
    return () => {
      live = false;
    };
  }, [machine, stamp]);
  return read?.machine === machine ? read.usage : null;
};

/** The card of a local model: the servers of the machine, looked for when the page opens and again on «Обновить». */
const LocalModel = ({
  machine,
  providers,
  onMade,
  onCheck,
}: {
  machine: MachineId;
  providers: readonly IExtProvider[];
  onMade: (provider: IExtProvider) => void;
  onCheck: (id: string) => Promise<unknown>;
}) => {
  const [box, setBox] = useState<{ machine: MachineId; state: TLocalState }>({
    machine,
    state: { k: 'busy' },
  });
  const [attempt, setAttempt] = useState(0);
  const [address, setAddress] = useState('');
  // The servers are looked for again for each (machine, address, press of «Обновить»); an answer of an older one is dropped.
  useEffect(() => {
    let live = true;
    void discoverLocal(machine, address).then((res) => {
      if (!live) return;
      const refused = refusalOf(res);
      if (refused === null && 'ok' in res && !isRefused(res.value))
        setBox({ machine, state: { k: 'ready', found: res.value } });
      else setBox({ machine, state: { k: 'failed', text: refused ?? t('next.settings.failed') } });
    });
    return () => {
      live = false;
    };
  }, [machine, address, attempt]);
  const state: TLocalState = box.machine === machine ? box.state : { k: 'busy' };

  const connected = providers.filter((p) => p.preset === LOCAL_PRESET);
  const onScan = (typed: string) => {
    setBox({ machine, state: { k: 'busy' } });
    setAddress(typed);
    setAttempt((n) => n + 1);
    // What the person connected earlier takes the models the server has now.
    for (const p of connected) void onCheck(p.id);
  };
  const onConnect = async (server: ILocalServer): Promise<string | null> => {
    if (connectedTo(connected, server)) return null;
    const res = await createExtProvider(machine, {
      name: t('next.localModel.accountName', { name: server.name }),
      baseUrl: server.baseUrl,
      api: server.api,
      preset: LOCAL_PRESET,
      key: '',
      models: server.models,
    });
    const refused = refusalOf(res);
    if (refused !== null || !('ok' in res) || isRefused(res.value))
      return refused ?? t('next.settings.failed');
    onMade(res.value);
    return null;
  };
  return <LocalModelCard state={state} connected={connected} onScan={onScan} onConnect={onConnect} />;
};

export const ExtProvidersPage = ({ machine }: { machine: MachineId }) => {
  const [box, setBox] = useState<{ machine: MachineId; load: TLoad }>({ machine, load: { k: 'loading' } });
  const [attempt, setAttempt] = useState(0);
  // Moves with every change of the list, so what the providers do is read again (a removed one does nothing).
  const [edits, setEdits] = useState(0);
  const usage = useUsage(machine, edits);
  const limits = useMachineLimits(machine);
  const spend = useAccountSpend(machine);

  useEffect(() => {
    let live = true;
    void loadExtProviders(machine).then((res) => {
      if (!live) return;
      const refused = refusalOf(res);
      if (refused === null && 'ok' in res && !isRefused(res.value))
        setBox({ machine, load: { k: 'ready', ...res.value } });
      else
        setBox((now) =>
          now.machine === machine && now.load.k === 'ready'
            ? now
            : { machine, load: { k: 'failed', text: refused ?? '' } },
        );
    });
    return () => {
      live = false;
    };
  }, [machine, attempt]);

  const load: TLoad = box.machine === machine ? box.load : { k: 'loading' };

  /** Changes the list as the machine answered. */
  const change = useCallback(
    (next: (providers: readonly IExtProvider[]) => readonly IExtProvider[]) => {
      setBox((now) =>
        now.machine !== machine || now.load.k !== 'ready'
          ? now
          : { machine, load: { ...now.load, providers: next(now.load.providers) } },
      );
      setEdits((n) => n + 1);
    },
    [machine],
  );
  const put = (provider: IExtProvider) =>
    change((list) =>
      list.some((p) => p.id === provider.id)
        ? list.map((p) => (p.id === provider.id ? provider : p))
        : [...list, provider],
    );
  const putModels = (id: string, models: readonly IExtModel[]) =>
    change((list) => list.map((p) => (p.id === id ? { ...p, models } : p)));

  const onAdd = async (draft: IExtAdd): Promise<{ id: string } | { error: string }> => {
    const probe = await checkExtDraft(machine, { baseUrl: draft.baseUrl, api: draft.api, key: draft.key });
    const unchecked = refusalOf(probe);
    if (unchecked !== null || !('ok' in probe) || isRefused(probe.value))
      return { error: unchecked ?? t('next.settings.failed') };
    if (!probe.value.ok) return { error: probe.value.message || t('next.extProviders.checkFailed') };
    const res = await createExtProvider(machine, {
      name: draft.name,
      baseUrl: draft.baseUrl,
      api: draft.api,
      preset: draft.preset.id,
      key: draft.key,
      models: probe.value.models,
    });
    const refused = refusalOf(res);
    if (refused !== null || !('ok' in res) || isRefused(res.value))
      return { error: refused ?? t('next.settings.failed') };
    put(res.value);
    return { id: res.value.id };
  };

  const onSave = async (id: string, edit: IExtEdit): Promise<string | null> => {
    const { key, ...rest } = edit;
    const res = await patchExtProvider(machine, id, key === '' ? rest : edit);
    const refused = refusalOf(res);
    if (refused !== null || !('ok' in res) || isRefused(res.value))
      return refused ?? t('next.settings.failed');
    put(res.value);
    return null;
  };

  /** The list as the machine has it now: a default that moved changes more accounts than the one that was touched. */
  const reread = () => setAttempt((n) => n + 1);
  const onRemove = async (id: string): Promise<string | null> => {
    const refused = refusalOf(await removeExtProvider(machine, id));
    if (refused !== null) return refused;
    change((list) => list.filter((p) => p.id !== id));
    reread();
    return null;
  };
  const onDefault = async (id: string): Promise<string | null> => {
    const res = await patchExtProvider(machine, id, { default: true });
    const refused = refusalOf(res);
    if (refused !== null || !('ok' in res) || isRefused(res.value))
      return refused ?? t('next.settings.failed');
    const made = res.value;
    change((list) =>
      list.map((p) => {
        if (p.id === made.id) return made;
        return p.group === made.group ? { ...p, isDefault: false } : p;
      }),
    );
    return null;
  };

  const onCheck = async (id: string): Promise<{ ok: boolean; text: string }> => {
    const res = await checkExtProvider(machine, id);
    const refused = refusalOf(res);
    if (refused !== null || !('ok' in res) || isRefused(res.value))
      return { ok: false, text: refused ?? t('next.settings.failed') };
    const check = res.value;
    if (!check.ok) return { ok: false, text: check.message || t('next.extProviders.checkFailed') };
    if (check.models.length > 0) putModels(id, check.models);
    return { ok: true, text: check.message || t('next.extProviders.checkOk') };
  };

  const onRefresh = async (id: string): Promise<string | null> => {
    const res = await refreshExtModels(machine, id);
    const refused = refusalOf(res);
    if (refused !== null || !('ok' in res) || isRefused(res.value))
      return refused ?? t('next.settings.failed');
    const { check, models } = res.value;
    if (check && !check.ok) return check.message || t('next.extProviders.checkFailed');
    putModels(id, models);
    return null;
  };

  let state: TExtProvidersState = { k: 'loading' };
  // The local model has a card of its own below: it is not offered as a provider to connect with a key.
  if (load.k === 'ready') state = { ...load, presets: load.presets.filter((p) => p.id !== LOCAL_PRESET) };
  else if (load.k === 'failed')
    state = {
      k: 'failed',
      text: load.text,
      onRetry: () => {
        setBox({ machine, load: { k: 'loading' } });
        setAttempt((n) => n + 1);
      },
    };

  return (
    <>
      <ExtProvidersSection
        state={state}
        limits={limits}
        spend={spend}
        usage={usage}
        onAdd={onAdd}
        onSave={onSave}
        onRemove={onRemove}
        onDefault={onDefault}
        onCheck={onCheck}
        onRefresh={onRefresh}
      />
      {load.k === 'ready' && (
        <LocalModel machine={machine} providers={load.providers} onMade={put} onCheck={onCheck} />
      )}
    </>
  );
};
