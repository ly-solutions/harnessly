// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/react';
import CanvasMenu from '@/next/features/designly/canvas-menu';

describe('the page’s right-click menu (HRN-1764)', () => {
  it('opens with no item lit; the arrows light them in turn', () => {
    const parts = [
      [
        { id: 'change', icon: 'x', label: 'Изменить блок' },
        { id: 'ask', icon: 'x', label: 'Спросить агента' },
      ],
    ];
    const { getByRole, getAllByRole } = render(
      <CanvasMenu x={10} y={10} state="open" label="" parts={parts} onPick={vi.fn()} onClose={vi.fn()} />,
    );
    const menu = getByRole('menu');
    const items = getAllByRole('menuitem');
    expect(document.activeElement).toBe(menu);
    fireEvent.keyDown(menu, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(items[0]);
    fireEvent.keyDown(menu, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(items[1]);
    fireEvent.keyDown(menu, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(items[0]);
  });
});
