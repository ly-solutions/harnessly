// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import { videoKeyAction } from '@/next/features/designly/video/use-video-keys';

describe('video keys', () => {
  it('plays and pauses with Space and K, in either layout', () => {
    expect(videoKeyAction(' ')).toEqual({ key: 'toggle' });
    expect(videoKeyAction('k')).toEqual({ key: 'toggle' });
    expect(videoKeyAction('Л')).toEqual({ key: 'toggle' });
  });

  it('jumps 5 s with the arrows, 10 s with Shift and with J and L', () => {
    expect(videoKeyAction('ArrowLeft')).toEqual({ key: 'back5' });
    expect(videoKeyAction('ArrowRight')).toEqual({ key: 'forward5' });
    expect(videoKeyAction('ArrowLeft', true)).toEqual({ key: 'back10' });
    expect(videoKeyAction('ArrowRight', true)).toEqual({ key: 'forward10' });
    expect(videoKeyAction('j')).toEqual({ key: 'back10' });
    expect(videoKeyAction('д')).toEqual({ key: 'forward10' });
  });

  it('steps a frame with the comma and the period, and their Russian keys', () => {
    expect(videoKeyAction(',')).toEqual({ key: 'frameBack' });
    expect(videoKeyAction('б')).toEqual({ key: 'frameBack' });
    expect(videoKeyAction('.')).toEqual({ key: 'frameForward' });
    expect(videoKeyAction('ю')).toEqual({ key: 'frameForward' });
  });

  it('changes scene and goes to the ends', () => {
    expect(videoKeyAction('PageUp')).toEqual({ key: 'prevScene' });
    expect(videoKeyAction('ArrowDown')).toEqual({ key: 'nextScene' });
    expect(videoKeyAction('Home')).toEqual({ key: 'start' });
    expect(videoKeyAction('End')).toEqual({ key: 'end' });
  });

  it('goes to a tenth with the digits', () => {
    expect(videoKeyAction('0')).toEqual({ key: 'tenth', n: 0 });
    expect(videoKeyAction('7')).toEqual({ key: 'tenth', n: 7 });
  });

  it('marks the range, comments and goes full screen, in either layout', () => {
    expect(videoKeyAction('i')).toEqual({ key: 'markIn' });
    expect(videoKeyAction('ш')).toEqual({ key: 'markIn' });
    expect(videoKeyAction('O')).toEqual({ key: 'markOut' });
    expect(videoKeyAction('щ')).toEqual({ key: 'markOut' });
    expect(videoKeyAction('c')).toEqual({ key: 'comment' });
    expect(videoKeyAction('С')).toEqual({ key: 'comment' });
    expect(videoKeyAction('f')).toEqual({ key: 'full' });
    expect(videoKeyAction('а')).toEqual({ key: 'full' });
  });

  it('does nothing for other keys, and nothing for names that are only object properties', () => {
    expect(videoKeyAction('Enter')).toBeNull();
    expect(videoKeyAction('x')).toBeNull();
    expect(videoKeyAction('toString')).toBeNull();
    expect(videoKeyAction('constructor')).toBeNull();
  });
});
