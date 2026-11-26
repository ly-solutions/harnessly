// «Кто откроет по ссылке»: who the design's address lets in — the invited people only, the whole organization
// (the team's level), or anyone with the link without signing in (a link of its own, made here and revoked here) — one
// segment and one quiet line of what it means.
import { useState } from 'react';
import {
  createGuestLink,
  putTeam,
  revokeGuestLink,
  type IGuestLink,
  type TTeamLevel,
} from '@/next/features/designly/access/api';
import { change, type ISectionProps } from '@/next/features/designly/access/section';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

export const AUDIENCE = { invited: 'invited', org: 'org', anyone: 'anyone' } as const;
export type TAudience = (typeof AUDIENCE)[keyof typeof AUDIENCE];

/** The title the link «Все по ссылке» is made with: only such a link is revoked by a switch away from it. */
export const ANYONE_TITLE = 'Все по ссылке';

const alive = (l: IGuestLink, now: number) =>
  l.revokedAt === null && (l.expiresAt === null || new Date(l.expiresAt).getTime() > now);

export const audienceOf = (
  team: TTeamLevel,
  guests: readonly IGuestLink[] | null,
  now = Date.now(),
): TAudience => {
  if (guests?.some((l) => l.title === ANYONE_TITLE && alive(l, now))) return AUDIENCE.anyone;
  return team === 'none' ? AUDIENCE.invited : AUDIENCE.org;
};

export interface ILinkAudienceProps extends ISectionProps {
  /** The links without sign-in; null where the gateway has none (then «Все по ссылке» is not offered). */
  guests: readonly IGuestLink[] | null;
  onGuests: (links: IGuestLink[]) => void;
}

const LinkAudience = (props: ILinkAudienceProps) => {
  const { project, access, action, guests, onGuests } = props;
  const [now] = useState(() => Date.now());
  const at = audienceOf(access.team, guests, now);
  const options: TAudience[] = guests === null ? [AUDIENCE.invited, AUDIENCE.org] : Object.values(AUDIENCE);
  const ownLinks = (guests ?? []).filter((l) => l.title === ANYONE_TITLE && alive(l, now));
  const dropOwn = async () => {
    for (const l of ownLinks) await revokeGuestLink(project, l.id);
    if (ownLinks.length > 0) onGuests((guests ?? []).filter((l) => !ownLinks.includes(l)));
  };
  const pick = (next: TAudience) => {
    if (next === at) return;
    if (next === AUDIENCE.anyone) {
      void action.run('audience', async () => {
        const made = await createGuestLink(project, { title: ANYONE_TITLE, rights: 'view', ttl: null });
        onGuests([...(guests ?? []), made]);
      });
      return;
    }
    const level: TTeamLevel =
      next === AUDIENCE.invited ? 'none' : access.team === 'none' ? 'view' : access.team;
    change(props, 'audience', async () => {
      await dropOwn();
      return level === access.team ? access : putTeam(project, level);
    });
  };
  return (
    <section className="hx-acc-card" data-section="team">
      <div className="hx-acc-card__h">
        <Icon name="globe" size="sm" />
        <b>{t('next.access.audience.title')}</b>
      </div>
      <div className="hx-acc-seg" role="radiogroup" aria-label={t('next.access.audience.title')}>
        {options.map((o) => (
          <button
            key={o}
            type="button"
            role="radio"
            aria-checked={o === at}
            disabled={action.busy !== null}
            onClick={() => pick(o)}
          >
            {t(`next.access.audience.${o}`)}
          </button>
        ))}
      </div>
      <p className="hx-acc-card__hint">{t(`next.access.audience.${at}.hint`)}</p>
    </section>
  );
};

export default LinkAudience;
