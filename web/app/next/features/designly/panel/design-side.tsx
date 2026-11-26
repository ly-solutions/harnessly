// The tools' panel beside an open design (the mockup «Чат и Sinopia — без вкладок»): «Инструменты» — the
// chat's launcher, as the chat's own right pane has it, without «во весь экран» — and «Макет», only while a design is
// open: Обзор · Настройки · Комментарии · Страницы · Версии. The toolbar's buttons open it on a tab
// (design-panel.ts: open(tab) again closes it).
import { createContext, use, type ReactNode } from 'react';
import Icon from '@/next/ui/icon';
import { usePresence } from '@/next/ui/presence';
import { t } from '@/next/i18n';
import type { TDesignTab } from '@/next/features/designly/design-panel';
import '@/next/styles/design-side.css';

/** The launcher of the chat the design is open beside (right-pane.tsx gives it); none on a design's own page. */
export const DesignToolsContext = createContext<ReactNode>(null);
export const useDesignTools = () => use(DesignToolsContext);

const EXIT_MS = 220;

export type TLayoutTab = Exclude<TDesignTab, 'tools'>;
export const LAYOUT_TABS: readonly TLayoutTab[] = ['info', 'tweaks', 'comments', 'pages', 'versions'];

export interface IDesignSideProps {
  /** The tab shown; null: the panel is shut. */
  tab: TDesignTab | null;
  onTab: (tab: TDesignTab) => void;
  onClose: () => void;
  /** The launcher; without it «Инструменты» is not offered. */
  tools: ReactNode;
  /** A count beside a tab's name: the open threads. */
  badges?: Partial<Record<TLayoutTab, number>>;
  /** What each tab of «Макет» shows. */
  render: (tab: TLayoutTab) => ReactNode;
}

const DesignSide = ({ tab, onTab, onClose, tools, badges, render }: IDesignSideProps) => {
  const shown: TDesignTab | null = tab === 'tools' && tools === null ? 'info' : tab;
  const presence = usePresence(shown, EXIT_MS);
  const at = presence.value;
  if (at === null) return null;
  const layout = at !== 'tools';
  return (
    <aside className="hx-sinopia__panel hx-ds" data-state={presence.state} aria-label={t('next.ds.title')}>
      <div className="hx-ds__head">
        <div className="hx-ds__tabs" role="tablist">
          {tools !== null && (
            <button
              type="button"
              role="tab"
              className="hx-ds__tab hx-ds__tab--icon"
              aria-selected={!layout}
              aria-label={t('next.ds.tools')}
              title={t('next.ds.tools')}
              onClick={() => onTab('tools')}
            >
              <Icon name="layout-grid" size="sm" />
            </button>
          )}
          <button
            type="button"
            role="tab"
            className="hx-ds__tab"
            aria-selected={layout}
            onClick={() => onTab(layout ? at : 'info')}
          >
            <Icon name="presentation" size="sm" />
            {t('next.ds.layout')}
          </button>
        </div>
        <span className="hx-spacer" />
        <button type="button" className="hx-head-btn" aria-label={t('next.ds.close')} onClick={onClose}>
          <Icon name="x" />
        </button>
      </div>
      {!layout ? (
        <div className="hx-ds__body hx-ds__body--tools">{tools}</div>
      ) : (
        <div className="hx-ds__body">
          <div className="hx-ds__sub" role="tablist">
            {LAYOUT_TABS.map((key) => (
              <button
                key={key}
                type="button"
                role="tab"
                aria-selected={at === key}
                onClick={() => at !== key && onTab(key)}
              >
                {t(`next.ds.tab.${key}`)}
                {(badges?.[key] ?? 0) > 0 && <small>{badges?.[key]}</small>}
              </button>
            ))}
          </div>
          <div key={at}>{render(at)}</div>
        </div>
      )}
    </aside>
  );
};

export default DesignSide;
