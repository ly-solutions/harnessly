// Which tools of the tools home a chat has used, and so which stay full tiles and which sit quietly in a row of chips.
// Pure: the signals come from the pane (what was opened, what the chat holds), the layout from here.
import type { ITool } from '@/next/features/pane/tools-launcher';
import type { IPaneRequest } from '@/next/features/pane/intents';

/** The agent's own terminal: always there, so it neither counts as used nor goes quiet. */
const ALWAYS_FULL: ReadonlySet<string> = new Set(['terminal']);

/**
 * The tools that go quiet: those a chat did not use, once it used any. A chat that used nothing keeps every tile
 * full. `used` is what the chat opened plus what it holds (subagents, made documents, pictures).
 */
export const quietTools = (tools: readonly ITool[], used: ReadonlySet<string>): ReadonlySet<string> => {
  const any = tools.some((x) => used.has(x.id) && !ALWAYS_FULL.has(x.id));
  if (!any) return new Set();
  return new Set(tools.filter((x) => !used.has(x.id) && !ALWAYS_FULL.has(x.id) && !x.live).map((x) => x.id));
};

/** Every tool that holds something for the chat now, off the data the pane already has. */
export interface IToolHolds {
  subagents: boolean;
  side: boolean;
  /** Kinds of the tabs open in the pane. */
  tabKinds: readonly string[];
  pictures: boolean;
  documents: boolean;
  designs: boolean;
}

/** The ids of the tools the chat holds content for. */
export const heldTools = (h: IToolHolds): Set<string> => {
  const held = new Set<string>();
  if (h.subagents) held.add('subagents');
  if (h.side) held.add('side');
  if (h.tabKinds.includes('shell')) held.add('shell');
  if (h.tabKinds.includes('browser')) held.add('browser');
  if (h.pictures) held.add('L:gallery');
  if (h.documents) held.add('L:artifacts');
  if (h.designs) held.add('designly');
  return held;
};

/** The tool a request to the pane opens, when it names one. */
export const toolOfRequest = (request: IPaneRequest): string | null => {
  switch (request.k) {
    case 'design':
      return 'designly';
    case 'subagent':
      return 'subagents';
    case 'files':
    case 'plan':
    case 'side':
    case 'browser':
      return request.k;
    case 'legacy':
      return `L:${request.kind}`;
    default:
      return null;
  }
};
