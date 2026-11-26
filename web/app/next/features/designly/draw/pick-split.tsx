// The bar's pick button with its tools: «Выбрать блок» (V), «Выделить область» (R), «Рисовать» (D). The
// button shows the tool chosen and turns it on and off; the chevron opens the list with the keys. The choice is kept
// for the chat.
import { useEffect, useRef, useState } from 'react';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';
import '@/next/styles/draw.css';
import { PICK_TOOL, type TPickTool } from '@/next/features/designly/draw/types';

export const PICK_TOOL_ICON: Record<TPickTool, string> = {
  block: 'square-mouse-pointer',
  area: 'square-dashed',
  draw: 'pencil',
};
const PICK_TOOL_KEY: Record<TPickTool, string> = { block: 'V', area: 'R', draw: 'D' };

export interface IPickSplitProps {
  tool: TPickTool;
  on: boolean;
  disabled: boolean;
  /** The tool's own words where a guest's are other («Комментировать»). */
  blockLabel?: string;
  blockTip?: string;
  keys: boolean;
  /** The block tool's key as the person set it. */
  blockKey?: string;
  onToggle: () => void;
  onTool: (tool: TPickTool) => void;
}

const PickSplit = ({
  tool,
  on,
  disabled,
  blockLabel,
  blockTip,
  keys,
  blockKey,
  onToggle,
  onTool,
}: IPickSplitProps) => {
  const keyOf = (x: TPickTool) => (x === PICK_TOOL.block && blockKey ? blockKey : PICK_TOOL_KEY[x]);
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: PointerEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    window.addEventListener('pointerdown', away);
    window.addEventListener('keydown', key);
    return () => {
      window.removeEventListener('pointerdown', away);
      window.removeEventListener('keydown', key);
    };
  }, [open]);
  const label = (x: TPickTool) =>
    x === PICK_TOOL.block && blockLabel ? blockLabel : t(`next.site.tool.${x}`);
  return (
    <div ref={box} className="hx-pick-split" data-on={on || undefined}>
      <button
        type="button"
        className="hx-head-btn hx-tipped hx-browser__pick"
        aria-pressed={on}
        aria-disabled={disabled}
        aria-label={label(tool)}
        onClick={() => !disabled && onToggle()}
      >
        <Icon name={PICK_TOOL_ICON[tool]} />
        <span className="hx-tip" data-pos="below">
          {tool === PICK_TOOL.block && blockTip ? blockTip : t(`next.site.tool.${tool}Tip`)}
          {keys && <kbd>{keyOf(tool)}</kbd>}
        </span>
      </button>
      <button
        type="button"
        className="hx-pick-split__chev"
        aria-label={t('next.site.tool.choose')}
        aria-haspopup="menu"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name="chevron-down" size="sm" />
      </button>
      {open && (
        <div className="hx-site-menu hx-pick-split__menu" role="menu" data-state="open">
          {(Object.values(PICK_TOOL) as TPickTool[]).map((x) => (
            <button
              key={x}
              type="button"
              role="menuitemradio"
              aria-checked={x === tool}
              className="hx-option hx-more__item"
              onClick={() => {
                setOpen(false);
                onTool(x);
              }}
            >
              <Icon name={PICK_TOOL_ICON[x]} size="sm" />
              <span className="hx-more__label">{label(x)}</span>
              {keys && <kbd className="hx-more__kbd">{keyOf(x)}</kbd>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
};

export default PickSplit;
