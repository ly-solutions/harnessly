import { Dialog } from '@base-ui/react/dialog';
import { Menu } from '@base-ui/react/menu';
import { isMac } from '@/next/features/keys/chord';
import { useKeyHint } from '@/next/features/keys/keys-help';
import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import Icon from '@/next/ui/icon';
import MenuPanel, { useMenuActions } from '@/next/ui/menu-panel';
import Switch from '@/next/ui/switch';
import { t } from '@/next/i18n';
import { usePortalContainer } from '@/next/ui/portal';
import LoadFailed, { LOAD_FAILED_SIZE } from '@/next/ui/load-failed';
import { useDelayed as useDelayedFlag } from '@/next/ui/skeleton';
import { partsOf } from '@/next/features/search/highlight';
import { SETTING_OFF, SETTING_ON } from '@/next/features/search/sources-api';
import type { SettingsSection } from '@/next/routes/routes';

export interface ISearchScope {
  id: string;
  label: string;
}

/** Where a row leads when it is opened. A row without one is a chat (its id) or a message (its target). */
export const ROW_GO = {
  setting: 'setting',
  section: 'section',
  design: 'design',
  link: 'link',
  review: 'review',
  newChat: 'newChat',
  designs: 'designs',
} as const;

export type TSearchGo =
  /** A setting of the catalog, in the section of Settings it is changed in. */
  | { k: typeof ROW_GO.setting; key: string; section: SettingsSection }
  /** A section of Settings; null is Settings itself. */
  | { k: typeof ROW_GO.section; section: SettingsSection | null }
  | { k: typeof ROW_GO.design; id: string }
  /** An address outside the page: a document, an issue, a wiki page. */
  | { k: typeof ROW_GO.link; url: string }
  /** A merge request, opened in the review. */
  | { k: typeof ROW_GO.review; url: string }
  | { k: typeof ROW_GO.newChat }
  | { k: typeof ROW_GO.designs };

export const ROW_WIDGET = { toggle: 'toggle', pick: 'pick' } as const;

/** The control a setting's row carries: the setting is changed without leaving the search. */
export type ISearchWidget =
  | { kind: typeof ROW_WIDGET.toggle; on: boolean }
  | {
      kind: typeof ROW_WIDGET.pick;
      value: string;
      label: string;
      options: readonly { value: string; label: string }[];
    };

export interface ISearchRow {
  id: string;
  /** chat, message, file, link, design: the icon's look. */
  kind: string;
  icon: string;
  title: string;
  /** The words around the match, for a message. */
  snippet?: string;
  /** Where it is: a chat's project, a file's chat, a date. */
  meta: string;
  /** For a message: the chat it is in and the place in it that opens. */
  target?: { chatId: string; entryId: string; turnId: string | null };
  go?: TSearchGo;
  widget?: ISearchWidget;
  /** What the side of the search says about the row: what it is, and a line about it. */
  preview?: { kind: string; text: string };
}

export const ANSWER = { wait: 'wait', ready: 'ready', noModel: 'noModel', failed: 'failed' } as const;

/** The model's answer to a question in words. */
export type TSearchAnswer =
  | { k: typeof ANSWER.wait }
  /** `sources` are the rows the answer rests on. */
  | { k: typeof ANSWER.ready; text: string; sources: readonly { row: ISearchRow; label: string }[] }
  /** No model is picked for the answers. */
  | { k: typeof ANSWER.noModel }
  | { k: typeof ANSWER.failed; message: string };

/** What makes the dialog the search of everything: the side with the selected row, the answer, settings in rows. */
export interface ISearchSmart {
  /** null when the words are not a question. */
  answer: TSearchAnswer | null;
  /** Asks the model about the words as they are (Tab). */
  onAsk: () => void;
  /** Changes a setting; resolves true when the machine took it. */
  onSetting: (key: string, value: string) => Promise<boolean>;
  onAddSource: () => void;
  onPickModel: () => void;
}

export interface ISearchGroup {
  id: string;
  label: string;
  rows: readonly ISearchRow[];
}

export interface ISearchDialogProps {
  open: boolean;
  onClose: () => void;
  query: string;
  onQuery: (query: string) => void;
  scopes: readonly ISearchScope[];
  scope: string;
  onScope: (scope: string) => void;
  /** The answer for the query; null while it is on its way. */
  results: readonly ISearchGroup[] | null;
  /** What shows before anything is typed. */
  recent: readonly ISearchRow[];
  /** Other words for the field and the box (the chat navigator uses this dialog); the search's own when absent. */
  words?: { title: string; placeholder: string };
  /** What shows before anything is typed instead of the recent (grouped as the results are). */
  idle?: readonly ISearchGroup[];
  onPick: (row: ISearchRow) => void;
  /** The machine is still reading its older chats, so the messages found may not be all there are. */
  indexing?: boolean;
  /** The chat list did not come: why, and how to ask again. null while it is on its way or here. */
  failed?: { reason: string; onRetry: () => void | Promise<unknown> } | null;
  /** The words lit in the rows when they are not the query itself (the keywords of a question). */
  marks?: string;
  smart?: ISearchSmart;
}

const SAVED_MS = 1600;
const COPIED_MS = 1400;

const ACT = { widget: 'widget', open: 'open', copy: 'copy' } as const;
type TAct = (typeof ACT)[keyof typeof ACT];

interface IRowAction {
  id: TAct;
  icon: string;
  label: string;
  key: string;
}

const urlOf = (row: ISearchRow): string | null =>
  row.go && (row.go.k === ROW_GO.link || row.go.k === ROW_GO.review) ? row.go.url : null;

const openLabel = (row: ISearchRow): string => {
  if (row.go?.k === ROW_GO.setting || row.go?.k === ROW_GO.section) return t('next.search.actOpenSettings');
  if (row.go?.k === ROW_GO.newChat) return t('next.search.actCreate');
  return t('next.search.actOpen');
};

/** What can be done with the selected row, the first by Enter. */
const actionsOf = (row: ISearchRow, mod: string): IRowAction[] => {
  const open = (key: string): IRowAction => ({
    id: ACT.open,
    icon: row.go?.k === ROW_GO.setting ? 'settings' : 'corner-down-left',
    label: openLabel(row),
    key,
  });
  if (row.widget?.kind === ROW_WIDGET.toggle)
    return [
      {
        id: ACT.widget,
        icon: 'corner-down-left',
        label: row.widget.on ? t('next.search.actOff') : t('next.search.actOn'),
        key: '↵',
      },
      open(`${mod}↵`),
    ];
  if (row.widget?.kind === ROW_WIDGET.pick)
    return [
      { id: ACT.widget, icon: 'corner-down-left', label: t('next.search.actChoose'), key: '↵' },
      open(`${mod}↵`),
    ];
  if (urlOf(row) !== null)
    return [open('↵'), { id: ACT.copy, icon: 'copy', label: t('next.search.actCopy'), key: '' }];
  return [open('↵')];
};

const groupsShown = (
  typed: boolean,
  results: readonly ISearchGroup[] | null,
  recent: readonly ISearchRow[],
  idle?: readonly ISearchGroup[],
): readonly ISearchGroup[] => {
  if (typed) return results ?? [];
  if (idle) return idle;
  if (recent.length === 0) return [];
  return [{ id: 'recent', label: t('next.search.recent'), rows: recent }];
};

const Highlighted = ({ text, query }: { text: string; query: string }) => (
  <>
    {partsOf(text, query).map((part, i) => (
      <span key={i} data-hit={part.hit || undefined}>
        {part.text}
      </span>
    ))}
  </>
);

interface IPickProps {
  row: ISearchRow;
  widget: Extract<ISearchWidget, { kind: typeof ROW_WIDGET.pick }>;
  open: boolean;
  busy: boolean;
  onOpen: (open: boolean) => void;
  onPick: (value: string) => void;
}

/** The value of a setting with a list of values: a menu of them, opened from the row. */
const RowPick = ({ row, widget, open, busy, onOpen, onPick }: IPickProps) => {
  const actions = useMenuActions();
  return (
    <Menu.Root actionsRef={actions} open={open} onOpenChange={onOpen}>
      <Menu.Trigger
        render={
          <button type="button" className="hx-srch__pick" disabled={busy} aria-label={row.title}>
            <span>{widget.label}</span>
            <Icon name="chevron-down" size="sm" />
          </button>
        }
      />
      <MenuPanel actions={actions} className="hx-cmenu" label={row.title} align="end" menu="search-pick">
        {widget.options.map((option) => (
          <Menu.Item
            key={option.value}
            className="hx-cmenu__item"
            role="menuitemradio"
            aria-checked={option.value === widget.value}
            onClick={() => {
              if (option.value !== widget.value) onPick(option.value);
            }}
          >
            <span className="hx-cmenu__txt">{option.label}</span>
            <Icon name="check" size="sm" className="hx-cmenu__check" />
          </Menu.Item>
        ))}
      </MenuPanel>
    </Menu.Root>
  );
};

/**
 * Search, ⌘K: one field, «where» scopes, results by group with the match lit, ↑↓ and Enter. An empty field shows what
 * to go on with; nothing found says so with a hint. With `smart` it is the search of everything: the side says what is
 * selected and what can be done with it, a setting is changed in its row, and a question gets the model's answer.
 */
const SearchDialog = ({
  open,
  onClose,
  query,
  onQuery,
  scopes,
  scope,
  onScope,
  results,
  recent,
  words,
  idle,
  onPick,
  failed = null,
  indexing = false,
  marks,
  smart,
}: ISearchDialogProps) => {
  const searchKey = useKeyHint('global.search');
  const container = usePortalContainer();
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const [selected, setSelected] = useState(0);
  /** The setting being saved, the one just saved, and the row whose menu of values is open. */
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [savedKey, setSavedKey] = useState<string | null>(null);
  const [pickOpen, setPickOpen] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const typed = query.trim() !== '';
  const lit = marks ?? query;
  const groups = groupsShown(typed, results, recent, idle);
  const starts = groups.map((_, i) => groups.slice(0, i).reduce((sum, group) => sum + group.rows.length, 0));
  const rows = groups.flatMap((group) => group.rows);
  const at = Math.min(selected, Math.max(0, rows.length - 1));
  const current = rows[at] ?? null;
  const currentGroup = groups.find(
    (group, i) => at >= (starts[i] ?? 0) && at < (starts[i] ?? 0) + group.rows.length,
  );
  const unavailable = failed !== null && results === null;
  const searching = useDelayedFlag(typed && results === null && !unavailable);
  const nothing = typed && results !== null && rows.length === 0;
  const mod = isMac() ? '⌘' : 'Ctrl ';
  const answer = smart?.answer ?? null;

  useEffect(() => {
    if (savedKey === null) return;
    const timer = setTimeout(() => setSavedKey(null), SAVED_MS);
    return () => clearTimeout(timer);
  }, [savedKey]);
  useEffect(() => {
    if (copied === null) return;
    const timer = setTimeout(() => setCopied(null), COPIED_MS);
    return () => clearTimeout(timer);
  }, [copied]);

  const move = (delta: number) => {
    if (rows.length === 0) return;
    const next = (at + delta + rows.length) % rows.length;
    setSelected(next);
    listRef.current?.querySelector(`[data-row="${next}"]`)?.scrollIntoView({ block: 'nearest' });
  };

  /** The setting is changed on the machine first; the row shows the new value when the machine took it. */
  const change = (row: ISearchRow, value: string) => {
    if (!smart || row.go?.k !== ROW_GO.setting || busyKey !== null) return;
    const key = row.go.key;
    setBusyKey(key);
    void smart.onSetting(key, value).then((taken) => {
      setBusyKey(null);
      if (taken) setSavedKey(key);
    });
  };

  const act = (row: ISearchRow, id: TAct) => {
    if (id === ACT.copy) {
      const url = urlOf(row);
      if (url !== null) void navigator.clipboard.writeText(url).then(() => setCopied(row.id));
      return;
    }
    if (id === ACT.widget && smart && row.widget?.kind === ROW_WIDGET.toggle) {
      change(row, row.widget.on ? SETTING_OFF : SETTING_ON);
      return;
    }
    if (id === ACT.widget && smart && row.widget?.kind === ROW_WIDGET.pick) {
      setPickOpen(row.id);
      return;
    }
    onPick(row);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      move(1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      move(-1);
    } else if (event.key === 'Enter') {
      event.preventDefault();
      if (!current) return;
      const withWidget = smart !== undefined && current.widget !== undefined;
      act(current, withWidget && !(event.metaKey || event.ctrlKey) ? ACT.widget : ACT.open);
    } else if (event.key === 'Tab' && smart) {
      event.preventDefault();
      if (typed) smart.onAsk();
    } else if (event.key === 'Tab' && scopes.length > 0) {
      event.preventDefault();
      const index = scopes.findIndex((item) => item.id === scope);
      const next = scopes[(index + (event.shiftKey ? -1 : 1) + scopes.length) % scopes.length];
      if (next) onScope(next.id);
    }
  };

  const select = (row: ISearchRow) => {
    const index = rows.findIndex((item) => item.id === row.id);
    if (index < 0) return;
    setSelected(index);
    listRef.current?.querySelector(`[data-row="${index}"]`)?.scrollIntoView({ block: 'nearest' });
  };

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <Dialog.Portal container={container}>
        <div className="hx-srch">
          <Dialog.Backdrop className="hx-srch__scrim" />
          <Dialog.Popup
            className="hx-srch__box"
            data-wide={smart !== undefined || undefined}
            aria-label={words?.title ?? t('next.search.title')}
            initialFocus={inputRef}
          >
            <label className="hx-srch__field">
              <Icon name="search" />
              <input
                ref={inputRef}
                type="search"
                placeholder={words?.placeholder ?? t('next.search.placeholder')}
                autoComplete="off"
                enterKeyHint="search"
                value={query}
                onChange={(event) => {
                  setSelected(0);
                  onQuery(event.target.value);
                }}
                onKeyDown={onKeyDown}
              />
              {typed && (
                <button
                  type="button"
                  className="hx-srch__clear"
                  aria-label={t('next.search.clear')}
                  onClick={() => {
                    onQuery('');
                    inputRef.current?.focus();
                  }}
                >
                  <Icon name="circle-x" size="sm" />
                </button>
              )}
              <Dialog.Close className="hx-srch__cancel">{t('next.search.cancel')}</Dialog.Close>
            </label>
            {scopes.length > 0 && (
              <div className="hx-srch__scopes" role="tablist" aria-label={t('next.search.where')}>
                {scopes.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    role="tab"
                    className="hx-srch__scope"
                    aria-selected={item.id === scope}
                    onClick={() => {
                      setSelected(0);
                      onScope(item.id);
                    }}
                  >
                    {item.label}
                  </button>
                ))}
                {smart && (
                  <button
                    type="button"
                    className="hx-srch__scope hx-srch__scope--add"
                    onClick={smart.onAddSource}
                  >
                    <Icon name="plus" size="sm" />
                    {t('next.search.addSource')}
                  </button>
                )}
              </div>
            )}
            <div className="hx-srch__body">
              <div
                className="hx-srch__list"
                ref={listRef}
                role="listbox"
                aria-label={t('next.search.results')}
              >
                {answer && (
                  <div className="hx-srch__ans" role="status">
                    <div className="hx-srch__ans-head">
                      <Icon name="sparkles" size="sm" />
                      {t('next.search.answerTitle')}
                    </div>
                    {answer.k === ANSWER.wait && (
                      <div className="hx-srch__ans-wait">
                        <Icon name="loader-circle" size="sm" />
                        {t('next.search.answerWait')}
                      </div>
                    )}
                    {answer.k === ANSWER.ready && (
                      <>
                        <div className="hx-srch__ans-text">{answer.text}</div>
                        {answer.sources.length > 0 && (
                          <div className="hx-srch__ans-srcs">
                            {answer.sources.map((source) => (
                              <button
                                key={source.row.id}
                                type="button"
                                className="hx-srch__ans-src"
                                onClick={() => select(source.row)}
                              >
                                <Icon name={source.row.icon} size="sm" />
                                <span>{source.label}</span>
                              </button>
                            ))}
                          </div>
                        )}
                      </>
                    )}
                    {answer.k === ANSWER.noModel && smart && (
                      <div className="hx-srch__ans-wait">
                        <span>{t('next.search.answerNoModel')}</span>
                        <button type="button" className="hx-srch__ans-src" onClick={smart.onPickModel}>
                          {t('next.search.answerPickModel')}
                        </button>
                      </div>
                    )}
                    {answer.k === ANSWER.failed && <div className="hx-srch__ans-wait">{answer.message}</div>}
                  </div>
                )}
                {searching && (
                  <div className="hx-srch__wait" role="status">
                    <Icon name="loader-circle" size="sm" />
                    {t('next.search.searching')}
                  </div>
                )}
                {groups.map((group, groupIndex) => (
                  <div key={group.id} className="hx-srch__group" role="group" aria-label={group.label}>
                    <div className="hx-srch__label">{group.label}</div>
                    {group.rows.map((row, rowIndex) => {
                      const mine = (starts[groupIndex] ?? 0) + rowIndex;
                      const settingKey = row.go?.k === ROW_GO.setting ? row.go.key : null;
                      const widget = smart ? row.widget : undefined;
                      return (
                        <div
                          key={row.id}
                          role="option"
                          tabIndex={-1}
                          aria-selected={mine === at}
                          className="hx-srch__row"
                          data-row={mine}
                          data-sel={mine === at}
                          data-kind={row.kind}
                          onClick={() => {
                            // A row with a control is changed by its control; the row itself only takes the selection.
                            if (widget) setSelected(mine);
                            else onPick(row);
                          }}
                          onPointerMove={() => {
                            if (mine !== at) setSelected(mine);
                          }}
                        >
                          <span className="hx-srch__ic">
                            <Icon name={row.icon} size="sm" />
                          </span>
                          <span className="hx-srch__text">
                            <span className="hx-srch__title">
                              <Highlighted text={row.title} query={lit} />
                            </span>
                            {row.snippet && (
                              <span className="hx-srch__snip">
                                <Highlighted text={row.snippet} query={lit} />
                              </span>
                            )}
                          </span>
                          {settingKey !== null && savedKey === settingKey && (
                            <span className="hx-srch__saved">{t('next.search.saved')}</span>
                          )}
                          {widget?.kind === ROW_WIDGET.toggle && (
                            <Switch
                              label={row.title}
                              checked={widget.on}
                              disabled={busyKey !== null}
                              onChange={(on) => change(row, on ? SETTING_ON : SETTING_OFF)}
                            />
                          )}
                          {widget?.kind === ROW_WIDGET.pick && (
                            <RowPick
                              row={row}
                              widget={widget}
                              open={pickOpen === row.id}
                              busy={busyKey !== null}
                              onOpen={(next) => setPickOpen(next ? row.id : null)}
                              onPick={(value) => change(row, value)}
                            />
                          )}
                          {!widget && <span className="hx-srch__meta">{row.meta}</span>}
                          {!widget && !smart && (
                            <Icon name="corner-down-left" size="sm" className="hx-srch__go" />
                          )}
                        </div>
                      );
                    })}
                  </div>
                ))}
                {typed && indexing && rows.length > 0 && (
                  <div className="hx-srch__note">{t('next.search.indexing')}</div>
                )}
                {nothing && (
                  <div className="hx-srch__none">
                    <b>{t('next.search.nothing')}</b>
                    <span>{t('next.search.nothingHint')}</span>
                    {indexing && <span>{t('next.search.indexing')}</span>}
                  </div>
                )}
                {unavailable && (
                  <LoadFailed
                    size={LOAD_FAILED_SIZE.pane}
                    title={t('next.load.search')}
                    reason={failed.reason}
                    onRetry={failed.onRetry}
                  />
                )}
                {!typed && recent.length === 0 && !idle && !unavailable && (
                  <div className="hx-srch__none">
                    <span>{t('next.search.start')}</span>
                  </div>
                )}
              </div>
              {smart && current && (
                <aside className="hx-srch__side" key={current.id}>
                  <div>
                    <div className="hx-srch__side-kind">{current.preview?.kind ?? currentGroup?.label}</div>
                    <h3 className="hx-srch__side-title">{current.title}</h3>
                  </div>
                  <div className="hx-srch__side-text">
                    <Highlighted
                      text={current.preview?.text ?? current.snippet ?? current.meta}
                      query={lit}
                    />
                  </div>
                  <div className="hx-srch__acts">
                    {actionsOf(current, mod).map((action) => (
                      <button
                        key={action.id}
                        type="button"
                        className="hx-srch__act"
                        onClick={() => act(current, action.id)}
                      >
                        <Icon name={action.icon} size="sm" />
                        <span>
                          {action.id === ACT.copy && copied === current.id
                            ? t('next.search.copied')
                            : action.label}
                        </span>
                        {action.key !== '' && <kbd>{action.key}</kbd>}
                      </button>
                    ))}
                  </div>
                </aside>
              )}
            </div>
            <div className="hx-srch__foot">
              <span>
                <kbd>↑</kbd>
                <kbd>↓</kbd>
                {t('next.search.keySelect')}
              </span>
              <span>
                <kbd>↵</kbd>
                {t('next.search.keyOpen')}
              </span>
              {smart && (
                <span>
                  <kbd>Tab</kbd>
                  {t('next.search.keyAsk')}
                </span>
              )}
              {!smart && scopes.length > 0 && (
                <span>
                  <kbd>Tab</kbd>
                  {t('next.search.where')}
                </span>
              )}
              <span className="hx-spacer" />
              {searchKey && (
                <span>
                  <kbd>{searchKey}</kbd>
                </span>
              )}
            </div>
          </Dialog.Popup>
        </div>
      </Dialog.Portal>
    </Dialog.Root>
  );
};

export default SearchDialog;
