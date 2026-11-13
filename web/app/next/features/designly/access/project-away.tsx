// What a person with a place in a project sees when the project can be had from neither side: the
// hub's store has no copy of it (or could not be read) and the machine did not answer (or does not have it). A project
// the store or a machine on the link can give opens without this card. The card names the reason as it is; the page
// keeps looking on its own and opens the project when it arrives, unless nothing can bring it (a hub with no store, a
// machine closed to the person: GitHub #82); the way back is always there.
import { t } from '@/next/i18n';
import { AWAY, type TAway } from '@/next/features/designly/project-read';
import '@/next/styles/design-access.css';

interface IProjectAwayProps {
  /** Why the project is on neither side. */
  why: TAway;
  /** The owner's name from the gateway; empty when it is not known. */
  ownerName: string;
  onBack: () => void;
}

const TITLE = {
  [AWAY.silentNoCopy]: 'next.access.away.silentNoCopy.title',
  [AWAY.silentUnreadable]: 'next.access.away.silentUnreadable.title',
  [AWAY.absentNoCopy]: 'next.access.away.absentNoCopy.title',
  [AWAY.absentUnreadable]: 'next.access.away.absentUnreadable.title',
  [AWAY.silentStoreOff]: 'next.access.away.silentStoreOff.title',
  [AWAY.absentStoreOff]: 'next.access.away.absentStoreOff.title',
  [AWAY.machineDenied]: 'next.access.away.machineDenied.title',
} as const;

const LEAD = {
  [AWAY.silentNoCopy]: 'next.access.away.silentNoCopy.lead',
  [AWAY.silentUnreadable]: 'next.access.away.silentUnreadable.lead',
  [AWAY.absentNoCopy]: 'next.access.away.absentNoCopy.lead',
  [AWAY.absentUnreadable]: 'next.access.away.absentUnreadable.lead',
  [AWAY.silentStoreOff]: 'next.access.away.silentStoreOff.lead',
  [AWAY.absentStoreOff]: 'next.access.away.absentStoreOff.lead',
  [AWAY.machineDenied]: 'next.access.away.machineDenied.lead',
} as const;

const ProjectAway = ({ why, ownerName, onBack }: IProjectAwayProps) => (
  <section className="hx-acc-card" aria-label={t(TITLE[why])} role="status" data-why={why}>
    <h2 className="hx-acc-card__title">{t(TITLE[why])}</h2>
    <p className="hx-acc-card__lead">{t(LEAD[why])}</p>
    {ownerName && <p className="hx-acc-card__lead">{t('next.access.owner', { name: ownerName })}</p>}
    <div className="hx-acc-card__acts">
      <button type="button" className="hx-btn" data-variant="ghost" onClick={onBack}>
        {t('next.access.back')}
      </button>
    </div>
  </section>
);

export default ProjectAway;
