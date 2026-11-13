// What an error text says, taken apart once for every place that shows one: the «Turn interrupted» row, the agent's
// error notice in the feed and a failed MCP tool's card. The text is whatever the agent, its model's API or anything
// on the way between them printed: JSON inside JSON inside a string, a blocking HTML page, a chain of «x failed:
// details - …», or plain words. Out comes the kind (it picks the human headline), the deepest message as one line,
// and the status code; the raw text stays whole for «Details». Text of an unknown shape is cut short, never thrown.
export const ERROR_KIND = {
  /** The agent's sign-in is gone: expired, revoked, never made. Signing in again is the cure. */
  login: 'login',
  /** A security filter on the way to the model refused the request (a tool it does not allow). */
  guard: 'guard',
  /** A network shield (Cloudflare, a proxy's block page) did not let the request through. */
  blocked: 'blocked',
  rateLimit: 'rate-limit',
  overloaded: 'overloaded',
  server: 'server',
  network: 'network',
  /** The model refused the request itself (too long, malformed). */
  request: 'request',
  /** 403 that is neither a filter nor a sign-in. */
  denied: 'denied',
  unknown: 'unknown',
} as const;
export type TErrorKind = (typeof ERROR_KIND)[keyof typeof ERROR_KIND];

export interface ParsedError {
  kind: TErrorKind;
  /** The HTTP status the text names; null when it names none. */
  status: number | null;
  /** The deepest message, one line, cut to REASON_MAX; null when the text says nothing beyond its kind. */
  reason: string | null;
  /** The tools a security filter named, for `guard`. */
  tools: string[];
  raw: string;
}

export const REASON_MAX = 160;
/** A text longer than this is not searched whole: an error says what it is at its start. */
const SCAN_MAX = 4_000;
const DEPTH_MAX = 6;
const BRACES_MAX = 8;

const LOGIN_HINT = 'please run /login';
const LOGIN_WORDS =
  /login expired|not logged in|oauth token (has )?(been )?(expired|revoked)|authentication_error|invalid (x-)?api[- ]key|invalid bearer token|(sign|log) ?in again|logged out|token could not be refreshed/i;
const GUARD_WORDS = /guardrail|malicious|invalid tools?:|security (filter|policy)|blocked by policy/i;
const BLOCK_WORDS = /cloudflare|attention required|just a moment\b|captcha|ddos protection/i;
const HTML_WORDS = /<!doctype html|<html[\s>]|<head[\s>]|<title[\s>]|<body[\s>]/i;
const NETWORK_WORDS =
  /econn\w+|etimedout|enotfound|eai_again|fetch failed|connection error|socket hang up|timed? ?out|network error|\btls\b|\bcertificate\b/i;
const MESSAGE_KEYS = ['message', 'error_description', 'detail', 'details', 'msg', 'reason', 'error'] as const;

/** The text without the agent's «Please run /login» and the «·» that joins it to the rest. Plain string search: the
 *  text is whatever a tool or a proxy printed, and a pattern with open-ended spaces around the phrase could be made
 *  to backtrack for seconds. */
const withoutLoginHint = (text: string): string => {
  const parts: string[] = [];
  const lower = text.toLowerCase();
  let from = 0;
  for (let at = lower.indexOf(LOGIN_HINT); at !== -1; at = lower.indexOf(LOGIN_HINT, from)) {
    parts.push(text.slice(from, at));
    from = at + LOGIN_HINT.length;
  }
  parts.push(text.slice(from));
  return parts
    .map((part) =>
      part
        .trim()
        .replace(/^[.·]+/, '')
        .replace(/·+$/, '')
        .trim(),
    )
    .filter((part) => part !== '')
    .join(' ');
};

const oneLine = (text: string): string => text.replace(/\s+/g, ' ').trim();

export const clip = (text: string, max: number = REASON_MAX): string => {
  const line = oneLine(text);
  return line.length > max ? `${line.slice(0, max - 1).trimEnd()}…` : line;
};

const positions = (text: string, char: string, fromEnd: boolean): number[] => {
  const found: number[] = [];
  let at = fromEnd ? text.lastIndexOf(char) : text.indexOf(char);
  while (at !== -1 && found.length < BRACES_MAX) {
    found.push(at);
    if (fromEnd && at === 0) break;
    at = fromEnd ? text.lastIndexOf(char, at - 1) : text.indexOf(char, at + 1);
  }
  return found;
};

const parseObject = (text: string): Record<string, unknown> | null => {
  try {
    const value: unknown = JSON.parse(text);
    return typeof value === 'object' && value !== null && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
};

/** The outermost JSON object written inside the text: the widest span of braces that parses. */
const objectIn = (text: string): Record<string, unknown> | null => {
  const ends = positions(text, '}', true);
  for (const start of positions(text, '{', false)) {
    for (const end of ends) {
      if (end <= start) break;
      const found = parseObject(text.slice(start, end + 1));
      if (found) return found;
    }
  }
  return null;
};

const unescaped = (text: string): string =>
  text
    .replace(/\\+n/g, ' ')
    .replace(/\\+t/g, ' ')
    .replace(/\\+(["'/])/g, '$1');

interface IMessage {
  text: string;
  type: string | null;
}

/** The message a JSON error carries: `error` as words or as an object with `message`, or one of the usual keys. */
const messageOf = (object: Record<string, unknown>): IMessage | null => {
  const type = typeof object.type === 'string' && object.type !== 'error' ? object.type : null;
  for (const key of MESSAGE_KEYS) {
    const value = object[key];
    if (typeof value === 'string' && value.trim() !== '') return { text: value, type };
    if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
      const inner = messageOf(value as Record<string, unknown>);
      if (inner) return { text: inner.text, type: inner.type ?? type };
    }
  }
  return null;
};

/** JSON cut off or badly quoted does not parse: the last `"message": "…"` or `"error": "…"` in it is the deepest. */
const lastQuotedMessage = (text: string): string | null => {
  const pattern = /"(?:message|error|error_description|detail)"\s*:\s*"([^"]*)/g;
  let last: string | null = null;
  for (const match of unescaped(text).matchAll(pattern)) {
    if (match[1].trim() !== '') last = match[1];
  }
  return last;
};

const htmlTitle = (html: string): string | null => {
  const found = /<title[^>]*>([^<]*)/i.exec(html) ?? /<h1[^>]*>([^<]*)/i.exec(html);
  const title = found ? oneLine(found[1]) : '';
  return title === '' ? null : title;
};

const stripTags = (html: string): string =>
  html
    .replace(/<(script|style)[\s\S]*?(<\/\1>|$)/gi, ' ')
    .replace(/<[^>]*>?/g, ' ')
    .replace(/&[a-z#0-9]+;/gi, ' ');

interface IDeepest {
  text: string;
  types: string[];
  html: boolean;
}

/** Walks down: text → the JSON in it → its message → the JSON in that, until plain words (or a page) are left. */
const deepest = (start: string): IDeepest => {
  let text = start;
  const types: string[] = [];
  for (let depth = 0; depth < DEPTH_MAX; depth += 1) {
    const object = objectIn(text) ?? (text.includes('\\"') ? objectIn(unescaped(text)) : null);
    const message = object ? messageOf(object) : null;
    if (message === null) break;
    if (message.type) types.push(message.type);
    text = message.text;
  }
  if (HTML_WORDS.test(text)) return { text: htmlTitle(text) ?? stripTags(text), types, html: true };
  if (text.includes('{')) {
    const quoted = lastQuotedMessage(text);
    if (quoted !== null) text = quoted;
  }
  // «a failed: b failed: details - the words»: the words after the last «details -».
  const details = text.split(/\bdetails?\s+-\s+/i).at(-1) ?? text;
  return { text: details.trim() === '' ? text : details, types, html: false };
};

const statusOf = (text: string): number | null => {
  const found =
    /\bapi error:?\s*\(?(\d{3})\b/i.exec(text) ??
    /\\*"status(?:_?code)?\\*"\s*:\s*(\d{3})\b/i.exec(text) ??
    /\b(?:http|status(?: code)?)[ :=]+(\d{3})\b/i.exec(text) ??
    /^\s*(\d{3})\b/.exec(text);
  if (!found) return null;
  const status = Number(found[1]);
  return status >= 400 && status <= 599 ? status : null;
};

const toolsOf = (text: string): string[] => {
  const found = /invalid tools?:\s*([\w\-.:]+(?:\s*,\s*[\w\-.:]+)*)/i.exec(text);
  return found ? found[1].split(',').map((tool) => tool.trim().replace(/\.+$/, '')) : [];
};

interface IFacts {
  status: number | null;
  text: string;
  types: string[];
  html: boolean;
  hinted: boolean;
}

const kindOf = ({ status, text, types, html, hinted }: IFacts): TErrorKind => {
  const has = (type: string) => types.includes(type);
  if (GUARD_WORDS.test(text)) return ERROR_KIND.guard;
  if (html || BLOCK_WORDS.test(text)) return ERROR_KIND.blocked;
  if (status === 401 || LOGIN_WORDS.test(text) || has('authentication_error')) return ERROR_KIND.login;
  // Claude asks for a login on any 403; only one the API itself signed as a refusal of this account is about it.
  if (status === 403 && has('permission_error')) return ERROR_KIND.login;
  if (status === 429 || has('rate_limit_error')) return ERROR_KIND.rateLimit;
  if (status === 529 || has('overloaded_error') || /\boverloaded\b/i.test(text)) return ERROR_KIND.overloaded;
  if ((status !== null && status >= 500) || has('api_error')) return ERROR_KIND.server;
  if (NETWORK_WORDS.test(text)) return ERROR_KIND.network;
  if (status === 403) return ERROR_KIND.denied;
  if (status !== null || has('invalid_request_error')) return ERROR_KIND.request;
  // «Please run /login» alone, with nothing that says otherwise, is the agent's word for a lost sign-in.
  return hinted ? ERROR_KIND.login : ERROR_KIND.unknown;
};

const EMPTY: ParsedError = { kind: ERROR_KIND.unknown, status: null, reason: null, tools: [], raw: '' };

export const parseErrorText = (input: string | null | undefined): ParsedError => {
  const raw = (input ?? '').trim();
  if (raw === '') return EMPTY;
  const scanned = raw.slice(0, SCAN_MAX);
  const hinted = scanned.toLowerCase().includes(LOGIN_HINT);
  const body = hinted ? withoutLoginHint(scanned) : scanned;
  const status = statusOf(body);
  // What follows «API Error: 403» is the payload; a text without the prefix is the payload itself.
  const payload = body.replace(/^[\s\S]*?\bapi error:?\s*\(?\d{3}\)?[\s:·-]*/i, '');
  const found = deepest(payload);
  const kind = kindOf({ status, text: body, types: found.types, html: found.html, hinted });
  const tools = kind === ERROR_KIND.guard ? toolsOf(body) : [];
  const words = clip(found.text);
  // The status alone («403») or nothing left: the headline already says it.
  const reason = words === '' || /^\d{3}$/.test(words) ? null : words;
  return { kind, status, reason, tools, raw };
};

/** The agent's sign-in ran out: the one kind of failure that the login card cures. */
export const isLoginError = (text: string | null | undefined): boolean =>
  parseErrorText(text).kind === ERROR_KIND.login;

/** A failure that a plain second try may pass: the request never reached the model, or the model was busy. */
export const isRetryable = (kind: TErrorKind): boolean =>
  kind === ERROR_KIND.guard ||
  kind === ERROR_KIND.blocked ||
  kind === ERROR_KIND.network ||
  kind === ERROR_KIND.server ||
  kind === ERROR_KIND.overloaded;
