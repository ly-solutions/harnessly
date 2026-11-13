// An error said in one line: the human headline, the deepest reason, the status code as one label, and the raw text
// folded under «Details» with a copy button. The same line stands in the «Turn interrupted» row, in the agent's error
// notice and in a failed MCP tool's card; error-text.ts takes the text apart. The raw text is data: it is drawn as
// text only.
import { useId, useState } from 'react';
import CopyButton from '@/next/features/chat/copy-button';
import { ERROR_KIND, type ParsedError, type TErrorKind } from '@/next/features/chat/error-text';
import { t } from '@/next/i18n';
import Icon from '@/next/ui/icon';
import { useSeen } from '@/next/ui/presence';
import '@/next/styles/error-line.css';

const HEADLINE_KEYS: Record<TErrorKind, string | null> = {
  [ERROR_KIND.login]: 'next.error.login',
  [ERROR_KIND.guard]: 'next.error.guard',
  [ERROR_KIND.blocked]: 'next.error.blocked',
  [ERROR_KIND.rateLimit]: 'next.error.rateLimit',
  [ERROR_KIND.overloaded]: 'next.error.overloaded',
  [ERROR_KIND.server]: 'next.error.server',
  [ERROR_KIND.network]: 'next.error.network',
  [ERROR_KIND.request]: 'next.error.request',
  [ERROR_KIND.denied]: 'next.error.denied',
  [ERROR_KIND.unknown]: null,
};

/** The headline of a failed call to the model in human words; null when the text is of no known kind. */
export const errorHeadline = (error: ParsedError): string | null => {
  const key = HEADLINE_KEYS[error.kind];
  return key === null ? null : t(key);
};

/** The one line of the reason: the tools a filter refused, or the deepest message of the text. */
export const errorReason = (error: ParsedError): string | null => {
  if (error.tools.length === 1) return t('next.error.tool', { tool: error.tools[0] });
  if (error.tools.length > 1) return t('next.error.tools', { tools: error.tools.join(', ') });
  return error.reason;
};

interface IErrorLineProps {
  error: ParsedError;
  /** What stands before the reason; null: the reason alone (the row around already says that it failed). */
  headline: string | null;
}

const ErrorLine = ({ error, headline }: IErrorLineProps) => {
  const [open, setOpen] = useState(false);
  const seen = useSeen(open);
  const foldId = useId();
  const reason = errorReason(error);
  const summary = [headline, reason].filter((part) => part !== null).join(': ');
  // «Details» only when the raw text holds more than the line already shows.
  const more = error.raw !== '' && error.raw !== summary && error.raw !== reason;
  return (
    <span className="hx-err" data-open={open || undefined}>
      <span className="hx-err__line">
        <span className="hx-err__text">{summary === '' ? t('next.error.unknown') : summary}</span>
        {error.status !== null && <span className="hx-err__code">{error.status}</span>}
        {more && (
          <button
            type="button"
            className="hx-err__more"
            aria-expanded={open}
            aria-controls={foldId}
            onClick={() => setOpen((value) => !value)}
          >
            <span>{t('next.error.details')}</span>
            <Icon name="chevron-right" size="sm" />
          </button>
        )}
      </span>
      {more && (
        <span id={foldId} className="hx-err__fold" aria-hidden={!open}>
          <span className="hx-err__clip">
            {seen && (
              <span className="hx-err__raw">
                <span className="hx-err__pre">{error.raw}</span>
                <CopyButton text={error.raw} />
              </span>
            )}
          </span>
        </span>
      )}
    </span>
  );
};

export default ErrorLine;
