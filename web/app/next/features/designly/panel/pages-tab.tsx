// «Макет → Страницы»: every page of the project in a plain list with a search; the one on screen is lit, a
// dot marks a page the agent of this chat changed.
import { useState } from 'react';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

export interface IPanelPage {
  path: string;
  name: string;
}

export interface IPagesTabProps {
  pages: readonly IPanelPage[];
  current: string | null;
  /** The pages the agent of this chat changed. */
  changed: ReadonlySet<string>;
  onPick: (path: string) => void;
}

/** The pages a search finds: every word of it in the name or the path, any case. */
export const findPages = (pages: readonly IPanelPage[], query: string) => {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return pages;
  return pages.filter((p) => {
    const hay = `${p.name} ${p.path}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
};

const PagesTab = ({ pages, current, changed, onPick }: IPagesTabProps) => {
  const [query, setQuery] = useState('');
  const found = findPages(pages, query);
  return (
    <div className="hx-ds__pane">
      <label className="hx-ds-search">
        <Icon name="search" size="sm" />
        <input
          value={query}
          placeholder={t('next.ds.pages.search')}
          aria-label={t('next.ds.pages.search')}
          onChange={(e) => setQuery(e.target.value)}
        />
      </label>
      {found.map((p) => (
        <button
          key={p.path}
          type="button"
          className="hx-ds-page"
          aria-current={p.path === current}
          title={p.path}
          onClick={() => onPick(p.path)}
        >
          <span>{p.name}</span>
          {changed.has(p.path) && <i className="hx-ds-dotmark" aria-label={t('next.ds.pages.changed')} />}
        </button>
      ))}
      {found.length === 0 && <div className="hx-ds__empty">{t('next.ds.pages.none')}</div>}
    </div>
  );
};

export default PagesTab;
