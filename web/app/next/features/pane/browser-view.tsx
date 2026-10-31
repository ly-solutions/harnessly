// A browser tab of the pane (.hx-tbrowser): back, forward, reload, the omnibox; with no address yet, the pinned pages and
// the servers that listen on the machine, the chat's own first. The page is the address opened in a preview slot.
import { useMemo } from 'react';
import Icon from '@/next/ui/icon';
import Omnibox from '@/next/features/pane/omnibox';
import { parseAddress, type IAddress } from '@/next/features/pane/address';
import { isPinned, pinLabel, type IPin } from '@/next/features/pane/pins';
import type { IListeningPort } from '@/next/core/net/preview';
import { t } from '@/next/i18n';
import { SIGN_IN, type TSignIn } from '@/next/features/pane/stand-sign-in';
import LoadFailed, { LOAD_FAILED_SIZE } from '@/next/ui/load-failed';

export interface IBrowserViewProps {
  /** What the bar shows; empty for a new tab. */
  address: string;
  /** What the page is called, when it was chosen by name. */
  title: string | null;
  canBack: boolean;
  canForward: boolean;
  /** The slot's entry for the page; null while it is asked for. */
  src: string | null;
  error: string | null;
  /** The machine's servers; null while they load. */
  ports: IListeningPort[] | null;
  portsError: string | null;
  /** Asks for the servers again. */
  onRetryPorts: () => void;
  /** The omnibox was entered: the servers may have changed. */
  onWantPorts: () => void;
  /** The person's pinned pages; null until the person is known. */
  pins: readonly IPin[] | null;
  /** The pages visited lately, the latest first. */
  visits: readonly string[];
  onPin: (pin: IPin) => void;
  onUnpin: (text: string) => void;
  reloadKey: number;
  /** The iframe: the tab talks to the page in it (the stand's wall asks for «Войти»). */
  onFrameLoad: (frame: HTMLIFrameElement) => void;
  /** The stand wants a sign-in (asked), or the person is signing in elsewhere (waiting). */
  signIn: TSignIn;
  onSignIn: () => void;
  onSignInRefresh: () => void;
  /** The app's theme: the page inside is told it (prefers-color-scheme). */
  theme: 'light' | 'dark';
  onGo: (address: IAddress) => void;
  onHistory: (step: -1 | 1) => void;
  onReload: () => void;
  onNewTab: () => void;
}

const translate = (key: string) => t(key);

const BrowserView = (props: IBrowserViewProps) => {
  const { address, canBack, canForward, src, error, ports, portsError, onRetryPorts, reloadKey, pins } =
    props;
  const { visits } = props;
  const sources = useMemo(() => ({ ports, pins, visits, translate }), [ports, pins, visits]);
  const pinnable = pins !== null && address !== '' && !address.startsWith('?');
  const pinned = pins !== null && isPinned(pins, address);

  return (
    <div className="hx-tv" data-tool="browser">
      <div className="hx-tbrowser">
        <div className="hx-tbrowser__bar">
          <button
            type="button"
            className="hx-head-btn"
            data-on={canBack}
            aria-label={t('next.site.back')}
            onClick={() => props.onHistory(-1)}
          >
            <Icon name="arrow-left" size="sm" />
          </button>
          <button
            type="button"
            className="hx-head-btn"
            data-on={canForward}
            aria-label={t('next.site.forward')}
            onClick={() => props.onHistory(1)}
          >
            <Icon name="arrow-right" size="sm" />
          </button>
          <button
            type="button"
            className="hx-head-btn"
            aria-label={t('next.site.reload')}
            onClick={props.onReload}
          >
            <Icon name="rotate-cw" size="sm" />
          </button>
          <Omnibox address={address} sources={sources} onGo={props.onGo} onEnter={props.onWantPorts} />
          {pinnable && (
            <button
              type="button"
              className="hx-head-btn hx-tipped"
              data-pinned={pinned}
              aria-pressed={pinned}
              aria-label={pinned ? t('next.pane.unpin') : t('next.pane.pin')}
              onClick={() =>
                pinned
                  ? props.onUnpin(address)
                  : props.onPin({ text: address, name: props.title ?? undefined })
              }
            >
              <Icon name={pinned ? 'pin-off' : 'pin'} size="sm" />
              <span className="hx-tip" data-pos="below" data-align="end">
                {pinned ? t('next.pane.unpin') : t('next.pane.pin')}
              </span>
            </button>
          )}
          <button
            type="button"
            className="hx-head-btn hx-tipped"
            aria-label={t('next.pane.newBrowserTab')}
            onClick={props.onNewTab}
          >
            <Icon name="plus" size="sm" />
            <span className="hx-tip" data-pos="below" data-align="end">
              {t('next.pane.newBrowserTab')}
            </span>
          </button>
        </div>
        {props.signIn !== SIGN_IN.none && (
          <div className="hx-tbrowser__offer" role="group" aria-label={t('next.pane.signInLabel')}>
            <span className="hx-tbrowser__offer-text">
              {props.signIn === SIGN_IN.asked ? t('next.pane.signInAsked') : t('next.pane.signInWaiting')}
            </span>
            {props.signIn === SIGN_IN.asked ? (
              <button type="button" className="hx-tbrowser__offer-btn" data-primary onClick={props.onSignIn}>
                {t('next.pane.signIn')}
              </button>
            ) : (
              <button type="button" className="hx-tbrowser__offer-btn" onClick={props.onSignInRefresh}>
                {t('next.pane.signInRefresh')}
              </button>
            )}
          </div>
        )}
        {address === '' && (
          <div className="hx-tbrowser__home">
            {pins !== null && pins.length > 0 && (
              <>
                <div className="hx-tv__label">{t('next.pane.pinned')}</div>
                <div className="hx-srv">
                  {pins.map((pin, i) => {
                    const next = parseAddress(pin.text);
                    const label = pinLabel(pin, translate);
                    return (
                      <div key={pin.text} className="hx-srv__row" data-i={Math.min(i, 8)}>
                        <button
                          type="button"
                          className="hx-srv__card"
                          onClick={() => next && props.onGo({ ...next, title: label })}
                        >
                          <span className="hx-srv__ico">
                            <Icon name="pin" size="sm" />
                          </span>
                          <span className="hx-srv__main">
                            <span className="hx-srv__name">{label}</span>
                            {label !== pin.text && <span className="hx-srv__where">{pin.text}</span>}
                          </span>
                        </button>
                        <button
                          type="button"
                          className="hx-srv__off"
                          aria-label={t('next.pane.unpin')}
                          onClick={() => props.onUnpin(pin.text)}
                        >
                          <Icon name="x" size="sm" />
                        </button>
                      </div>
                    );
                  })}
                </div>
              </>
            )}
            <div className="hx-tv__label">{t('next.pane.servers')}</div>
            {ports === null && portsError === null && (
              <div className="hx-tbrowser__note">{t('next.pane.serversLoading')}</div>
            )}
            {portsError !== null && (
              <LoadFailed
                size={LOAD_FAILED_SIZE.inline}
                title={t('next.load.ports')}
                reason={portsError}
                onRetry={onRetryPorts}
              />
            )}
            {ports?.length === 0 && <div className="hx-tbrowser__note">{t('next.pane.noServers')}</div>}
            {ports && ports.length > 0 && (
              <div className="hx-srv">
                {ports.map((p, i) => {
                  const next = parseAddress(String(p.port));
                  const name = p.title ?? p.command ?? `:${p.port}`;
                  const where = [`localhost:${p.port}`, p.title ? p.command : null]
                    .filter(Boolean)
                    .join(' · ');
                  return (
                    <button
                      key={p.port}
                      type="button"
                      className="hx-srv__card"
                      data-i={Math.min(i, 8)}
                      onClick={() => next && props.onGo({ ...next, title: name })}
                    >
                      <span className="hx-srv__ico">
                        {p.icon ? (
                          <img src={p.icon} alt="" width={16} height={16} />
                        ) : (
                          <Icon name="globe" size="sm" />
                        )}
                      </span>
                      <span className="hx-srv__main">
                        <span className="hx-srv__name">{name}</span>
                        {p.description && <span className="hx-srv__about">{p.description}</span>}
                        <span className="hx-srv__where">
                          {where}
                          {p.group === 'chat' && <b>{t('next.pane.thisChat')}</b>}
                        </span>
                      </span>
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        )}
        {address !== '' && error !== null && (
          <LoadFailed title={t('next.load.page')} reason={error} onRetry={props.onReload} />
        )}
        {address !== '' && error === null && src === null && <div className="hx-tbrowser__page" />}
        {address !== '' && src !== null && (
          <iframe
            key={reloadKey}
            onLoad={(event) => props.onFrameLoad(event.currentTarget)}
            className="hx-tbrowser__frame"
            data-theme={props.theme}
            src={src}
            title={address}
            allow="clipboard-write"
          />
        )}
      </div>
    </div>
  );
};

export default BrowserView;
