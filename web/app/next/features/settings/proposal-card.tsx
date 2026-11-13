// An agent of an ordinary chat asks to change a setting: the card over the chat's field with what changes,
// from what to what, the agent's why, and the person's yes or no. Nothing changes until the machine has taken the yes.
import { useState } from 'react';
import type { ISettingProposal, TProposalVerb } from '@/next/features/settings/changes-api';
import { PROPOSAL_VERB } from '@/next/features/settings/changes-api';
import { t } from '@/next/i18n';
import { Button } from '@/next/ui/button';
import Icon from '@/next/ui/icon';

interface IProposalCardProps {
  proposal: ISettingProposal;
  closing: boolean;
  /** Resolves null when the machine took the answer, else the words of why not. */
  onAnswer: (proposal: ISettingProposal, verb: TProposalVerb) => Promise<string | null>;
}

const ProposalCard = ({ proposal, closing, onAnswer }: IProposalCardProps) => {
  const [busy, setBusy] = useState<TProposalVerb | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const answer = async (verb: TProposalVerb) => {
    if (busy !== null) return;
    setBusy(verb);
    setFailure(null);
    const refused = await onAnswer(proposal, verb);
    setBusy(null);
    setFailure(refused);
  };
  return (
    <section
      className="hx-setask"
      data-state={closing ? 'closing' : undefined}
      aria-label={t('next.settingsChanges.ask.aria')}
    >
      <div className="hx-setask__text">
        <span className="hx-setask__lead">{t('next.settingsChanges.ask.lead')}</span>
        <strong className="hx-setask__title">{proposal.title}</strong>
        <span className="hx-setlog__diff">
          <s>{proposal.oldLabel}</s>
          <Icon name="arrow-right" size="sm" />
          <span>{proposal.newLabel}</span>
        </span>
        {proposal.why !== '' && <span className="hx-setask__why">{proposal.why}</span>}
      </div>
      {failure && (
        <p className="hx-setask__fail" role="alert">
          {failure}
        </p>
      )}
      <div className="hx-setask__acts">
        <Button
          size="sm"
          variant="ghost"
          busy={busy === PROPOSAL_VERB.decline}
          disabled={busy !== null}
          onClick={() => void answer(PROPOSAL_VERB.decline)}
        >
          {t('next.settingsChanges.ask.no')}
        </Button>
        <Button
          size="sm"
          variant="primary"
          busy={busy === PROPOSAL_VERB.approve}
          disabled={busy !== null}
          onClick={() => void answer(PROPOSAL_VERB.approve)}
        >
          {t('next.settingsChanges.ask.yes')}
        </Button>
      </div>
    </section>
  );
};

export default ProposalCard;
