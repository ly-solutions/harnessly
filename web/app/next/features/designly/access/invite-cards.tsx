// «Ссылки-приглашения» of «Поделиться»: each live invite a card — its name and role, how long it lives and
// «3 из 10 вошли» with a bar, copy (the address is known only from the moment it is made: the gateway keeps a hash) and
// revoke; «Новая» opens the form inside the card — the name, the role, how long the link lives (chips), how many people
// (a counter), only for the team — and «Создать ссылку» copies the new address.
import { useLayoutEffect, useRef, useState } from 'react';
import {
  createInvite,
  fetchAccess,
  revokeInvite,
  type IInvite,
  type TInviteRole,
} from '@/next/features/designly/access/api';
import { isAlive, copyText } from '@/next/features/designly/access/copy-link';
import { change, type ISectionProps } from '@/next/features/designly/access/section';
import { canLabel, INVITE_ROLES, untilLabel } from '@/next/features/designly/access/labels';
import { toast } from '@/next/features/designly/sdk/toast';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

const TERMS = ['1d', '7d', '30d'] as const;
type TTerm = (typeof TERMS)[number];
const MAX_STEPS = [null, 1, 3, 5, 10, 25, 50, 100] as const;

interface IForm {
  name: string;
  role: TInviteRole;
  term: TTerm;
  max: (typeof MAX_STEPS)[number];
  team: boolean;
}

const BLANK: IForm = { name: '', role: 'view', term: '7d', max: 10, team: false };

/** The share of the places taken, for the bar: «3 из 10» is 30%. */
export const usedShare = (invite: Pick<IInvite, 'uses' | 'maxUses'>): number =>
  invite.maxUses === null || invite.maxUses <= 0
    ? 0
    : Math.min(100, Math.round((invite.uses / invite.maxUses) * 100));

const copy = (url: string) =>
  copyText(Promise.resolve(url)).then(
    () => toast(t('next.access.invites.copied')),
    () => toast(t('next.site.copyFailed'), { tone: 'error' }),
  );

const Bar = ({ share }: { share: number }) => {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    ref.current?.style.setProperty('--w', `${share}%`);
  }, [share]);
  return (
    <div ref={ref} className="hx-acc-bar" role="presentation">
      <i />
    </div>
  );
};

export interface IInviteCardsProps extends ISectionProps {
  /** The team the project belongs to: «Только для команды» makes the link for it. */
  teamId?: string;
}

const InviteCards = (props: IInviteCardsProps) => {
  const { project, access, action } = props;
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<IForm>(BLANK);
  // The addresses made in this dialog: the gateway does not give them again.
  const [made, setMade] = useState<Record<string, string>>({});
  const [now] = useState(() => Date.now());
  const invites = (access.invites ?? []).filter((i) => isAlive(i, now));
  const put = (part: Partial<IForm>) => setForm((f) => ({ ...f, ...part }));
  const stepMax = (by: 1 | -1) => {
    const at = MAX_STEPS.indexOf(form.max);
    put({ max: MAX_STEPS[Math.max(0, Math.min(MAX_STEPS.length - 1, at + by))] });
  };
  const create = () =>
    void action.run('invite', async () => {
      const answer = await createInvite(project, {
        title: form.name.trim() || t('next.access.invites.untitled'),
        role: form.role,
        roleTtl: null,
        ttl: form.term,
        maxUses: form.max,
        ...(form.team && props.teamId ? { teamId: props.teamId } : {}),
      });
      setMade((m) => ({ ...m, [answer.id]: answer.url }));
      void copy(answer.url);
      setOpen(false);
      setForm(BLANK);
      const next = await fetchAccess(project);
      if (next) props.onNext(next);
    });
  return (
    <section className="hx-acc-card" data-section="invites">
      <div className="hx-acc-card__h">
        <Icon name="link" size="sm" />
        <b>{t('next.access.invites.title')}</b>
        <span className="hx-spacer" />
        <button type="button" className="hx-acc-add" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
          <Icon name="plus" size="sm" />
          {t('next.access.invites.new')}
        </button>
      </div>
      {invites.length === 0 && !open && <p className="hx-acc-card__hint">{t('next.access.invites.none')}</p>}
      {invites.map((i) => (
        <div key={i.id} className="hx-acc-link">
          <div className="hx-acc-link__t">
            <b>{i.title}</b>
            <span className="hx-acc-badge">{canLabel(i.role)}</span>
          </div>
          <div className="hx-acc-card__hint">
            {untilLabel(i.expiresAt) ?? ''}
            {i.maxUses !== null && ` · ${t('next.access.invites.used', { n: i.uses, max: i.maxUses })}`}
          </div>
          {i.maxUses !== null && <Bar share={usedShare(i)} />}
          <div className="hx-acc-link__a">
            {made[i.id] ? (
              <>
                <span className="hx-acc-link__url">{made[i.id].replace(/^https?:\/\//, '')}</span>
                <button
                  type="button"
                  className="hx-btn"
                  data-size="sm"
                  data-variant="ghost"
                  onClick={() => void copy(made[i.id])}
                >
                  <Icon name="copy" size="sm" />
                  <span>{t('next.access.invites.copy')}</span>
                </button>
              </>
            ) : (
              <span className="hx-acc-link__url hx-acc-link__url--hidden">
                {t('next.access.invites.hidden')}
              </span>
            )}
            <button
              type="button"
              className="hx-head-btn"
              aria-label={t('next.access.invites.revoke')}
              title={t('next.access.invites.revoke')}
              disabled={action.busy !== null}
              onClick={() => change(props, `revoke:${i.id}`, () => revokeInvite(project, i.id))}
            >
              <Icon name="trash-2" size="sm" />
            </button>
          </div>
        </div>
      ))}
      {open && (
        <div className="hx-acc-new">
          <input
            className="hx-acc-new__name"
            value={form.name}
            maxLength={80}
            placeholder={t('next.access.invites.namePh')}
            aria-label={t('next.access.invites.name')}
            onChange={(e) => put({ name: e.target.value })}
          />
          <div className="hx-acc-new__row">
            <span>{t('next.access.invites.role')}</span>
            <div className="hx-acc-seg hx-acc-seg--auto" role="radiogroup">
              {INVITE_ROLES.map((r) => (
                <button
                  key={r}
                  type="button"
                  role="radio"
                  aria-checked={form.role === r}
                  onClick={() => put({ role: r })}
                >
                  {canLabel(r)}
                </button>
              ))}
            </div>
          </div>
          <div className="hx-acc-new__row">
            <span>{t('next.access.invites.term')}</span>
            <div className="hx-acc-chips" role="radiogroup">
              {TERMS.map((term) => (
                <button
                  key={term}
                  type="button"
                  role="radio"
                  aria-checked={form.term === term}
                  onClick={() => put({ term })}
                >
                  {t(`next.access.invites.term.${term}`)}
                </button>
              ))}
            </div>
          </div>
          <div className="hx-acc-new__row">
            <span>{t('next.access.invites.max')}</span>
            <div className="hx-acc-count">
              <button type="button" aria-label={t('next.access.invites.less')} onClick={() => stepMax(-1)}>
                −
              </button>
              <span>{form.max === null ? '∞' : form.max}</span>
              <button type="button" aria-label={t('next.access.invites.more')} onClick={() => stepMax(1)}>
                +
              </button>
            </div>
          </div>
          {props.teamId && (
            <div className="hx-acc-new__row">
              <span>{t('next.access.invites.team')}</span>
              <button
                type="button"
                className="hx-acc-toggle"
                role="switch"
                aria-checked={form.team}
                aria-label={t('next.access.invites.team')}
                onClick={() => put({ team: !form.team })}
              >
                <i />
              </button>
            </div>
          )}
          <div className="hx-acc-new__foot">
            <button
              type="button"
              className="hx-btn"
              data-size="sm"
              data-variant="ghost"
              onClick={() => setOpen(false)}
            >
              {t('next.access.invites.cancel')}
            </button>
            <button
              type="button"
              className="hx-btn"
              data-size="sm"
              data-variant="primary"
              data-busy={action.busy === 'invite'}
              disabled={action.busy !== null}
              onClick={create}
            >
              {t('next.access.invites.create')}
            </button>
          </div>
        </div>
      )}
    </section>
  );
};

export default InviteCards;
