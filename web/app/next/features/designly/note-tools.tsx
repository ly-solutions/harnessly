// The row of the hand under the card's field: bold, italic, underline, struck, smaller and larger, the inks,
// and behind «⋯» the fill, the corners and the opacity. With words selected on the board the type goes to them, else to
// the whole block; what is changed shows on the block at once and the card writes it into the board's source.
import { useLayoutEffect, useRef, type ReactNode } from 'react';
import {
  changedKeys,
  NO_EDIT,
  OPACITY,
  shownValues,
  stepRadius,
  stepSize,
  SWATCHES,
  TEXT_SWATCHES,
  toggleLine,
  withValue,
  type IStyleEdit,
  type TStyleKey,
} from '@/next/features/designly/element-style';
import type { IBoardHand, THandCommand } from '@/next/features/designly/hand-edit';
import type { IPicked, IPickedLook } from '@/next/features/designly/page-bridge';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

/** One step of «A−» and «A+» on selected words; the size is kept within the bounds. */
const SIZE_STEP = 1.1;
const WORDS_SIZE = { min: 8, max: 200 } as const;
const BOLD = 600;

export const nextSize = (size: number, direction: -1 | 1): number =>
  Math.max(
    WORDS_SIZE.min,
    Math.min(WORDS_SIZE.max, Math.round(direction > 0 ? size * SIZE_STEP : size / SIZE_STEP)),
  );

/** The colour of a swatch reaches the CSS through a custom property of its own, not through a style attribute. */
const useSwatchColor = <E extends HTMLElement>(color: string | null) => {
  const ref = useRef<E>(null);
  useLayoutEffect(() => {
    if (color) ref.current?.style.setProperty('--sw', color);
  }, [color]);
  return ref;
};

/** The chip of a colour; crossed out when there is none. */
export const SwatchDot = ({ color }: { color: string | null }) => {
  const ref = useSwatchColor<HTMLElement>(color);
  return <i ref={ref} className="hx-ann__sw" data-none={color === null} />;
};

const SwatchButton = ({
  color,
  label,
  on,
  onPick,
}: {
  color: string | null;
  label: string;
  on: boolean;
  onPick: () => void;
}) => {
  const ref = useSwatchColor<HTMLButtonElement>(color);
  return (
    <button
      ref={ref}
      type="button"
      className="hx-ann__swb"
      role="radio"
      aria-checked={on}
      aria-label={label}
      title={label}
      data-none={color === null}
      onClick={onPick}
    />
  );
};

/** The opacity slider: the share of its track that is filled goes to the CSS as `--v`. */
const OpacityRange = ({ value, onChange }: { value: number; onChange: (value: number) => void }) => {
  const ref = useRef<HTMLInputElement>(null);
  useLayoutEffect(() => {
    const share = ((value - OPACITY.min) / (OPACITY.max - OPACITY.min)) * 100;
    ref.current?.style.setProperty('--v', `${Math.min(100, Math.max(0, share)).toFixed(1)}%`);
  }, [value]);
  return (
    <span className="hx-ann__range">
      <input
        ref={ref}
        type="range"
        min={OPACITY.min}
        max={OPACITY.max}
        step={OPACITY.step}
        value={Math.min(OPACITY.max, Math.max(OPACITY.min, value))}
        aria-label={t('next.notes.style.opacity')}
        onChange={(event) => onChange(Number(event.target.value))}
      />
      <span className="hx-ann__vtext">{value}%</span>
    </span>
  );
};

export interface INoteToolsProps {
  picked: IPicked;
  look: IPickedLook;
  edit: IStyleEdit;
  onEdit: (edit: IStyleEdit) => void;
  /** The held block on the board; null where the source takes no hand edit. */
  hand: IBoardHand | null;
  /** «⋯» is unfolded. */
  open: boolean;
  onOpen: (open: boolean) => void;
  /** The page no longer has the block: the look cannot be shown on it. */
  gone: boolean;
}

const NoteTools = ({ picked, look, edit, onEdit, hand, open, onOpen, gone }: INoteToolsProps) => {
  const shown = shownValues(edit, look);
  const state = hand?.state ?? null;
  const selected = state?.selected === true;
  const typed = look.font !== undefined && picked.text !== '';
  const changed = changedKeys(edit).length > 0 || state?.changed === true;
  const set = <K extends TStyleKey>(key: K, value: IStyleEdit[K]) =>
    onEdit(withValue(edit, look, key, value));
  const bold = (shown.weight ?? 400) >= BOLD;

  /** A button of the type: on selected words the board's own command, else a style of the whole block. */
  const mark = (
    id: 'bold' | 'italic' | 'underline' | 'strike',
    command: THandCommand,
    on: boolean,
    whole: () => void,
    body: ReactNode,
  ) => (
    <button
      type="button"
      className="hx-ann__tool hx-tipped"
      data-mark={id}
      aria-label={t(`next.site.text.${id}`)}
      aria-pressed={selected ? undefined : on}
      onClick={() => (selected ? hand?.command(command) : whole())}
    >
      {body}
      <span className="hx-tip" data-pos="below">
        {t(`next.site.text.${id}`)}
      </span>
    </button>
  );
  const resize = (direction: -1 | 1) => () => {
    if (selected && state) hand?.wrap('font-size', `${nextSize(state.size, direction)}px`);
    else set('size', stepSize(shown.size, direction));
  };
  const size = (direction: -1 | 1) => {
    const label = t(direction > 0 ? 'next.site.text.larger' : 'next.site.text.smaller');
    return (
      <button type="button" className="hx-ann__tool hx-tipped" aria-label={label} onClick={resize(direction)}>
        A<small>{direction > 0 ? '+' : '−'}</small>
        <span className="hx-tip" data-pos="below">
          {label}
        </span>
      </button>
    );
  };

  return (
    // A press on the row keeps the words selected on the board and the caret where it is.
    <div
      className="hx-ann__hand"
      onMouseDown={(event) => event.target instanceof HTMLInputElement || event.preventDefault()}
    >
      <div className="hx-ann__tools" role="toolbar" aria-label={t('next.notes.tools')}>
        {typed && (
          <>
            {mark(
              'bold',
              'bold',
              bold,
              () => set('weight', bold ? 400 : 700),
              <b>{t('next.site.text.boldMark')}</b>,
            )}
            {mark(
              'italic',
              'italic',
              shown.slant === 'italic',
              () => set('slant', shown.slant === 'italic' ? 'normal' : 'italic'),
              <i>{t('next.site.text.italicMark')}</i>,
            )}
            {mark(
              'underline',
              'underline',
              shown.line.includes('underline'),
              () => set('line', toggleLine(shown.line, 'underline')),
              <u>{t('next.site.text.underlineMark')}</u>,
            )}
            {mark(
              'strike',
              'strikeThrough',
              shown.line.includes('line-through'),
              () => set('line', toggleLine(shown.line, 'line-through')),
              <s>{t('next.site.text.strikeMark')}</s>,
            )}
            <span className="hx-ann__sep" />
            {size(-1)}
            {size(1)}
            <span className="hx-ann__sep" />
            <span className="hx-ann__inks" role="radiogroup" aria-label={t('next.notes.style.color')}>
              {[null, ...TEXT_SWATCHES].map((color) => (
                <SwatchButton
                  key={color ?? 'none'}
                  color={color}
                  label={color ?? t('next.notes.style.asTheme')}
                  on={!selected && (color?.toLowerCase() ?? null) === (shown.color?.toLowerCase() ?? null)}
                  onPick={() => (selected ? hand?.wrap('color', color) : set('color', color))}
                />
              ))}
            </span>
          </>
        )}
        <span className="hx-spacer" />
        {changed && (
          <button
            type="button"
            className="hx-ann__tool hx-ann__reset hx-tipped"
            aria-label={t('next.notes.reset')}
            onClick={() => {
              onEdit(NO_EDIT);
              hand?.revert();
            }}
          >
            <Icon name="rotate-ccw" size="sm" />
            <span className="hx-tip" data-pos="below" data-align="end">
              {t('next.notes.reset')}
            </span>
          </button>
        )}
        <button
          type="button"
          className="hx-ann__tool hx-ann__more hx-tipped"
          aria-label={t('next.notes.more')}
          aria-expanded={open}
          onClick={() => onOpen(!open)}
        >
          <Icon name="ellipsis" size="sm" />
          <span className="hx-tip" data-pos="below" data-align="end">
            {t('next.notes.more')}
          </span>
        </button>
      </div>
      <div className="hx-ann__props" aria-hidden={!open} inert={!open}>
        <div className="hx-ann__props-in">
          <div className="hx-ann__grid">
            <div className="hx-ann__prop" data-changed={edit.background !== null}>
              <span className="hx-ann__label">{t('next.notes.style.background')}</span>
              <span className="hx-ann__sws" role="radiogroup" aria-label={t('next.notes.style.background')}>
                {[null, ...SWATCHES].map((color) => (
                  <SwatchButton
                    key={color ?? 'none'}
                    color={color}
                    label={color ?? t('next.notes.style.asTheme')}
                    on={(color?.toLowerCase() ?? null) === (shown.background?.toLowerCase() ?? null)}
                    onPick={() => set('background', color)}
                  />
                ))}
              </span>
            </div>
            <div className="hx-ann__prop" data-changed={edit.radius !== null}>
              <span className="hx-ann__label">{t('next.notes.style.radius')}</span>
              <span className="hx-ann__step">
                <button
                  type="button"
                  aria-label={t('next.notes.style.less')}
                  onClick={() => set('radius', stepRadius(shown.radius, -1))}
                >
                  <Icon name="minus" size="sm" />
                </button>
                <span className="hx-ann__vtext">{shown.radius === null ? '—' : `${shown.radius} px`}</span>
                <button
                  type="button"
                  aria-label={t('next.notes.style.more')}
                  onClick={() => set('radius', stepRadius(shown.radius, 1))}
                >
                  <Icon name="plus" size="sm" />
                </button>
              </span>
            </div>
            <div className="hx-ann__prop" data-changed={edit.opacity !== null}>
              <span className="hx-ann__label">{t('next.notes.style.opacity')}</span>
              <OpacityRange value={shown.opacity} onChange={(value) => set('opacity', value)} />
            </div>
          </div>
        </div>
      </div>
      {state?.dynamic && <div className="hx-ann__hint">{t('next.site.text.dynamic')}</div>}
      {gone && <div className="hx-ann__hint">{t('next.notes.style.gone')}</div>}
    </div>
  );
};

export default NoteTools;
