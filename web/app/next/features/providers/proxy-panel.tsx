// A preset that runs through a program on the machine (ChatGPT through claude-code-proxy): no key, the program's own
// sign-in instead. While the program is not installed the panel says so and gives the line that installs it; once it
// is, the sign-in is the device login of the agents' cards, and a signed-in preset can be checked like a key.
import { useEffect, useRef, useState } from 'react';
import LoginPanel from '@/next/features/accounts/login-panel';
import type { LoginState } from '@/next/features/accounts/model';
import { testLine } from '@/next/features/providers/key-panel';
import { PROXY_INSTALL, type IPresetState, type TestState } from '@/next/features/providers/model';
import { SetButton } from '@/next/features/settings/rows';
import { t } from '@/next/i18n';
import Icon from '@/next/ui/icon';

interface IProxyPanelProps {
  state: IPresetState;
  login: LoginState;
  onLogin: () => Promise<string | null>;
  onCancel: () => Promise<void>;
  onLogout: () => Promise<string | null>;
  onTest: () => Promise<TestState>;
}

const ProxyPanel = ({ state, login, onLogin, onCancel, onLogout, onTest }: IProxyPanelProps) => {
  const [test, setTest] = useState<TestState>({ k: 'idle' });
  const alive = useRef(true);
  const installed = state.proxy?.installed === true;
  const signed = state.login !== undefined;

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  const check = async () => {
    setTest({ k: 'testing' });
    const result = await onTest();
    if (alive.current) setTest(result);
  };
  const line = testLine(test);
  // The line stays on screen while its row closes.
  const [kept, setKept] = useState({ text: '', ok: false });
  const ok = test.k === 'ok';
  if (line !== null && (kept.text !== line || kept.ok !== ok)) setKept({ text: line, ok });

  return (
    <div className="hx-preset">
      {state.hint !== '' && <p className="hx-preset__hint">{state.hint}</p>}
      <div className="hx-reveal" data-open={!installed}>
        <div className="hx-reveal__in" inert={installed || undefined}>
          <p className="hx-preset__hint">{t('next.providers.proxy.missing')}</p>
          <code className="hx-preset__cmd">{PROXY_INSTALL}</code>
          <p className="hx-preset__where">{t('next.providers.proxy.missingNote')}</p>
        </div>
      </div>
      <div className="hx-reveal" data-open={installed}>
        <div className="hx-reveal__in" inert={!installed || undefined}>
          <LoginPanel
            via="subscription"
            account={{
              signedIn: signed,
              method: 'subscription',
              plan: null,
              who: state.login?.account || null,
            }}
            login={login}
            onLogin={onLogin}
            // A device login asks for no code back.
            onSubmitCode={async () => t('next.login.codeFailed')}
            onCancel={onCancel}
            onLogout={onLogout}
            startLabel={t('next.providers.proxy.start')}
          />
          <div className="hx-reveal" data-open={signed}>
            <div className="hx-reveal__in" inert={!signed || undefined}>
              <div className="hx-preset__row">
                <SetButton tone="primary" busy={test.k === 'testing'} onClick={() => void check()}>
                  {test.k === 'testing' ? t('next.providers.testing') : t('next.providers.test')}
                </SetButton>
              </div>
            </div>
          </div>
          <div className="hx-reveal" data-open={line !== null && signed}>
            <div className="hx-reveal__in">
              <div className="hx-preset__test" data-ok={kept.ok || undefined} role="status">
                <Icon name={kept.ok ? 'circle-check' : 'triangle-alert'} size="sm" />
                <span>{kept.text}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ProxyPanel;
