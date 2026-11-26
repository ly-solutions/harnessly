// What a device adds around the page of an open design: the phone's own interface in a case (the status bar, the bars
// of its browser, the home indicator), the drawn parts of our cases, and the phone's code. They are drawings of the
// design's .hx-vp and .hx-phone; the page itself is the frame the site view puts between them. «Все экраны» is
// site-overview.tsx.
import { useState } from 'react';
import Icon from '@/next/ui/icon';
import { locale, t } from '@/next/i18n';
import { PHONE_OS, type IBrowser, type TPhoneOs } from '@/next/features/designly/devices';

interface IBarsProps {
  /** The browser's bars; null on a screen without a system. */
  browser: IBrowser | null;
  os: TPhoneOs;
  /** The status bar on top. */
  status: boolean;
  /** The host the browser's address shows. */
  host: string;
  /** The design's name: a mini app is headed by it. */
  title: string;
  /** A press on a folded bar unfolds the bars. */
  onUnfold: () => void;
}

const StatusBar = ({ os }: { os: TPhoneOs }) => {
  const [today] = useState(() =>
    new Intl.DateTimeFormat(locale(), { weekday: 'short', day: 'numeric', month: 'short' }).format(
      new Date(),
    ),
  );
  return (
    <div className="hx-sb" aria-hidden="true">
      <b>9:41</b>
      {os === PHONE_OS.ipad && <span className="hx-sb__date">{today}</span>}
      <span className="hx-spacer" />
      <span className="hx-sb__r">
        <span className="hx-sb__sig">
          <i />
          <i />
          <i />
          <i />
        </span>
        <Icon name="wifi" size="sm" />
        <span className="hx-sb__bat" />
      </span>
    </div>
  );
};

/** One icon of a browser's bar; `tabs` is the counter of its open tabs. */
const BarIcon = ({ name }: { name: string }) =>
  name === 'tabs' ? <span className="hx-brt__tabs">3</span> : <Icon name={name} size="sm" />;

const Address = ({ browser, host, below = false }: { browser: IBrowser; host: string; below?: boolean }) => (
  <span className="hx-burl" data-centred={below || browser.centred === true}>
    {below || browser.aa ? <small>аА</small> : <Icon name="lock" size="sm" />}
    <b>{host}</b>
    {below && <Icon name="rotate-cw" size="sm" />}
  </span>
);

/** Over the page: the status bar and, for a browser with its address on top or a mini app, that bar. */
export const PhoneTop = ({ browser, os, status, host, title, onUnfold }: IBarsProps) => (
  <>
    {status && <StatusBar os={os} />}
    {browser?.top === 'tg' && (
      <div className="hx-brt" data-t="tg" aria-hidden="true" onClick={onUnfold}>
        <span className="hx-brt__close">{t('next.site.miniClose')}</span>
        <span className="hx-brt__app">
          <b>{title}</b>
          <small>{t('next.site.miniApp')}</small>
        </span>
        <Icon name="ellipsis" size="sm" />
      </div>
    )}
    {browser?.top === 'addr' && (
      <div className="hx-brt" data-t="addr" aria-hidden="true" onClick={onUnfold}>
        {browser.lead?.map((name) => (
          <BarIcon key={name} name={name} />
        ))}
        <Address browser={browser} host={host} />
        {browser.trail?.map((name) => (
          <BarIcon key={name} name={name} />
        ))}
      </div>
    )}
  </>
);

/** Under the page: the browser's bars at the bottom, the glass buttons over the page and the home indicator. */
export const PhoneBottom = ({ browser, host, onUnfold }: Omit<IBarsProps, 'os' | 'status' | 'title'>) => (
  <>
    {browser && (browser.addrBelow || browser.icons) && (
      <div className="hx-brb" data-slim={!browser.addrBelow} aria-hidden="true" onClick={onUnfold}>
        {browser.addrBelow && <Address browser={browser} host={host} below />}
        {browser.icons && (
          <span className="hx-brb__i">
            {browser.icons.map((name) => (
              <BarIcon key={name} name={name} />
            ))}
          </span>
        )}
      </div>
    )}
    {browser?.float && (
      <div className="hx-brf" aria-hidden="true" onClick={onUnfold}>
        <span className="hx-brf__b">
          <Icon name="chevron-left" size="sm" />
        </span>
        <span className="hx-brf__u">
          <span className="hx-brf__fav" />
          <span className="hx-brf__host">{host}</span>
        </span>
        <span className="hx-brf__b">
          <Icon name="ellipsis" size="sm" />
        </span>
      </div>
    )}
    <span className="hx-navb" aria-hidden="true">
      <span />
    </span>
  </>
);

/** The drawn parts of our own cases over the screen: the cut for the camera, a fold's crease, the folding half. */
export const ScreenParts = () => (
  <>
    <span className="hx-fpanel" aria-hidden="true" />
    <span className="hx-cut" aria-hidden="true" />
    <span className="hx-crease" aria-hidden="true" />
  </>
);

/** The parts of our own cases beside the screen: the «Home» button and the camera of the old ones. */
export const CaseParts = () => (
  <>
    <span className="hx-hbtn" aria-hidden="true" />
    <span className="hx-pcam" aria-hidden="true" />
  </>
);

interface IPhoneProps {
  /** The code's modules, rows of dark (true) and light ones. */
  modules: readonly (readonly boolean[])[];
  state: 'open' | 'closing';
  onCopy: () => void;
}

/** «Открыть на телефоне»: the code of the address of this page, to be pointed at with the phone's camera. */
export const PhonePopover = ({ modules, state, onCopy }: IPhoneProps) => (
  <div
    className="hx-site-menu hx-site-menu--end hx-phone"
    data-state={state}
    role="dialog"
    aria-label={t('next.site.phone')}
  >
    <div
      className="hx-qr"
      aria-hidden="true"
      ref={(el) => el?.style.setProperty('--n', String(modules.length))}
    >
      {modules.flatMap((row, y) => row.map((on, x) => <i key={`${y}:${x}`} data-on={on} />))}
    </div>
    <div className="hx-phone__title">{t('next.site.phoneTitle')}</div>
    <div className="hx-phone__sub">{t('next.site.phoneSub')}</div>
    <button type="button" className="hx-phone__send" onClick={onCopy}>
      <Icon name="link" size="sm" />
      {t('next.site.copyLink')}
    </button>
  </div>
);
