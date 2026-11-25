// The two things every screen needs first: the hub's configuration and who is signed in (GET /api/config, GET
// /api/me; a 401 on the second is just "signed out").
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import { type ApiError, isApiError } from './api';
import { hubCache } from './cache';
import { useApi } from './env';
import type { Config, Me } from './types';

type Phase =
  { kind: 'loading' } | { kind: 'error'; error: ApiError } | { kind: 'ready'; config: Config; me: Me | null };

export interface Session {
  config: Config;
  me: Me | null;
  /** Reads /api/me again (after a sign-in, a new space, an accepted invite). */
  reload: () => Promise<Me | null>;
  /** Forgets the person (after a sign-out). */
  clear: () => void;
}

const SESSION_KEY = 'session';

/** What was last read of the hub's configuration and of the person; a copy that is not of this shape is no copy. */
export const sessionCopy = (raw: unknown): { config: Config; me: Me | null } | null => {
  if (!raw || typeof raw !== 'object') return null;
  const { config, me } = raw as { config?: unknown; me?: unknown };
  if (!config || typeof config !== 'object' || typeof (config as Config).version !== 'string') return null;
  if (me === null) return { config: config as Config, me: null };
  const user = (me as Me | undefined)?.user;
  if (!user || typeof user.id !== 'string' || !Array.isArray((me as Me).spaces)) return null;
  return { config: config as Config, me: me as Me };
};

/** Remembers the answers for the next load; a person who is not signed in leaves no copy of anyone's data. */
const keepSession = (config: Config, me: Me | null) => {
  if (me) {
    hubCache.setPerson(me.user.id);
  } else {
    hubCache.clear();
  }
  hubCache.setShared(SESSION_KEY, { config, me });
};

const SessionContext = createContext<Session | null>(null);

export const useSession = (): Session => {
  const s = useContext(SessionContext);
  if (!s) throw new Error('SessionProvider is missing');
  return s;
};

/** The signed-in person; for screens that are only reached signed in. */
export const useMe = (): Me => {
  const { me } = useSession();
  if (!me) throw new Error('not signed in');
  return me;
};

export const SessionProvider = ({
  children,
  failed,
  loading,
}: {
  children: ReactNode;
  failed: (error: ApiError, retry: () => void) => ReactNode;
  loading: ReactNode;
}) => {
  const api = useApi();
  // Drawn at once from the last answers while the live ones load. The person in it is provisional: the live
  // /api/me still says who is signed in, and a signed-out answer drops the copy and goes to sign-in as ever.
  const [seed] = useState(() => sessionCopy(hubCache.getShared(SESSION_KEY)));
  const [phase, setPhase] = useState<Phase>(() => {
    if (!seed) return { kind: 'loading' };
    hubCache.setPerson(seed.me?.user.id ?? null);
    return { kind: 'ready', config: seed.config, me: seed.me };
  });
  const [attempt, setAttempt] = useState(0);
  // True while what is drawn is the copy and not an answer.
  const provisional = useRef(seed !== null);

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        // Both at once; the error of the configuration, when there is one, is the one that is shown.
        const meP = api.me.get().then(
          (m): Me | null => m,
          (e: unknown) => {
            if (isApiError(e, 'unauthenticated')) return null;
            throw e;
          },
        );
        meP.catch(() => undefined);
        const config = await api.config();
        const me = await meP;
        if (!alive) return;
        provisional.current = false;
        keepSession(config, me);
        setPhase({ kind: 'ready', config, me });
      } catch (e) {
        if (!alive) return;
        // A copy is on the screen: a refresh that failed (offline, the hub restarting) says nothing, but a refusal
        // takes the copy away.
        if (provisional.current && !isApiError(e, 'forbidden')) return;
        provisional.current = false;
        hubCache.clear();
        setPhase({ kind: 'error', error: isApiError(e) ? e : (e as ApiError) });
      }
    })();
    return () => {
      alive = false;
    };
  }, [api, attempt]);

  const reload = useCallback(async () => {
    const me = await api.me.get().catch((e: unknown) => {
      if (isApiError(e, 'unauthenticated')) return null;
      throw e;
    });
    setPhase((p) => {
      if (p.kind !== 'ready') return p;
      keepSession(p.config, me);
      return { ...p, me };
    });
    return me;
  }, [api]);

  const clear = useCallback(() => {
    hubCache.clear();
    setPhase((p) => (p.kind === 'ready' ? { ...p, me: null } : p));
  }, []);

  const value = useMemo<Session | null>(
    () => (phase.kind === 'ready' ? { config: phase.config, me: phase.me, reload, clear } : null),
    [phase, reload, clear],
  );

  if (phase.kind === 'loading') return <>{loading}</>;
  if (phase.kind === 'error')
    return (
      <>
        {failed(phase.error, () => {
          setPhase({ kind: 'loading' });
          setAttempt((n) => n + 1);
        })}
      </>
    );
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
};
