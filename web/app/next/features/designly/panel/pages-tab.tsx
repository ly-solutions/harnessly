// «Макет → Страницы»: every page of the project, numbered, in a dense list with a search — how many there are
// (or how many the search found), the words found lit, Enter opens the first one. The page on screen is lit and
// scrolled to; a dot marks a page the agent of this chat changed. Whoever may edit gets «+ Новая страница» and each
// row's «⋯»: open, open in a new tab, rename in place, duplicate as a variant, delete (asked once more).
import { Fragment, useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

export interface IPanelPage {
  path: string;
  name: string;
}

/** What may be done with the pages; each resolves when done and throws a refusal. */
export interface IPageActions {
  create: () => Promise<void>;
  duplicate: (path: string) => Promise<void>;
  rename: (path: string, title: string) => Promise<void>;
  remove: (path: string) => Promise<void>;
  /** Says what went wrong. */
  failed: (error: unknown) => void;
}

export interface IPagesTabProps {
  pages: readonly IPanelPage[];
  current: string | null;
  /** The pages the agent of this chat changed. */
  changed: ReadonlySet<string>;
  onPick: (path: string) => void;
  /** The page's own address, for «Открыть в новой вкладке». */
  hrefOf?: (path: string) => string;
  /** Absent for whoever may only look. */
  actions?: IPageActions;
}

const wordsOf = (query: string) => query.toLowerCase().split(/\s+/).filter(Boolean);

/** The pages a search finds: every word of it in the name or the path, any case. */
export const findPages = (pages: readonly IPanelPage[], query: string) => {
  const words = wordsOf(query);
  if (words.length === 0) return pages;
  return pages.filter((p) => {
    const hay = `${p.name} ${p.path}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
};

/** The name with the search's words lit. */
export const litName = (name: string, query: string): ReactNode[] => {
  const words = wordsOf(query);
  if (words.length === 0) return [name];
  const lower = name.toLowerCase();
  const on = new Array<boolean>(name.length).fill(false);
  for (const w of words) {
    for (let at = lower.indexOf(w); at >= 0; at = lower.indexOf(w, at + w.length))
      on.fill(true, at, at + w.length);
  }
  const out: ReactNode[] = [];
  let from = 0;
  for (let i = 1; i <= name.length; i++) {
    if (i < name.length && on[i] === on[from]) continue;
    const part = name.slice(from, i);
    out.push(on[from] ? <mark key={from}>{part}</mark> : <Fragment key={from}>{part}</Fragment>);
    from = i;
  }
  return out;
};

const PagesTab = ({ pages, current, changed, onPick, hrefOf, actions }: IPagesTabProps) => {
  const [query, setQuery] = useState('');
  const [menu, setMenu] = useState<string | null>(null);
  const [asking, setAsking] = useState<string | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const list = useRef<HTMLDivElement>(null);
  const found = findPages(pages, query);
  const number = new Map(pages.map((p, i) => [p.path, i + 1]));

  // The page on screen is in view when the tab opens.
  useEffect(() => {
    list.current?.querySelector('[aria-current="true"]')?.scrollIntoView({ block: 'center' });
  }, []);

  // A click elsewhere or Escape closes the row's menu.
  useEffect(() => {
    if (menu === null) return;
    const away = (e: Event) => {
      if (!(e.target instanceof Element) || !e.target.closest('.hx-ds-pmenu, .hx-ds-pli__more')) {
        setMenu(null);
        setAsking(null);
      }
    };
    const key = (e: globalThis.KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      setMenu(null);
      setAsking(null);
    };
    document.addEventListener('pointerdown', away);
    document.addEventListener('keydown', key);
    return () => {
      document.removeEventListener('pointerdown', away);
      document.removeEventListener('keydown', key);
    };
  }, [menu]);

  const run = (job: () => Promise<void>) => {
    if (!actions || busy) return;
    setBusy(true);
    setMenu(null);
    setAsking(null);
    job()
      .catch(actions.failed)
      .finally(() => setBusy(false));
  };

  const onSearchKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' && found.length > 0) {
      e.preventDefault();
      onPick(found[0].path);
    }
    if (e.key === 'Escape' && query !== '') {
      e.preventDefault();
      setQuery('');
    }
  };

  const onRenameKey = (e: KeyboardEvent<HTMLInputElement>, p: IPanelPage) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      setRenaming(null);
      return;
    }
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const title = e.currentTarget.value.trim();
    setRenaming(null);
    if (title !== '' && title !== p.name && actions) run(() => actions.rename(p.path, title));
  };

  return (
    <div className="hx-ds__pane hx-ds-pages" data-busy={busy || undefined}>
      <div className="hx-ds-pages__head">
        <span className="hx-ds-pages__count">
          {query.trim() === ''
            ? t('next.ds.pages.count', { n: pages.length })
            : t('next.ds.pages.found', { n: found.length, all: pages.length })}
        </span>
        {actions && (
          <button
            type="button"
            className="hx-ds-pages__new"
            disabled={busy}
            onClick={() => run(actions.create)}
          >
            <Icon name="plus" size="sm" />
            <span>{t('next.ds.pages.new')}</span>
          </button>
        )}
      </div>
      <label className="hx-ds-search">
        <Icon name="search" size="sm" />
        <input
          value={query}
          placeholder={t('next.ds.pages.search')}
          aria-label={t('next.ds.pages.search')}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onSearchKey}
        />
      </label>
      <div ref={list} className="hx-ds-plist" role="list">
        {found.map((p) => {
          const on = p.path === current;
          const open = menu === p.path;
          return (
            <div key={p.path} className="hx-ds-pli" role="listitem" data-menu={open || undefined}>
              {renaming === p.path ? (
                <input
                  className="hx-ds-pli__rename"
                  defaultValue={p.name}
                  aria-label={t('next.ds.pages.rename')}
                  autoFocus
                  onFocus={(e) => e.currentTarget.select()}
                  onBlur={() => setRenaming(null)}
                  onKeyDown={(e) => onRenameKey(e, p)}
                />
              ) : (
                <button
                  type="button"
                  className="hx-ds-pli__go"
                  aria-current={on}
                  title={p.name}
                  onClick={() => onPick(p.path)}
                >
                  <span className="hx-ds-pli__num">{number.get(p.path)}</span>
                  <span className="hx-ds-pli__name">{litName(p.name, query)}</span>
                  {changed.has(p.path) && (
                    <i
                      className="hx-ds-dotmark"
                      title={t('next.ds.pages.changed')}
                      aria-label={t('next.ds.pages.changed')}
                    />
                  )}
                </button>
              )}
              {renaming !== p.path && (hrefOf || actions) && (
                <button
                  type="button"
                  className="hx-ds-pli__more"
                  aria-label={t('next.ds.pages.more')}
                  title={t('next.ds.pages.more')}
                  aria-haspopup="menu"
                  aria-expanded={open}
                  onClick={() => {
                    setAsking(null);
                    setMenu(open ? null : p.path);
                  }}
                >
                  <Icon name="ellipsis" size="sm" />
                </button>
              )}
              {open && (
                <div className="hx-ds-pmenu" role="menu">
                  {asking === p.path && actions ? (
                    <div className="hx-ds-pmenu__ask">
                      <span>{t('next.ds.pages.removeAsk', { name: p.name })}</span>
                      <div>
                        <button
                          type="button"
                          className="hx-btn"
                          data-size="sm"
                          data-variant="ghost"
                          onClick={() => setAsking(null)}
                        >
                          {t('next.ds.pages.cancel')}
                        </button>
                        <button
                          type="button"
                          className="hx-btn"
                          data-size="sm"
                          data-variant="danger"
                          onClick={() => run(() => actions.remove(p.path))}
                        >
                          {t('next.ds.pages.removeYes')}
                        </button>
                      </div>
                    </div>
                  ) : (
                    <>
                      <button
                        type="button"
                        role="menuitem"
                        onClick={() => {
                          setMenu(null);
                          onPick(p.path);
                        }}
                      >
                        <Icon name="arrow-right" size="sm" />
                        {t('next.ds.pages.open')}
                      </button>
                      {hrefOf && (
                        <a
                          role="menuitem"
                          href={hrefOf(p.path)}
                          target="_blank"
                          rel="noreferrer"
                          onClick={() => setMenu(null)}
                        >
                          <Icon name="external-link" size="sm" />
                          {t('next.ds.pages.openTab')}
                        </a>
                      )}
                      {actions && (
                        <>
                          <button
                            type="button"
                            role="menuitem"
                            onClick={() => {
                              setMenu(null);
                              setRenaming(p.path);
                            }}
                          >
                            <Icon name="pencil" size="sm" />
                            {t('next.ds.pages.rename')}
                          </button>
                          <button
                            type="button"
                            role="menuitem"
                            onClick={() => run(() => actions.duplicate(p.path))}
                          >
                            <Icon name="copy" size="sm" />
                            {t('next.ds.pages.duplicate')}
                          </button>
                          <button
                            type="button"
                            role="menuitem"
                            data-danger
                            disabled={pages.length < 2}
                            onClick={() => setAsking(p.path)}
                          >
                            <Icon name="trash-2" size="sm" />
                            {t('next.ds.pages.remove')}
                          </button>
                        </>
                      )}
                    </>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
      {found.length === 0 && <div className="hx-ds__empty">{t('next.ds.pages.none')}</div>}
    </div>
  );
};

export default PagesTab;
