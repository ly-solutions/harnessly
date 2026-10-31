import { describe, expect, it } from 'vitest';
import {
  readVideoComment,
  readVideoRenderJob,
  readVideoScene,
  readVideoSettings,
  readVideoState,
} from '@/next/features/designly/video/wire';

const comment = {
  id: '01JB0000000000000000000000',
  page: 'p1',
  x: 10,
  y: 20,
  label: 'Заголовок',
  text: 'Позже',
  by: { name: 'Пётр', email: 'p@example.com' },
  at: '2026-10-28T10:00:00Z',
  resolved: false,
  resolvedBy: null,
  resolvedAt: null,
};

describe('readVideoState', () => {
  it('is null for what is not a video, so loading never reads as empty', () => {
    expect(readVideoState(null)).toBeNull();
    expect(readVideoState({ rev: 1 })).toBeNull();
    expect(readVideoState([])).toBeNull();
  });

  it('reads a video whose Go slices came as null as empty lists', () => {
    const st = readVideoState({
      rev: 5,
      settings: { v: 1, formats: ['16:9'], fps: 30, seconds: null, voice: 'none', subs: true },
      deck: null,
      chrome: true,
      ffmpeg: false,
      durMs: 0,
      scenes: null,
      comments: null,
      guestResolved: null,
      renders: null,
      job: null,
    });
    expect(st).not.toBeNull();
    expect(st?.scenes).toEqual([]);
    expect(st?.comments).toEqual([]);
    expect(st?.renders).toEqual([]);
    expect(st?.job).toBeNull();
    expect(st?.chrome).toBe(true);
  });

  it('reads scenes, their moments and keys, and leaves out what is malformed', () => {
    const st = readVideoState({
      rev: 9,
      settings: { formats: ['9:16', '16:9', '4:3', '9:16'], seconds: 30, subs: false },
      scenes: [
        {
          id: 'p1',
          path: 'Сцена 1.dc.html',
          title: 'Сцена 1',
          start: 0,
          dur: 4200,
          auto: true,
          transition: { kind: 'fade', ms: 400 },
          moments: [
            { block: 'Заголовок', tpl: 3, at: 400, out: null, buildIn: 'rise', derived: false },
            { block: 'Битый', at: -1 },
            { block: 'Шаг', tpl: null, at: 1800, out: 7500, buildIn: 'spin', derived: true },
          ],
          words: 6,
          notes: 'Оплата за 2 клика',
          version: 'v1',
          blank: false,
          keys: { '16:9': 'abc', '9:16': '', '4:3': 'x' },
        },
        { title: 'без id' },
      ],
      renders: [
        { format: '16:9', published: true, key: 'abc', currentKey: 'abc', fresh: true, durMs: 4200, at: 'x' },
        { format: '4:3' },
      ],
      job: {
        id: 'j1',
        format: '9:16',
        scene: null,
        draft: true,
        state: 'running',
        progress: 2,
        file: null,
        error: null,
        at: 'x',
      },
    });
    expect(st?.settings.formats).toEqual(['16:9', '9:16']);
    expect(st?.settings.seconds).toBe(30);
    expect(st?.settings.subs).toBe(false);
    expect(st?.scenes).toHaveLength(1);
    expect(st?.scenes[0]?.moments).toEqual([
      { block: 'Заголовок', tpl: 3, at: 400, out: null, buildIn: 'rise', derived: false },
      { block: 'Шаг', tpl: null, at: 1800, out: 7500, buildIn: '', derived: true },
    ]);
    expect(st?.scenes[0]?.keys).toEqual({ '16:9': 'abc' });
    expect(st?.scenes[0]?.transition).toEqual({ kind: 'fade', ms: 400 });
    expect(st?.renders).toHaveLength(1);
    expect(st?.job?.progress).toBe(1);
  });
});

describe('readVideoComment', () => {
  it('reads a comment on a stretch', () => {
    const c = readVideoComment({
      ...comment,
      view: 'video',
      t0: 1200,
      t1: 2400,
      format: '9:16',
      block: 'Заголовок',
      frame: '.media/video/frames/k.jpg',
    });
    expect(c?.video).toEqual({
      t0: 1200,
      t1: 2400,
      format: '9:16',
      block: 'Заголовок',
      frame: '.media/video/frames/k.jpg',
    });
  });

  it('reads a moment (no t1) and a board comment (no video part)', () => {
    expect(readVideoComment({ ...comment, view: 'video', t0: 500, format: '16:9' })?.video).toEqual({
      t0: 500,
      t1: null,
      format: '16:9',
      block: '',
      frame: '',
    });
    expect(readVideoComment(comment)?.video).toBeNull();
    expect(readVideoComment({ ...comment, view: 'video', t0: 500, format: '4:3' })?.video).toBeNull();
    expect(
      readVideoComment({ ...comment, view: 'video', t0: 900, t1: 100, format: '16:9' })?.video?.t1,
    ).toBeNull();
  });
});

describe('the other readers', () => {
  it('reads settings with the formats in their order and a job only when it is one', () => {
    expect(readVideoSettings({ formats: [] }).formats).toEqual(['16:9']);
    expect(readVideoRenderJob({ id: '', format: '16:9' })).toBeNull();
    expect(readVideoRenderJob({ id: 'j', format: '1:1' })).toBeNull();
    expect(readVideoRenderJob({ id: 'j', format: '16:9', state: 'odd' })?.state).toBe('failed');
    expect(readVideoScene({ id: 'p', keys: null })?.keys).toEqual({});
  });
});
