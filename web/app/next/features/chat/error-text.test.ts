import { describe, expect, it } from 'vitest';
import {
  ERROR_KIND,
  REASON_MAX,
  isLoginError,
  isRetryable,
  parseErrorText,
} from '@/next/features/chat/error-text';

// The two refusals of 13.11.2026, as the agent wrote them into its transcript.
const GUARD_INNER =
  '{"error":"Security Guardrail Triggered: Malicious MCP detected. Invalid tools: mcp__harnessly-app__page_call","operation":"forbidden"}';
const GUARD_OUTER = JSON.stringify({
  error: `anon request failed: failed to detect critical data: fail anon mask critical data: anon error: details - ${GUARD_INNER}\n`,
});
const GUARD = `Please run /login · API Error: 403 ${GUARD_OUTER}`;
const CLOUDFLARE = 'Please run /login · API Error: 403 Attention Required! | Cloudflare';
const CLOUDFLARE_PAGE = `API Error: 403 <!DOCTYPE html>
<!--[if lt IE 7]> <html class="no-js ie6 oldie" lang="en-US"> <![endif]-->
<html class="no-js" lang="en-US">
<head>
<title>Attention Required! | Cloudflare</title>
<meta charset="UTF-8" />
<style>body{margin:0}</style>
</head>
<body><div id="cf-wrapper"><h1 data-translate="block_headline">Sorry, you have been blocked</h1></div></body></html>`;

describe('parseErrorText: the samples of 13.11', () => {
  it('finds the filter’s refusal under two levels of JSON and names the tool', () => {
    const error = parseErrorText(GUARD);
    expect(error.kind).toBe(ERROR_KIND.guard);
    expect(error.status).toBe(403);
    expect(error.tools).toEqual(['mcp__harnessly-app__page_call']);
    expect(error.reason).toBe(
      'Security Guardrail Triggered: Malicious MCP detected. Invalid tools: mcp__harnessly-app__page_call',
    );
    expect(error.raw).toBe(GUARD);
    expect(isLoginError(GUARD)).toBe(false);
    expect(isRetryable(error.kind)).toBe(true);
  });

  it('reads the same refusal when the inner quotes are not escaped (as the tracker has it)', () => {
    const text = `API Error: 403 {"error":"anon request failed: failed to detect critical data: fail anon mask critical data: anon error: details - ${GUARD_INNER}"}`;
    const error = parseErrorText(text);
    expect(error.kind).toBe(ERROR_KIND.guard);
    expect(error.tools).toEqual(['mcp__harnessly-app__page_call']);
    expect(error.reason).toMatch(/^Security Guardrail Triggered/);
  });

  it('takes the Cloudflare block for a network shield, not for a lost sign-in', () => {
    const error = parseErrorText(CLOUDFLARE);
    expect(error.kind).toBe(ERROR_KIND.blocked);
    expect(error.status).toBe(403);
    expect(error.reason).toBe('Attention Required! | Cloudflare');
    expect(isLoginError(CLOUDFLARE)).toBe(false);
    expect(isRetryable(error.kind)).toBe(true);
  });

  it('shows the title of a blocking HTML page and none of its markup', () => {
    const error = parseErrorText(CLOUDFLARE_PAGE);
    expect(error.kind).toBe(ERROR_KIND.blocked);
    expect(error.status).toBe(403);
    expect(error.reason).toBe('Attention Required! | Cloudflare');
  });
});

describe('parseErrorText: the sign-in', () => {
  it('keeps a real loss of the sign-in a sign-in error', () => {
    for (const text of [
      'Login expired · Please run /login',
      'Invalid API key · Please run /login',
      'Not logged in · Please run /login',
      'Please run /login',
      'API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"OAuth token has expired. Please obtain a new token or refresh your existing token."}} · Please run /login',
      'API Error: 403 {"type":"error","error":{"type":"permission_error","message":"OAuth token has been revoked."}} · Please run /login',
      'Your access token could not be refreshed because your refresh token was already used. Please log out and sign in again.',
    ])
      expect(parseErrorText(text).kind, text).toBe(ERROR_KIND.login);
  });

  it('gives the message of the API’s own JSON as the reason', () => {
    const error = parseErrorText(
      'API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"OAuth token has expired."}} · Please run /login',
    );
    expect(error.status).toBe(401);
    expect(error.reason).toBe('OAuth token has expired.');
  });

  it('does not take a 403 with no word about the account for a sign-in error', () => {
    const error = parseErrorText('Please run /login · API Error: 403 {"error":"forbidden"}');
    expect(error.kind).toBe(ERROR_KIND.denied);
    expect(error.reason).toBe('forbidden');
  });
});

describe('parseErrorText: other kinds', () => {
  it.each([
    [
      'API Error: 529 {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}',
      ERROR_KIND.overloaded,
      529,
      'Overloaded',
    ],
    ['API Error: 529 Overloaded', ERROR_KIND.overloaded, 529, 'Overloaded'],
    [
      'API Error: 500 {"type":"error","error":{"type":"api_error","message":"Internal server error"}}',
      ERROR_KIND.server,
      500,
      'Internal server error',
    ],
    [
      'API Error: 429 {"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your rate limit."}}',
      ERROR_KIND.rateLimit,
      429,
      'This request would exceed your rate limit.',
    ],
    [
      'API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 215000 tokens > 200000 maximum"}}',
      ERROR_KIND.request,
      400,
      'prompt is too long: 215000 tokens > 200000 maximum',
    ],
    ['API Error: Connection error.', ERROR_KIND.network, null, 'API Error: Connection error.'],
    [
      'TypeError: fetch failed (ECONNRESET)',
      ERROR_KIND.network,
      null,
      'TypeError: fetch failed (ECONNRESET)',
    ],
    ['Prompt is too long', ERROR_KIND.unknown, null, 'Prompt is too long'],
  ])('%s', (text, kind, status, reason) => {
    const error = parseErrorText(text);
    expect(error.kind).toBe(kind);
    expect(error.status).toBe(status);
    expect(error.reason).toBe(reason);
  });

  it('walks down several levels of «failed: details -» and of JSON as a string', () => {
    const inner = JSON.stringify({ message: 'upstream said no', code: 7 });
    const middle = JSON.stringify({ error: `proxy failed: details - ${inner}` });
    const text = `MCP error -32000: call failed: details - ${JSON.stringify({ error: { message: `gateway: ${middle}` } })}`;
    expect(parseErrorText(text).reason).toBe('upstream said no');
    expect(parseErrorText('a failed: b failed: details - the disk is full').reason).toBe('the disk is full');
  });

  it('reads the status of a tool’s JSON answer', () => {
    const error = parseErrorText('{"status":502,"message":"Bad gateway from the tracker"}');
    expect(error.status).toBe(502);
    expect(error.reason).toBe('Bad gateway from the tracker');
  });
});

describe('parseErrorText: rubbish', () => {
  it('answers an empty text with nothing', () => {
    for (const text of ['', '   \n ', null, undefined]) {
      const error = parseErrorText(text);
      expect(error).toMatchObject({ kind: ERROR_KIND.unknown, status: null, reason: null, raw: '' });
    }
  });

  it('gets the deepest message out of JSON cut off in the middle', () => {
    const cut = GUARD.slice(0, GUARD.indexOf('Malicious MCP') + 'Malicious MCP'.length);
    const error = parseErrorText(cut);
    expect(error.kind).toBe(ERROR_KIND.guard);
    expect(error.status).toBe(403);
    expect(error.reason).toBe('Security Guardrail Triggered: Malicious MCP');
    expect(error.tools).toEqual([]);
  });

  it('cuts a very long text to one short line and keeps the raw text whole', () => {
    const long = `Something broke ${'x'.repeat(200_000)}`;
    const error = parseErrorText(long);
    expect(error.kind).toBe(ERROR_KIND.unknown);
    expect(error.reason).toHaveLength(REASON_MAX);
    expect(error.reason?.endsWith('…')).toBe(true);
    expect(error.raw).toBe(long);
  });

  it('never throws on braces, tags and escapes that lead nowhere', () => {
    for (const text of [
      '{{{{{{{{{{{{',
      '}}}}{{{{',
      '{"error":',
      '{"error":{"message":null}}',
      '[1,2,3]',
      '<html><head><title></title>',
      '<<<>>><script>',
      '\\\\\\"\\"\\n',
      `{"a":${'{"a":'.repeat(40)}1${'}'.repeat(41)}`,
      'API Error: 403',
    ]) {
      const error = parseErrorText(text);
      expect(error.raw, text).toBe(text.trim());
      expect((error.reason ?? '').length, text).toBeLessThanOrEqual(REASON_MAX);
    }
    expect(parseErrorText('API Error: 403')).toMatchObject({
      kind: ERROR_KIND.denied,
      status: 403,
      reason: null,
    });
  });

  it('stays fast on text made to make a pattern backtrack', () => {
    const started = performance.now();
    for (const text of [
      ' '.repeat(100_000),
      `${' ·'.repeat(50_000)} please run`,
      '\\'.repeat(100_000),
      '<'.repeat(100_000),
      `{"error":"${'\\"'.repeat(50_000)}`,
    ])
      parseErrorText(text);
    expect(performance.now() - started).toBeLessThan(1000);
  });

  it('strips the markup of a page that has no title', () => {
    const error = parseErrorText('<html><body><h2>Blocked</h2><p>Your IP is not allowed</p></body></html>');
    expect(error.kind).toBe(ERROR_KIND.blocked);
    expect(error.reason).toBe('Blocked Your IP is not allowed');
  });
});
