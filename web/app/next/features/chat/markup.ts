// The service blocks an agent's harness writes into the text of a person's message besides their words: reminders of the
// system, a background task's report, a slash command, a `!` command and its output, a teammate's message, the file or
// lines open in the IDE. The bubble never shows them as raw tags: each becomes a part of its own that the chat draws as a
// card (features/chat/markup-block.tsx). `<attached>`, `<annotation>` and `<pasted_content>` have readers of their own.

export const MARKUP = {
  reminder: 'reminder',
  task: 'task',
  command: 'command',
  shell: 'shell',
  output: 'output',
  teammate: 'teammate',
  hook: 'hook',
  ide: 'ide',
} as const;
export type TMarkup = (typeof MARKUP)[keyof typeof MARKUP];

export const TASK_STATE = { done: 'done', failed: 'failed', running: 'running' } as const;
export type TTaskState = (typeof TASK_STATE)[keyof typeof TASK_STATE];

export type TMarkupBlock =
  | { kind: typeof MARKUP.reminder; body: string }
  | {
      kind: typeof MARKUP.task;
      state: TTaskState;
      status: string;
      summary: string;
      output: string;
      result: string;
    }
  | { kind: typeof MARKUP.command; name: string; args: string }
  | { kind: typeof MARKUP.shell; command: string }
  | { kind: typeof MARKUP.output; body: string; error: boolean }
  | { kind: typeof MARKUP.teammate; from: string; summary: string; body: string }
  | { kind: typeof MARKUP.hook; body: string }
  | { kind: typeof MARKUP.ide; body: string; selection: boolean };

export type TMarkupPart =
  { key: string; text: string; block: null } | { key: string; text: null; block: TMarkupBlock };

const TAGS = [
  'system-reminder',
  'task-notification',
  'command-name',
  'command-message',
  'command-args',
  'local-command-caveat',
  'local-command-stdout',
  'local-command-stderr',
  'bash-input',
  'bash-stdout',
  'bash-stderr',
  'teammate-message',
  'user-prompt-submit-hook',
  'ide_selection',
  'ide_opened_file',
] as const;
type TTag = (typeof TAGS)[number];

/** One block of a known tag, with its attributes and its body; a tag of any other name stays the person's text. */
const BLOCK_RE = new RegExp(`<(${TAGS.join('|')})\\b([^>]*)>([\\s\\S]*?)</\\1>`, 'g');
const ATTR_RE = /([a-z_][a-z0-9_-]*)="([^"]*)"/gi;

const fieldOf = (body: string, name: string): string =>
  new RegExp(`<${name}>([\\s\\S]*?)</${name}>`).exec(body)?.[1].trim() ?? '';

const attrsOf = (raw: string): Record<string, string> =>
  Object.fromEntries([...raw.matchAll(ATTR_RE)].map((m) => [m[1], m[2]]));

const TASK_STATE_OF: Record<string, TTaskState> = {
  completed: TASK_STATE.done,
  done: TASK_STATE.done,
  success: TASK_STATE.done,
  failed: TASK_STATE.failed,
  error: TASK_STATE.failed,
  killed: TASK_STATE.failed,
  stopped: TASK_STATE.failed,
};

/** The block a tag makes, or null for a tag that says nothing to the person (the CLI's caveat, a command's echo). */
const blockOf = (tag: TTag, rawAttrs: string, body: string): TMarkupBlock | null => {
  const text = body.trim();
  switch (tag) {
    case 'system-reminder':
      return text ? { kind: MARKUP.reminder, body: text } : null;
    case 'task-notification': {
      const status = fieldOf(body, 'status');
      return {
        kind: MARKUP.task,
        state: TASK_STATE_OF[status.toLowerCase()] ?? TASK_STATE.running,
        status,
        summary: fieldOf(body, 'summary') || text,
        output: fieldOf(body, 'output-file'),
        result: fieldOf(body, 'result'),
      };
    }
    case 'command-name':
      return { kind: MARKUP.command, name: text.replace(/^\/?/, '/'), args: '' };
    case 'command-message':
    case 'local-command-caveat':
      return null;
    case 'command-args':
      return text ? { kind: MARKUP.command, name: '', args: text } : null;
    case 'local-command-stdout':
    case 'bash-stdout':
      return text ? { kind: MARKUP.output, body: text, error: false } : null;
    case 'local-command-stderr':
    case 'bash-stderr':
      return text ? { kind: MARKUP.output, body: text, error: true } : null;
    case 'bash-input':
      return { kind: MARKUP.shell, command: text };
    case 'teammate-message': {
      const attrs = attrsOf(rawAttrs);
      return {
        kind: MARKUP.teammate,
        from: attrs.teammate_id ?? attrs.from ?? '',
        summary: attrs.summary ?? '',
        body: text,
      };
    }
    case 'user-prompt-submit-hook':
      return text ? { kind: MARKUP.hook, body: text } : null;
    case 'ide_selection':
    case 'ide_opened_file':
      return { kind: MARKUP.ide, body: text, selection: tag === 'ide_selection' };
  }
};

/** A command's name and its arguments come as two tags: the parts keep them as one block. */
const joined = (parts: TMarkupPart[]): TMarkupPart[] =>
  parts.reduce<TMarkupPart[]>((out, part) => {
    const last = out.at(-1);
    if (
      part.block?.kind === MARKUP.command &&
      last?.block?.kind === MARKUP.command &&
      (last.block.args === '' || part.block.name === '')
    ) {
      out[out.length - 1] = {
        key: last.key,
        text: null,
        block: {
          kind: MARKUP.command,
          name: last.block.name || part.block.name,
          args: last.block.args || part.block.args,
        },
      };
      return out;
    }
    out.push(part);
    return out;
  }, []);

/** The text split into the person's words and the service blocks among them, in order; a text without any is one part. */
export const splitMarkup = (text: string): TMarkupPart[] => {
  const parts: TMarkupPart[] = [];
  let at = 0;
  for (const m of text.matchAll(BLOCK_RE)) {
    const index = m.index ?? 0;
    const before = text.slice(at, index).trim();
    if (before) parts.push({ key: `t${at}`, text: before, block: null });
    const block = blockOf(m[1] as TTag, m[2], m[3]);
    if (block) parts.push({ key: `b${index}`, text: null, block });
    at = index + m[0].length;
  }
  const rest = text.slice(at).trim();
  if (rest) parts.push({ key: `t${at}`, text: rest, block: null });
  return joined(parts);
};

/** «[Image: source: /path/x.png]», as Claude Code names a picture of the message: the path alone, which the bubble
 *  turns into the picture itself (features/chat/pictures). */
export const IMAGE_SOURCE = /\[Image: source: ([^\]\n]+)\]/g;
export const withImagePaths = (text: string): string =>
  text.replace(IMAGE_SOURCE, (_, path: string) => path.trim());
