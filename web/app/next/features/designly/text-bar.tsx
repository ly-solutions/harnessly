// The bar of a text edited by hand: bold, italic, underline, struck, smaller and larger, the colours, the
// eraser and «Done», in a strip over the page so the text is never covered.
import type { ReactNode } from 'react';
import type { ITextState } from '@/next/features/designly/hand-edit';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

/** The inks of the bar; null gives the block the page's own colour back. */
export const TEXT_INKS = [
  { id: 'theme', color: null },
  { id: 'accent', color: '#7b6cf0' },
  { id: 'red', color: '#e5484d' },
  { id: 'grey', color: '#8a8f98' },
] as const;
/** One step of «A−» and «A+». */
const SIZE_STEP = 1.1;
const SIZE = { min: 8, max: 200 } as const;

export const nextSize = (size: number, direction: -1 | 1): number =>
  Math.max(SIZE.min, Math.min(SIZE.max, Math.round(direction > 0 ? size * SIZE_STEP : size / SIZE_STEP)));

interface ITextBarProps {
  text: ITextState;
  /** Sends a message to the board. */
  post: (message: Record<string, unknown>) => void;
}

const TextBar = ({ text, post }: ITextBarProps) => {
  const button = (id: string, label: string, body: ReactNode, act: () => void, kbd?: string) => (
    <button key={id} type="button" className="hx-textbar__btn hx-tipped" aria-label={label} onClick={act}>
      {body}
      <span className="hx-tip" data-pos="below">
        {label}
        {kbd && <kbd>{kbd}</kbd>}
      </span>
    </button>
  );
  const command = (cmd: string) => () => post({ type: 'tk:text-cmd', cmd });
  const resize = (direction: -1 | 1) => () =>
    post({ type: 'tk:text-style', prop: 'font-size', value: `${nextSize(text.size, direction)}px` });
  return (
    // A press on the bar keeps the words selected on the board.
    <div
      className="hx-textbar"
      role="toolbar"
      aria-label={t('next.site.text.bar', { name: text.label })}
      onMouseDown={(event) => event.preventDefault()}
    >
      {button('bold', t('next.site.text.bold'), <b>{t('next.site.text.boldMark')}</b>, command('bold'), '⌘B')}
      {button(
        'italic',
        t('next.site.text.italic'),
        <i>{t('next.site.text.italicMark')}</i>,
        command('italic'),
        '⌘I',
      )}
      {button(
        'underline',
        t('next.site.text.underline'),
        <u>{t('next.site.text.underlineMark')}</u>,
        command('underline'),
        '⌘U',
      )}
      {button(
        'strike',
        t('next.site.text.strike'),
        <s>{t('next.site.text.strikeMark')}</s>,
        command('strikeThrough'),
      )}
      <span className="hx-textbar__sep" />
      {button(
        'smaller',
        t('next.site.text.smaller'),
        <>
          A<small>−</small>
        </>,
        resize(-1),
      )}
      {button(
        'larger',
        t('next.site.text.larger'),
        <>
          A<small>+</small>
        </>,
        resize(1),
      )}
      <span className="hx-textbar__sep" />
      {TEXT_INKS.map((ink) =>
        button(
          ink.id,
          t(`next.site.text.ink.${ink.id}`),
          <span className="hx-textbar__ink" data-ink={ink.id} />,
          () => post({ type: 'tk:text-style', prop: 'color', value: ink.color }),
        ),
      )}
      <span className="hx-textbar__sep" />
      {button('clear', t('next.site.text.clear'), <Icon name="eraser" size="sm" />, command('removeFormat'))}
      <button type="button" className="hx-textbar__done" onClick={() => post({ type: 'tk:text-done' })}>
        {t('next.site.text.done')}
      </button>
    </div>
  );
};

export default TextBar;
