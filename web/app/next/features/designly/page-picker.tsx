// The list of a design's pages that opens from the pane's pager: a search where there are many pages, a row
// per page with its icon and its whole name (wrapped, never cut), the file under it only where it differs from the
// name, the current page lit, the page's actions over the row's end on hover or focus, the arrows moving the
// highlight and Enter going there. On the phone it is a sheet like the bar's menus. It opens from the pager under the page
// (above it) and from the address bar (below it); either way it is no taller than the window leaves it.
import { Fragment, useEffect, useLayoutEffect, useRef, useState } from 'react';
import type { ISitePage } from '@/next/features/designly/site-view';
import {
  boardGroups,
  pageFileNote,
  pagesFound,
  rowAfter,
  SEARCH_FROM,
} from '@/next/features/designly/page-pick';
import Icon from '@/next/ui/icon';
import { PHONE_QUERY } from '@/next/ui/use-media';
import { SheetGrip } from '@/next/ui/sheet';
import { t } from '@/next/i18n';

interface IPagePickerProps {
  pages: readonly ISitePage[];
  /** The page on screen. */
  path: string | null;
  state: string;
  onGo: (path: string) => void;
  onClose: () => void;
  /** Where the list stands by its button: above it (the pager) or below it, centred or from its start. */
  place?: TPickerPlace;
  /** A page's own download button at the end of its row, given where it was pressed. */
  onSave?: (path: string, at: DOMRect) => void;
  /** The download button's words for a page. */
  saveLabel?: (page: ISitePage) => string;
  /** The list is the bar's board switcher: the chat's boards come first, a new one is asked for at its foot. */
  boards?: IPickerBoards;
}

export interface IPickerBoards {
  /** The boards the agent wrote in this chat: a dot on their rows. */
  changed: ReadonlySet<string>;
  /** «Новая доска»; absent where the person may not change the design. */
  onNew?: () => void;
}

export const PICKER_PLACE = { up: 'up', center: 'center', start: 'start' } as const;
export type TPickerPlace = (typeof PICKER_PLACE)[keyof typeof PICKER_PLACE];

const PLACE_CLASS: Record<TPickerPlace, string> = {
  up: 'hx-pdock__menu',
  center: 'hx-site-menu--center',
  start: 'hx-site-menu--start',
};

/** What the list keeps clear of the window's edge. */
const EDGE = 12;

const PagePicker = ({
  pages,
  path,
  state,
  onGo,
  onClose,
  place = PICKER_PLACE.up,
  onSave,
  saveLabel,
  boards,
}: IPagePickerProps) => {
  const [query, setQuery] = useState('');
  const matched = pagesFound(pages, query);
  const groups = boards ? boardGroups(matched, boards.changed, path) : null;
  // The rows as the arrows walk them: the chat's boards, then the rest.
  const found = groups ? [...groups.chat, ...groups.rest] : matched;
  const [at, setAt] = useState(() => {
    const first = boards ? boardGroups(pages, boards.changed, path) : null;
    const list = first ? [...first.chat, ...first.rest] : pages;
    return Math.max(
      0,
      list.findIndex((p) => p.path === path),
    );
  });
  const row = found.length === 0 ? -1 : Math.min(at, found.length - 1);
  const searchable = boards !== undefined || pages.length > SEARCH_FROM;
  const box = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  // A fade at the list's foot while there is more of it below.
  const list = useRef<HTMLDivElement>(null);
  const [more, setMore] = useState(false);
  const measure = () => {
    const el = list.current;
    if (el) setMore(el.scrollTop + el.clientHeight < el.scrollHeight - 2);
  };
  useLayoutEffect(measure);

  // The room from the list's anchored edge to the window's: it scrolls inside that, never past the window.
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const fit = () => {
      const rect = el.getBoundingClientRect();
      const room = place === PICKER_PLACE.up ? rect.bottom - EDGE : window.innerHeight - rect.top - EDGE;
      el.style.setProperty('--hx-pgpick-room', `${Math.max(160, Math.round(room))}px`);
    };
    fit();
    window.addEventListener('resize', fit);
    return () => window.removeEventListener('resize', fit);
  }, [place]);

  // On the phone the search waits for a tap: the keyboard does not come up over the list.
  useEffect(() => {
    if (searchable && !matchMedia(PHONE_QUERY).matches) input.current?.focus({ preventScroll: true });
    else box.current?.focus({ preventScroll: true });
  }, [searchable]);

  // The highlighted row stays in view as the arrows move it; on opening, the page on screen stands mid-list.
  const opened = useRef(false);
  useEffect(() => {
    const block = opened.current ? 'nearest' : 'center';
    opened.current = true;
    box.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block });
  }, [row]);

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'Enter') {
      const page = found[row];
      if (!page) return;
      event.preventDefault();
      onGo(page.path);
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
      return;
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    if ((event.key === 'Home' || event.key === 'End') && event.target === input.current) return;
    event.preventDefault();
    setAt(rowAfter(event.key, row, found.length));
  };

  return (
    <div
      ref={box}
      className={`hx-site-menu ${PLACE_CLASS[place]} hx-pgpick hx-sheet-phone`}
      data-boards={boards !== undefined || undefined}
      data-state={state}
      role="dialog"
      aria-label={t('next.site.pages')}
      tabIndex={-1}
      onKeyDown={onKeyDown}
    >
      <SheetGrip />
      {searchable && (
        <label className="hx-pgpick__search">
          <Icon name="search" size="sm" />
          <input
            ref={input}
            type="text"
            value={query}
            placeholder={t(boards ? 'next.site.findBoard' : 'next.site.findPage')}
            aria-label={t(boards ? 'next.site.findBoard' : 'next.site.findPage')}
            onChange={(event) => {
              setQuery(event.target.value);
              setAt(0);
            }}
          />
        </label>
      )}
      <div
        ref={list}
        className="hx-pgpick__list"
        role="listbox"
        aria-label={t('next.site.pages')}
        data-more={more || undefined}
        onScroll={measure}
      >
        {found.map((p, i) => {
          const note = pageFileNote(p);
          const title =
            groups === null
              ? null
              : i === 0 && groups.chat.length > 0
                ? t('next.site.boardsOfChat')
                : i === groups.chat.length
                  ? t('next.site.boardsAll', { n: pages.length })
                  : null;
          return (
            <Fragment key={p.path}>
              {title !== null && <div className="hx-pgpick__group">{title}</div>}
              <div className="hx-pgpick__item" data-active={i === row}>
                <button
                  type="button"
                  role="option"
                  tabIndex={-1}
                  className="hx-pgpick__row"
                  aria-selected={p.path === path}
                  data-active={i === row}
                  onMouseMove={() => i !== row && setAt(i)}
                  onClick={() => onGo(p.path)}
                >
                  <Icon name={boards ? 'app-window' : 'file-text'} size="sm" className="hx-pgpick__ic" />
                  <span className="hx-pgpick__text">
                    <span className="hx-pgpick__name">{p.name}</span>
                    {note !== null && !boards && <span className="hx-pgpick__meta">{note}</span>}
                  </span>
                  {groups?.openOnly === p.path && (
                    <span className="hx-pgpick__tag">{t('next.site.boardOpen')}</span>
                  )}
                  {boards?.changed.has(p.path) && (
                    <span className="hx-pgpick__dot" title={t('next.site.boardChanged')} />
                  )}
                </button>
                {onSave && (
                  <span className="hx-pgpick__acts">
                    <button
                      type="button"
                      className="hx-head-btn hx-pgpick__dl"
                      aria-label={saveLabel?.(p)}
                      title={saveLabel?.(p)}
                      onClick={(event) => onSave(p.path, event.currentTarget.getBoundingClientRect())}
                    >
                      <Icon name="download" size="sm" />
                    </button>
                  </span>
                )}
              </div>
            </Fragment>
          );
        })}
        {found.length === 0 && <div className="hx-pgpick__empty">{t('next.site.pagesNone')}</div>}
      </div>
      {boards?.onNew && (
        <button type="button" className="hx-pgpick__new" onClick={boards.onNew}>
          <Icon name="plus" size="sm" />
          <span>{t('next.site.boardNew')}</span>
        </button>
      )}
    </div>
  );
};

export default PagePicker;
