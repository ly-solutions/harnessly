// @vitest-environment jsdom
import { cleanup, fireEvent, render } from '@testing-library/react';
import { createRef } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import BoardFrames from './board-frames';
afterEach(cleanup);
it('warms after load, preserves the active document on return, and drops a previous revision', () => {
  const frameRef = createRef<HTMLIFrameElement>(),
    ready = vi.fn();
  const board = (src: string, nextFrame: string | null, revision = 1) => (
    <BoardFrames key={revision} {...{ src, nextFrame, frameRef, ready }} capacity={3} title="Board" />
  );
  const { rerender, container } = render(board('/a', '/b'));
  const a = frameRef.current!;
  expect(container.querySelectorAll('iframe')).toHaveLength(1);
  fireEvent.load(a);
  const b = container.querySelector<HTMLIFrameElement>('[src="/b"]')!;
  fireEvent.load(b);
  expect(ready).toHaveBeenCalledTimes(1);
  rerender(board('/b', '/c'));
  expect(frameRef.current).toBe(b);
  rerender(board('/a', '/b'));
  expect(frameRef.current).toBe(a);
  expect(container.querySelectorAll('iframe')).toHaveLength(3);
  rerender(board('/a', null, 2));
  expect(frameRef.current).not.toBe(a);
  expect(container.querySelectorAll('iframe')).toHaveLength(1);
});
