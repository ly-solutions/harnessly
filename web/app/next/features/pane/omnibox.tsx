// The browser's address field: an address, a port or words to search for, with what fits under it (the machine's
// servers, the pinned pages, the visited ones). It leaves with an animation, like every popover.
import { useId, useMemo, useRef, useState } from 'react';
import Icon from '@/next/ui/icon';
import { resolveInput, type IAddress } from '@/next/features/pane/address';
import {
  suggest,
  type ISuggestion,
  type ISuggestSources,
  type TSuggestKind,
} from '@/next/features/pane/suggest';
import { usePresentFlag } from '@/next/ui/presence';
import { t } from '@/next/i18n';

/** The list sinks out as long as the popovers do. */
export const OMNI_EXIT_MS = 160;

const ICONS: Record<TSuggestKind, string> = {
  port: 'globe',
  pin: 'pin',
  visit: 'history',
  search: 'search',
};

interface IOmniboxProps {
  /** The page now, as the bar shows it; empty for a new tab. */
  address: string;
  sources: ISuggestSources;
  onGo: (address: IAddress) => void;
  /** The bar was entered: the servers may have changed since they were asked. */
  onEnter: () => void;
}

const Omnibox = ({ address, sources, onGo, onEnter }: IOmniboxProps) => {
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  // null: the bar is not in use and shows the address; edited: the person changed the text.
  const [draft, setDraft] = useState<string | null>(null);
  const [edited, setEdited] = useState(false);
  const [active, setActive] = useState(-1);
  const typed = draft ?? address;
  const rows = useMemo(
    () => (draft === null ? [] : suggest(edited ? draft : '', sources)),
    [draft, edited, sources],
  );
  const [shown, setShown] = useState<readonly ISuggestion[]>([]);
  // The last rows stay on screen while the list leaves.
  if (rows.length > 0 && rows !== shown) setShown(rows);
  const { mounted, state } = usePresentFlag(rows.length > 0, OMNI_EXIT_MS);

  const done = () => {
    setDraft(null);
    setEdited(false);
    setActive(-1);
    input.current?.blur();
  };
  const go = (to: IAddress) => {
    done();
    onGo(to);
  };

  return (
    <div className="hx-omni">
      <input
        ref={input}
        className="hx-tbrowser__url"
        type="text"
        role="combobox"
        aria-expanded={rows.length > 0}
        aria-controls={`${id}-list`}
        aria-activedescendant={active >= 0 ? `${id}-${active}` : undefined}
        aria-autocomplete="list"
        autoComplete="off"
        autoCapitalize="off"
        spellCheck={false}
        placeholder={t('next.pane.address')}
        aria-label={t('next.pane.addressLabel')}
        value={typed}
        onFocus={(event) => {
          setDraft(address);
          setActive(-1);
          onEnter();
          event.currentTarget.select();
        }}
        onChange={(event) => {
          setDraft(event.target.value);
          setEdited(true);
          setActive(-1);
        }}
        onBlur={() => {
          setDraft(null);
          setEdited(false);
          setActive(-1);
        }}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            done();
            return;
          }
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            if (rows.length === 0) return;
            event.preventDefault();
            const step = event.key === 'ArrowDown' ? 1 : -1;
            setActive((now) => (now + step + rows.length + (now < 0 && step < 0 ? 1 : 0)) % rows.length);
            return;
          }
          if (event.key !== 'Enter') return;
          const picked = active >= 0 ? rows[active]?.address : resolveInput(typed);
          if (picked) go(picked);
        }}
      />
      {mounted && (
        <div id={`${id}-list`} className="hx-omni__list" role="listbox" data-state={state}>
          {shown.map((row, i) => (
            <button
              key={row.id}
              id={`${id}-${i}`}
              type="button"
              role="option"
              aria-selected={i === active}
              className="hx-omni__row"
              data-active={i === active}
              data-kind={row.kind}
              // The field keeps its focus: a press on a row is a choice, not the end of the typing.
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => go(row.address)}
            >
              <Icon name={ICONS[row.kind]} size="sm" />
              <span className="hx-omni__label">
                {row.kind === 'search' ? t('next.pane.suggestSearch', { q: row.label }) : row.label}
              </span>
              {row.hint !== null && <span className="hx-omni__hint">{row.hint}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
};

export default Omnibox;
