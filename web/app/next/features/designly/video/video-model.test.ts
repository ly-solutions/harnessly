import { describe, expect, it } from 'vitest';
import { scene, videoState } from '@/next/features/designly/video/video-fixture';
import {
  RENDER_BADGE,
  clock,
  formatOf,
  jobActive,
  renderBadge,
  renderOf,
  sceneAt,
  sourceOf,
  targetOf,
} from '@/next/features/designly/video/video-model';

describe('sceneAt', () => {
  const scenes = [scene(1, 0, 4000), scene(2, 4000, 5000)];
  it('finds the scene a time falls in, the next one from its start', () => {
    expect(sceneAt(scenes, 0)?.id).toBe('P1');
    expect(sceneAt(scenes, 3999)?.id).toBe('P1');
    expect(sceneAt(scenes, 4000)?.id).toBe('P2');
  });
  it('gives the last scene the very end and past it', () => {
    expect(sceneAt(scenes, 9000)?.id).toBe('P2');
    expect(sceneAt(scenes, 99999)?.id).toBe('P2');
  });
  it('is null for no scenes', () => {
    expect(sceneAt([], 0)).toBeNull();
  });
});

describe('format and source', () => {
  it('keeps the pick while the video has the format, else the first one', () => {
    const v = videoState();
    expect(formatOf(v, '9:16')).toBe('9:16');
    expect(formatOf(v, null)).toBe('16:9');
    const one = videoState({ settings: { ...v.settings, formats: ['16:9'] } });
    expect(formatOf(one, '9:16')).toBe('16:9');
  });
  it('plays the MP4 only where one is published', () => {
    const v = videoState();
    expect(sourceOf(v, '16:9', 'mp4')).toBe('mp4');
    expect(sourceOf(v, '9:16', 'mp4')).toBe('live');
    expect(sourceOf(v, '16:9', 'live')).toBe('live');
  });
});

describe('render badge', () => {
  it('is none without a published file, current on the same key', () => {
    const v = videoState();
    expect(renderBadge(renderOf(v, '9:16'))).toBe(RENDER_BADGE.none);
    expect(renderBadge(renderOf(v, '16:9'))).toBe(RENDER_BADGE.current);
    expect(renderBadge(null)).toBe(RENDER_BADGE.none);
  });
  it('goes outdated when the current key moves on', () => {
    const v = videoState();
    const info = renderOf(v, '16:9');
    expect(info).not.toBeNull();
    if (info === null) return;
    expect(renderBadge({ ...info, currentKey: 'k9' })).toBe(RENDER_BADGE.outdated);
  });
});

describe('jobActive', () => {
  const job = {
    id: 'j',
    format: '16:9' as const,
    scene: null,
    draft: false,
    progress: 0.2,
    file: null,
    error: null,
    at: '',
  };
  it('is true for queued and running, false for the rest', () => {
    expect(jobActive({ ...job, state: 'queued' })).toBe(true);
    expect(jobActive({ ...job, state: 'running' })).toBe(true);
    expect(jobActive({ ...job, state: 'done' })).toBe(false);
    expect(jobActive({ ...job, state: 'failed' })).toBe(false);
    expect(jobActive(null)).toBe(false);
  });
});

describe('targetOf', () => {
  const v = videoState();
  it('is a moment on the scene under the playhead, relative to the scene', () => {
    expect(targetOf(v, '16:9', 5500, null, null)).toEqual({
      page: 'P2',
      t0: 1500,
      t1: null,
      format: '16:9',
      pick: null,
    });
  });
  it('is a stretch for a range, cut at the end of its scene', () => {
    const t = targetOf(v, '9:16', 3000, { t0: 3000, t1: 6000 }, null);
    expect(t).toMatchObject({ page: 'P1', t0: 3000, t1: 4000 });
  });
  it('keeps a pick on its scene', () => {
    const pick = { page: 'P2', block: 'title', x: 10, y: 20 };
    expect(targetOf(v, '16:9', 1000, null, pick)).toMatchObject({ page: 'P2', t0: 0, pick });
  });
  it('is null for no scenes', () => {
    expect(targetOf(videoState({ scenes: [] }), '16:9', 0, null, null)).toBeNull();
  });
});

describe('clock', () => {
  it('writes minutes and seconds', () => {
    expect(clock(0)).toBe('0:00');
    expect(clock(12999)).toBe('0:12');
    expect(clock(61000)).toBe('1:01');
  });
});
