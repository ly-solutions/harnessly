// «Макет» in the chat's tools panel (the mockup «Чат и Sinopia — без вкладок»): Обзор · Настройки ·
// Комментарии · Страницы · Версии of the design open beside the chat. The panel gives the place (layout-tab.ts); the
// design draws this into it, so its parts read the design's own state.
import type { ReactNode } from 'react';
import { t } from '@/next/i18n';
import type { TLayoutPart } from '@/next/features/designly/panel/layout-tab';
import '@/next/styles/design-side.css';

export const LAYOUT_PARTS: readonly TLayoutPart[] = ['info', 'tweaks', 'comments', 'pages', 'versions'];

export interface ILayoutBodyProps {
  part: TLayoutPart;
  onPart: (part: TLayoutPart) => void;
  /** A count beside a part's name: the open threads. */
  badges?: Partial<Record<TLayoutPart, number>>;
  render: (part: TLayoutPart) => ReactNode;
}

const LayoutBody = ({ part, onPart, badges, render }: ILayoutBodyProps) => (
  <div className="hx-ds__body" aria-label={t('next.ds.title')}>
    <div className="hx-ds__sub" role="tablist">
      {LAYOUT_PARTS.map((key) => (
        <button
          key={key}
          type="button"
          role="tab"
          aria-selected={part === key}
          onClick={() => part !== key && onPart(key)}
        >
          {t(`next.ds.tab.${key}`)}
          {(badges?.[key] ?? 0) > 0 && <small>{badges?.[key]}</small>}
        </button>
      ))}
    </div>
    <div key={part}>{render(part)}</div>
  </div>
);

export default LayoutBody;
