// The agent's login ran out in the middle of a chat: the card under the turn signs it in again right there, with the
// same login the Settings page runs (its link, its code), and then sends the person's message once more. Nobody has to
// find out where the sign-in lives or what «/login» means.
import { useState } from 'react';
import { isLoginError } from '@/next/features/chat/error-text';
import LoginPanel from '@/next/features/accounts/login-panel';
import SeenAccounts from '@/next/features/accounts/seen-accounts';
import type { ICredentialActions, LoginState } from '@/next/features/accounts/model';
import { t } from '@/next/i18n';
import type { AgentAccount, Provider } from '@/next/protocol/v2';
import { Button } from '@/next/ui/button';
import Icon from '@/next/ui/icon';
import '@/next/styles/limit.css';
import '@/next/styles/providers.css';

/** What Claude Code and Codex print when their sign-in is gone: expired, revoked or never made. «Please run /login»
 *  on a refusal that is not about the sign-in (a filter's or a network shield's 403) is not one: error-text.ts tells. */
export const isLoginLost = (text: string | null | undefined): boolean => isLoginError(text);

/** The card always offers a fresh login: the machine may still count the expired one as signed in. */
const SIGNED_OUT: AgentAccount = { signedIn: false, method: null, plan: null, who: null };

interface ILoginCardProps {
  provider: Provider;
  login: LoginState;
  actions: Pick<ICredentialActions, 'onLogin' | 'onSubmitCode' | 'onCancelLogin'>;
  /** The login went through on the machine. */
  done: boolean;
  /** The accounts this machine was signed in with, newest first; undefined while not read. */
  seen: readonly string[] | undefined;
  /** Sends the turn's message again; null when the turn had none. */
  onRetry: (() => Promise<void>) | null;
}

const LoginCard = ({ provider, login, actions, done, seen, onRetry }: ILoginCardProps) => {
  const [sending, setSending] = useState(false);
  const agent = t(`next.settings.accounts.name.${provider}`);
  return (
    <div className="hx-terminalfailure" data-tone={done ? 'ok' : undefined} role={done ? 'status' : 'alert'}>
      <Icon name={done ? 'circle-check' : 'log-in'} size="sm" className="hx-terminalfailure__ic" />
      <span className="hx-terminalfailure__body">
        <strong className="hx-terminalfailure__title">
          {done ? t('next.loginCard.doneTitle', { agent }) : t('next.loginCard.title', { agent })}
        </strong>
        <span className="hx-terminalfailure__text">
          {done ? t('next.loginCard.doneLead') : t('next.loginCard.lead')}
        </span>
        <SeenAccounts seen={seen} />
        {!done && (
          <LoginPanel
            via="subscription"
            account={SIGNED_OUT}
            login={login}
            onLogin={actions.onLogin}
            onSubmitCode={actions.onSubmitCode}
            onCancel={actions.onCancelLogin}
            onLogout={() => Promise.resolve(null)}
          />
        )}
      </span>
      {done && onRetry && (
        <Button
          size="sm"
          variant="primary"
          disabled={sending}
          onClick={() => {
            setSending(true);
            void onRetry();
          }}
        >
          {t('next.loginCard.retry')}
        </Button>
      )}
    </div>
  );
};

export default LoginCard;
