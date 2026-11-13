// «Ещё входы» on the Codex page: one more ChatGPT plan is one more sign-in of the CLI, with a name the person gives it.
// A chat starts on it from the model list of a new chat. Each sign-in signs in and out here like the main one does;
// the list says whose plan a sign-in holds and never a token.
import { useEffect, useState, type ReactNode } from 'react';
import type { ICodexLogin } from '@/next/features/settings/codex-logins-api';
import { FormError, Group, SetButton } from '@/next/features/settings/rows';
import { t } from '@/next/i18n';
import Icon from '@/next/ui/icon';

const SURE_MS = 4000;

interface ICodexLoginCardProps {
  login: ICodexLogin;
  /** Resolves the words of a refusal, or null when the name was taken. */
  onRename: (name: string) => Promise<string | null>;
  onRemove: () => Promise<string | null>;
  /** Makes it the sign-in a new chat starts on, or gives that back. */
  onMain: (main: boolean) => Promise<string | null>;
  /** What is left of its plan, drawn beside whose plan it is. */
  left?: ReactNode;
  /** The sign-in panel of this login. */
  children: ReactNode;
}

export const CodexLoginCard = ({
  login,
  onRename,
  onRemove,
  onMain,
  left,
  children,
}: ICodexLoginCardProps) => {
  const [name, setName] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [sure, setSure] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (!sure) return;
    const timer = window.setTimeout(() => setSure(false), SURE_MS);
    return () => window.clearTimeout(timer);
  }, [sure]);

  const run = async (act: () => Promise<string | null>, done: () => void) => {
    setBusy(true);
    setError(null);
    const refused = await act();
    setBusy(false);
    if (refused === null) done();
    else setError(refused);
  };
  const editing = name !== null;
  const typed = (name ?? '').trim();
  const save = () => {
    if (typed === '' || typed === login.name) return setName(null);
    void run(
      () => onRename(typed),
      () => setName(null),
    );
  };

  return (
    <div className="hx-acct hx-acct--page hx-cxl" data-open data-on={login.account !== ''}>
      <div className="hx-cxl__head">
        {editing ? (
          <form
            className="hx-cxl__rename"
            onSubmit={(e) => {
              e.preventDefault();
              save();
            }}
          >
            <input
              className="hx-set__input"
              value={name}
              maxLength={40}
              autoFocus
              aria-label={t('next.codexLogins.name')}
              onChange={(e) => setName(e.target.value)}
            />
            <SetButton tone="primary" disabled={busy} onClick={save}>
              {t('next.codexLogins.save')}
            </SetButton>
            <SetButton tone="ghost" disabled={busy} onClick={() => setName(null)}>
              {t('next.providers.cancel')}
            </SetButton>
          </form>
        ) : (
          <>
            <div className="hx-cxl__who">
              <b>{login.name}</b>
              <small>
                {login.main && <span className="hx-cxl__main">{t('next.codexLogins.main')}</span>}
                {login.account === '' ? t('next.codexLogins.off') : login.account}
              </small>
              {left}
            </div>
            {login.account !== '' && (
              <SetButton
                tone="ghost"
                disabled={busy}
                onClick={() =>
                  void run(
                    () => onMain(!login.main),
                    () => undefined,
                  )
                }
              >
                {login.main ? t('next.codexLogins.unmain') : t('next.codexLogins.makeMain')}
              </SetButton>
            )}
            <button
              type="button"
              className="hx-head-btn"
              aria-label={t('next.codexLogins.rename')}
              title={t('next.codexLogins.rename')}
              disabled={busy}
              onClick={() => setName(login.name)}
            >
              <Icon name="pencil" size="sm" />
            </button>
            <SetButton
              tone={sure ? 'danger' : 'ghost'}
              disabled={busy}
              onClick={() => {
                if (!sure) return setSure(true);
                setSure(false);
                void run(onRemove, () => undefined);
              }}
            >
              {sure ? t('next.codexLogins.removeSure') : t('next.codexLogins.remove')}
            </SetButton>
          </>
        )}
      </div>
      {error !== null && <FormError>{error}</FormError>}
      <div className="hx-acct__body">
        <div className="hx-acct__inner">{children}</div>
      </div>
    </div>
  );
};

interface ICodexLoginsGroupProps {
  /** null while the list is read. */
  logins: readonly ICodexLogin[] | null;
  /** No more sign-ins fit. */
  full: boolean;
  onAdd: (name: string) => Promise<string | null>;
  renderLogin: (login: ICodexLogin) => ReactNode;
}

const CodexLoginsGroup = ({ logins, full, onAdd, renderLogin }: ICodexLoginsGroupProps) => {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (logins === null) return null;
  const typed = name.trim();
  const add = () => {
    if (typed === '' || busy) return;
    setBusy(true);
    setError(null);
    void onAdd(typed).then((refused) => {
      setBusy(false);
      if (refused === null) setName('');
      else setError(refused);
    });
  };
  return (
    <Group title={t('next.codexLogins.title')} foot={t('next.codexLogins.foot')}>
      {logins.map(renderLogin)}
      {!full && (
        <form
          className="hx-cxl__add"
          onSubmit={(e) => {
            e.preventDefault();
            add();
          }}
        >
          <input
            className="hx-set__input"
            value={name}
            maxLength={40}
            placeholder={t('next.codexLogins.namePlaceholder')}
            aria-label={t('next.codexLogins.name')}
            onChange={(e) => setName(e.target.value)}
          />
          <SetButton tone="primary" disabled={busy || typed === ''} onClick={add}>
            {t('next.codexLogins.add')}
          </SetButton>
        </form>
      )}
      {error !== null && <FormError>{error}</FormError>}
    </Group>
  );
};

export default CodexLoginsGroup;
