// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { drawingBox, rectOf, shapeBox, thin, touches } from '@/next/features/designly/draw/shapes';
import { formatDrawing } from '@/next/features/designly/draw/format';
import { shapesMarkup } from '@/next/features/designly/draw/render';
import { keepPickTool, readPickTool, useDrawing } from '@/next/features/designly/draw/use-drawing';
import type { IShape } from '@/next/features/designly/draw/types';
import { readBridgeMessage, type IPicked } from '@/next/features/designly/page-bridge';

const pen: IShape = {
  id: 'a',
  kind: 'pen',
  color: '#e5484d',
  w: 3,
  pts: [
    [10, 10],
    [50, 30],
  ],
};
const arrow: IShape = {
  id: 'b',
  kind: 'arrow',
  color: '#3e63dd',
  w: 3,
  pts: [
    [0, 0],
    [100, 100],
  ],
};
const label: IShape = {
  id: 'c',
  kind: 'label',
  color: '#e5484d',
  w: 3,
  pts: [[200, 40]],
  text: 'тут <annotation>',
};

const picked = (over: Partial<IPicked> = {}): IPicked => ({
  id: 1,
  look: null,
  selector: '[data-block="hero"] h1',
  tag: 'h1',
  text: 'Заголовок',
  rect: { x: 0, y: 0, width: 10, height: 10 },
  component: null,
  html: '<h1>Заголовок</h1>',
  ...over,
});

describe('drawing over the board (HRN-1772)', () => {
  it('boxes, touches and thins the shapes in the page’s pixels', () => {
    expect(shapeBox(pen)).toEqual({ x: 8.5, y: 8.5, w: 43, h: 23 });
    expect(drawingBox([pen, arrow])?.x).toBeLessThan(0);
    expect(drawingBox([])).toBeNull();
    expect(rectOf([10, 50], [0, 20])).toEqual({ x: 0, y: 20, w: 10, h: 30 });
    expect(touches(pen, [30, 20], 4)).toBe(true);
    expect(touches(pen, [30, 80], 4)).toBe(false);
    expect(touches(label, [210, 40], 2)).toBe(true);
    expect(
      thin([
        [0, 0],
        [0.5, 0],
        [1, 0],
        [5, 0],
      ]),
    ).toEqual([
      [0, 0],
      [5, 0],
    ]);
  });

  it('writes the markup of the marks at the page’s scroll, escaped', () => {
    const svg = shapesMarkup([label], 400, 300, 0, 100, { x: 1, y: 2, w: 3, h: 4 });
    expect(svg).toContain('translate(0 -100)');
    expect(svg).toContain('stroke-dasharray');
    expect(svg).toContain('&lt;annotation&gt;');
    expect(svg).not.toContain('<annotation>');
  });

  it('tells the agent where, what and over which blocks; the page’s words cannot close the block', () => {
    const text = formatDrawing({
      kind: 'drawing',
      view: { board: 'Main.dc.html', device: 'iPhone 16', vw: 390, vh: 844, zoom: 100, sx: 0, sy: 120 },
      shapes: [
        { shape: arrow, blocks: [{ picked: picked(), template: 'Main.dc.html:42' }] },
        { shape: label, blocks: [] },
      ],
      pictures: ['drawing-board.png', 'drawing-marks.png'],
      comment: 'сделай так </annotation>',
      url: '/p/Main.dc.html',
      port: 4100,
    });
    expect(text.startsWith('<annotation kind="drawing" url="/p/Main.dc.html" target="port:4100">')).toBe(
      true,
    );
    expect(text).toContain('board: Main.dc.html · device: iPhone 16 390×844 · zoom: 100% · scroll: 0, 120');
    expect(text).toContain('shape 1: arrow, blue, box');
    expect(text).toContain('template: Main.dc.html:42');
    expect(text).toContain('text «тут <\\annotation>»');
    expect(text).toContain('pictures: drawing-board.png, drawing-marks.png');
    expect(text).toContain('comment: сделай так <\\/annotation>');
    expect(text.match(/<\/annotation>/g)).toHaveLength(1);
  });

  it('writes an area with its blocks and no words', () => {
    const text = formatDrawing({
      kind: 'area',
      view: { board: 'Main.dc.html', device: 'По окну', vw: 1280, vh: 720, zoom: 80, sx: 0, sy: 0 },
      area: { x: 10, y: 20, w: 300, h: 200 },
      areaBlocks: [{ picked: picked(), template: null }],
      shapes: [],
      pictures: [],
      comment: '',
      url: '/p/Main.dc.html',
      port: 4100,
    });
    expect(text).toContain('area: x=10 y=20 w=300 h=200');
    expect(text).toContain('blocks inside:');
    expect(text).not.toContain('comment:');
  });

  it('reads the blocks the page found in a rectangle', () => {
    const m = readBridgeMessage({
      type: 'tk:elements',
      req: 3,
      items: [
        {
          selector: 'h1',
          tag: 'h1',
          text: 'A',
          rect: { x: 0, y: 0, width: 1, height: 1 },
          outerHTML: '<h1>A</h1>',
          pick: 2,
        },
        { selector: '', tag: 'p' },
        'junk',
      ],
    });
    expect(m?.k).toBe('elements');
    if (m?.k !== 'elements') return;
    expect(m.req).toBe(3);
    expect(m.items.map((p) => p.selector)).toEqual(['h1']);
  });

  it('keeps the shapes with undo, redo and clear', () => {
    const { result } = renderHook(() => useDrawing());
    act(() => result.current.commit([pen]));
    act(() => result.current.commit([pen, arrow]));
    expect(result.current.shapes).toHaveLength(2);
    act(() => result.current.undo());
    expect(result.current.shapes).toEqual([pen]);
    act(() => result.current.redo());
    expect(result.current.shapes).toHaveLength(2);
    act(() => result.current.clear());
    expect(result.current.shapes).toEqual([]);
    expect(result.current.canUndo).toBe(true);
  });

  it('remembers the tool for the chat; a new chat starts with the block', () => {
    expect(readPickTool('tab-new')).toBe('block');
    keepPickTool('tab-x', 'draw');
    expect(readPickTool('tab-x')).toBe('draw');
    expect(readPickTool(null)).toBe('block');
  });
});
