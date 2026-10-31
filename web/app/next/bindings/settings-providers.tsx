// The «Модели» page on a machine: what the runtime has of the vendors' keys, the three things a person does with one
// (save, delete, check), and the sign-in of a preset that runs through a program on the machine (ChatGPT). The list
// changes only from the runtime's answers, so a failure leaves it where it was.
import { useCallback, useEffect, useState } from 'react';
import { pageText } from '@/next/bindings/outcome';
import { loginOf, refusalText } from '@/next/bindings/settings-credentials';
import type { LoginState } from '@/next/features/accounts/model';
import {
  cancelProviderLogin,
  deleteProviderKey,
  loadProviderLogin,
  loadProviders,
  logoutProvider,
  putProviderKey,
  startProviderLogin,
  testProvider,
} from '@/next/features/providers/api';
import type { IPresetState, PresetId, TestState } from '@/next/features/providers/model';
import { t } from '@/next/i18n';
import type { LoginStatus, MachineId, ProfileKey } from '@/next/protocol/v2';

const POLL_MS = 1500;
const IDLE: LoginState = { k: 'idle' };
const isRunning = (login: LoginState | undefined): boolean =>
  login?.k === 'starting' || login?.k === 'waiting' || login?.k === 'checking';

type TState = { k: 'loading' } | { k: 'failed'; text: string } | { k: 'ready'; presets: IPresetState[] };

/** An older runtime has no such address: said as that, not as a failure to load. */
const isMissing = (res: Exclude<Awaited<ReturnType<typeof loadProviders>>, { ok: true }>): boolean =>
  res.r === 'refused' ? res.code === 'not-found' : res.code === 'http' && res.status === 404;

/** The sign-ins of the presets' proxies: the login the machine runs, asked again while it waits for the person. */
const useProxyLogins = (machine: MachineId, reload: () => void) => {
  const [proxyLogins, setLogins] = useState<Readonly<Partial<Record<PresetId, LoginState>>>>({});
  const running = Object.entries(proxyLogins)
    .filter(([, login]) => isRunning(login))
    .map(([preset]) => preset);
  const watched = running.join(' ');

  const take = useCallback(
    (preset: PresetId, status: LoginStatus) => {
      setLogins((now) => ({ ...now, [preset]: loginOf(status.flow) }));
      if (status.flow?.state === 'done') reload();
    },
    [reload],
  );

  useEffect(() => {
    if (watched === '') return;
    let live = true;
    const timer = setInterval(() => {
      for (const preset of watched.split(' ')) {
        void loadProviderLogin(machine, preset).then((res) => {
          if (live && 'ok' in res) take(preset, res.value);
        });
      }
    }, POLL_MS);
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, [watched, machine, take]);

  const onProxyLogin = useCallback(
    async (preset: PresetId): Promise<string | null> => {
      setLogins((now) => ({ ...now, [preset]: { k: 'starting' } }));
      const res = await startProviderLogin(machine, preset);
      if (!('ok' in res)) {
        setLogins((now) => ({ ...now, [preset]: IDLE }));
        return refusalText(res, t('next.login.startFailed'));
      }
      take(preset, res.value);
      return null;
    },
    [machine, take],
  );
  const onProxyCancel = useCallback(
    async (preset: PresetId) => {
      await cancelProviderLogin(machine, preset);
      setLogins((now) => ({ ...now, [preset]: IDLE }));
    },
    [machine],
  );
  const onProxyLogout = useCallback(
    async (preset: PresetId): Promise<string | null> => {
      const res = await logoutProvider(machine, preset);
      if (!('ok' in res)) return t('next.login.logoutFailed');
      setLogins((now) => ({ ...now, [preset]: IDLE }));
      reload();
      return null;
    },
    [machine, reload],
  );

  return { proxyLogins, onProxyLogin, onProxyCancel, onProxyLogout };
};

/** The presets of a machine and the actions on them; the first-run step uses the same hook. */
export const useProviders = (machine: MachineId, enabled = true) => {
  const [state, setState] = useState<TState>({ k: 'loading' });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    let live = true;
    void loadProviders(machine).then((res) => {
      if (!live) return;
      if ('ok' in res) setState({ k: 'ready', presets: res.value.profiles });
      else {
        const text = isMissing(res) ? t('next.providers.unsupported') : pageText(res);
        setState((now) => (now.k === 'ready' ? now : { k: 'failed', text }));
      }
    });
    return () => {
      live = false;
    };
  }, [enabled, machine, attempt]);

  const change = useCallback((preset: PresetId, key: ProfileKey) => {
    setState((now) =>
      now.k !== 'ready'
        ? now
        : { k: 'ready', presets: now.presets.map((p) => (p.id === preset ? { ...p, key } : p)) },
    );
  }, []);

  const onSave = useCallback(
    async (preset: PresetId, key: string): Promise<string | null> => {
      const res = await putProviderKey(machine, preset, key);
      if (!('ok' in res)) return t('next.providers.saveFailed');
      change(preset, res.value);
      return null;
    },
    [machine, change],
  );
  const onRemove = useCallback(
    async (preset: PresetId): Promise<string | null> => {
      const res = await deleteProviderKey(machine, preset);
      if (!('ok' in res)) return t('next.providers.removeFailed');
      change(preset, res.value);
      return null;
    },
    [machine, change],
  );
  const onTest = useCallback(
    async (preset: PresetId): Promise<TestState> => {
      const res = await testProvider(machine, preset);
      if (!('ok' in res)) return { k: 'failed', reason: 'page' };
      const { ok, reason } = res.value;
      if (ok) return { k: 'ok' };
      return { k: 'failed', reason: reason ?? 'rejected' };
    },
    [machine],
  );
  const retry = useCallback(() => {
    setState({ k: 'loading' });
    setAttempt((n) => n + 1);
  }, []);
  // A sign-in changed what the machine has: the list is read again, the one on screen staying until it comes.
  const reload = useCallback(() => setAttempt((n) => n + 1), []);
  const logins = useProxyLogins(machine, reload);

  return { state, onSave, onRemove, onTest, retry, ...logins };
};
