import { describe, expect, it } from 'vitest';
import { heldTools, quietTools, toolOfRequest } from '@/next/features/pane/tools-order';
import type { ITool } from '@/next/features/pane/tools-launcher';

const tool = (id: string, over: Partial<ITool> = {}): ITool => ({
  id,
  group: 'chat',
  icon: 'x',
  name: id,
  hint: '',
  ...over,
});
const TOOLS = [tool('plan'), tool('subagents'), tool('files'), tool('terminal', { group: 'vm' })];

describe('quietTools', () => {
  it('keeps every tile full in a chat that used nothing, the agent terminal not counting', () => {
    expect(quietTools(TOOLS, new Set()).size).toBe(0);
    expect(quietTools(TOOLS, new Set(['terminal'])).size).toBe(0);
  });

  it('quiets the unused once any was used, never the terminal', () => {
    expect([...quietTools(TOOLS, new Set(['subagents']))].sort()).toEqual(['files', 'plan']);
  });

  it('does not quiet a tool that shows something live', () => {
    const tools = [tool('plan'), tool('shell', { live: '2 open' })];
    expect(quietTools(tools, new Set(['plan'])).size).toBe(0);
  });

  it('ignores used ids the pane does not have', () => {
    expect(quietTools(TOOLS, new Set(['L:vault'])).size).toBe(0);
  });
});

describe('heldTools', () => {
  it('names the tools the chat has content for', () => {
    const held = heldTools({
      subagents: true,
      side: false,
      tabKinds: ['shell', 'plan'],
      pictures: true,
      documents: true,
      designs: false,
    });
    expect([...held].sort()).toEqual(['L:artifacts', 'L:gallery', 'shell', 'subagents']);
  });
});

describe('toolOfRequest', () => {
  it('maps a request to its tool', () => {
    expect(toolOfRequest({ k: 'design', target: null, reload: false })).toBe('designly');
    expect(toolOfRequest({ k: 'legacy', kind: 'gallery', target: null, reload: false })).toBe('L:gallery');
    expect(toolOfRequest({ k: 'subagent', reload: false })).toBe('subagents');
    expect(toolOfRequest({ k: 'show', reload: false })).toBeNull();
  });
});
