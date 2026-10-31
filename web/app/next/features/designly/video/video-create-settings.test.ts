import { describe, expect, it } from 'vitest';
import {
  DEFAULT_VIDEO_SETTINGS,
  VIDEO_FORMATS_PICK,
  toWireVideo,
} from '@/next/features/designly/video/video-create-settings';
import { DESIGN_VIDEO_FORMATS } from '@/next/protocol/v2';

describe('toWireVideo', () => {
  it('sends the defaults as one landscape video of free length with subtitles', () => {
    expect(toWireVideo(DEFAULT_VIDEO_SETTINGS)).toEqual({ formats: ['16:9'], seconds: null, subs: true });
  });
  it('sends both formats in the fixed order, no repeats', () => {
    const wire = toWireVideo({ ...DEFAULT_VIDEO_SETTINGS, formats: VIDEO_FORMATS_PICK.both, seconds: 30 });
    expect(wire.formats).toEqual(['16:9', '9:16']);
    expect(new Set(wire.formats).size).toBe(wire.formats.length);
  });
  it('only sends formats the schema knows', () => {
    for (const pick of Object.values(VIDEO_FORMATS_PICK)) {
      for (const f of toWireVideo({ ...DEFAULT_VIDEO_SETTINGS, formats: pick }).formats) {
        expect(DESIGN_VIDEO_FORMATS).toContain(f);
      }
    }
  });
  it('keeps the seconds and the subtitles off', () => {
    expect(toWireVideo({ formats: 'portrait', seconds: 180, subs: false })).toEqual({
      formats: ['9:16'],
      seconds: 180,
      subs: false,
    });
  });
});
