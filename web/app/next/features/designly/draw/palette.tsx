// The palette of a drawing: the tools, the colours, the two widths, undo and redo, «Очистить» and «Готово».
// The words and the sending are the card's, which opens at «Готово».
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';
import type { IDrawing } from '@/next/features/designly/draw/use-drawing';
import { DRAW_COLORS, DRAW_TOOL, DRAW_WIDTHS, type TDrawTool } from '@/next/features/designly/draw/types';
import { COLOR_NAMES } from '@/next/features/designly/draw/shapes';

const TOOLS: { tool: TDrawTool; icon: string }[] = [
  { tool: DRAW_TOOL.pen, icon: 'pencil' },
  { tool: DRAW_TOOL.marker, icon: 'highlighter' },
  { tool: DRAW_TOOL.arrow, icon: 'arrow-up-right' },
  { tool: DRAW_TOOL.rect, icon: 'square' },
  { tool: DRAW_TOOL.label, icon: 'type' },
  { tool: DRAW_TOOL.eraser, icon: 'eraser' },
];

export interface IDrawPaletteProps {
  drawing: IDrawing;
  onDone: () => void;
}

const DrawPalette = ({ drawing, onDone }: IDrawPaletteProps) => (
  <div className="hx-draw-pal" role="toolbar" aria-label={t('next.site.draw.palette')}>
    <div className="hx-draw-pal__group" role="radiogroup" aria-label={t('next.site.draw.tools')}>
      {TOOLS.map(({ tool, icon }) => (
        <button
          key={tool}
          type="button"
          role="radio"
          className="hx-draw-pal__btn hx-tipped"
          aria-checked={drawing.tool === tool}
          aria-label={t(`next.site.draw.tool.${tool}`)}
          onClick={() => drawing.setTool(tool)}
        >
          <Icon name={icon} size="sm" />
          <span className="hx-tip" data-pos="below">
            {t(`next.site.draw.tool.${tool}`)}
          </span>
        </button>
      ))}
    </div>
    <span className="hx-draw-pal__sep" />
    <div className="hx-draw-pal__group" role="radiogroup" aria-label={t('next.site.draw.colors')}>
      {DRAW_COLORS.map((c) => (
        <button
          key={c}
          type="button"
          role="radio"
          className="hx-draw-pal__color"
          data-c={COLOR_NAMES[c]}
          aria-checked={drawing.color === c}
          aria-label={t(`next.site.draw.color.${COLOR_NAMES[c]}`)}
          onClick={() => drawing.setColor(c)}
        />
      ))}
    </div>
    <span className="hx-draw-pal__sep" />
    <div className="hx-draw-pal__group" role="radiogroup" aria-label={t('next.site.draw.widths')}>
      {DRAW_WIDTHS.map((w, i) => (
        <button
          key={w}
          type="button"
          role="radio"
          className="hx-draw-pal__btn"
          data-w={i}
          aria-checked={drawing.width === w}
          aria-label={t(i === 0 ? 'next.site.draw.thin' : 'next.site.draw.thick')}
          onClick={() => drawing.setWidth(w)}
        >
          <i className="hx-draw-pal__dot" />
        </button>
      ))}
    </div>
    <span className="hx-draw-pal__sep" />
    <button
      type="button"
      className="hx-draw-pal__btn hx-tipped"
      aria-label={t('next.site.draw.undo')}
      aria-disabled={!drawing.canUndo}
      onClick={drawing.undo}
    >
      <Icon name="undo-2" size="sm" />
      <span className="hx-tip" data-pos="below">
        {t('next.site.draw.undo')} <kbd>⌘Z</kbd>
      </span>
    </button>
    <button
      type="button"
      className="hx-draw-pal__btn hx-tipped"
      aria-label={t('next.site.draw.redo')}
      aria-disabled={!drawing.canRedo}
      onClick={drawing.redo}
    >
      <Icon name="redo-2" size="sm" />
      <span className="hx-tip" data-pos="below">
        {t('next.site.draw.redo')} <kbd>⇧⌘Z</kbd>
      </span>
    </button>
    <button
      type="button"
      className="hx-draw-pal__btn hx-tipped"
      aria-label={t('next.site.draw.clear')}
      aria-disabled={drawing.shapes.length === 0}
      onClick={drawing.clear}
    >
      <Icon name="trash-2" size="sm" />
      <span className="hx-tip" data-pos="below">
        {t('next.site.draw.clear')}
      </span>
    </button>
    <button
      type="button"
      className="hx-btn hx-draw-pal__done"
      data-size="sm"
      data-variant="primary"
      disabled={drawing.shapes.length === 0}
      onClick={onDone}
    >
      <Icon name="check" size="sm" />
      <span>{t('next.site.draw.done')}</span>
    </button>
  </div>
);

export default DrawPalette;
