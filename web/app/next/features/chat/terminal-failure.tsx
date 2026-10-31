// A terminal provider failure at the end of a chat. It never starts work by itself: the person retries the current
// model or opens the existing manual handoff picker.
import type { TerminalFailure } from '@/next/protocol/v2';
import { Button } from '@/next/ui/button';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';
import usePending, { isBusy } from '@/next/ui/use-pending';
import '@/next/styles/limit.css';

interface ITerminalFailureCardProps {
  failure: TerminalFailure | null;
  onChooseTarget: (() => void) | null;
  onRetry: () => Promise<unknown>;
}

const TerminalFailureCard = ({ failure, onChooseTarget, onRetry }: ITerminalFailureCardProps) => {
  const action = usePending();
  if (!failure || !failure.terminal || !failure.needsTarget) return null;
  const busy = isBusy(action.phase);
  return (
    <div className="hx-terminalfailure" role="alert">
      <Icon name="circle-alert" size="sm" className="hx-terminalfailure__ic" />
      <span className="hx-terminalfailure__body">
        <strong className="hx-terminalfailure__title">{t('next.terminalFailure.quota.title')}</strong>
        <span className="hx-terminalfailure__text">{t('next.terminalFailure.quota.lead')}</span>
        {failure.detail && <span className="hx-terminalfailure__detail">{failure.detail}</span>}
      </span>
      <Button
        size="sm"
        variant="ghost"
        busy={busy}
        disabled={busy}
        onClick={() => action.run('retry', onRetry)}
      >
        {t('next.terminalFailure.retry')}
      </Button>
      <Button size="sm" disabled={busy || onChooseTarget === null} onClick={onChooseTarget ?? undefined}>
        {t('next.terminalFailure.choose')}
      </Button>
    </div>
  );
};

export default TerminalFailureCard;
