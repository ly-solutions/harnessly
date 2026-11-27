// «Общий доступ» of the one «Поделиться», the Google Docs way: «Доступ ограничен» or «Все, у кого есть
// ссылка» with a level (смотреть / комментировать). On, the resource opens to anyone at its one address, without an
// account and whether the owner's machine is on or not; the dialog's «Копировать ссылку» copies that address.
import { useState } from 'react';
import { canLabel } from '@/next/features/designly/access/labels';
import type { IResourceSection } from '@/next/features/designly/access/resource-people';
import { putGeneral, type TGeneralLevel } from '@/next/features/designly/access/resource-api';
import RoleMenu from '@/next/features/designly/access/role-menu';
import Icon from '@/next/ui/icon';
import Popover from '@/next/ui/popover';
import { t } from '@/next/i18n';

const LEVELS = ['view', 'comment'] as const;

export const ResourceGeneral = (props: IResourceSection) => {
  const { target, view, action, onNext } = props;
  const [open, setOpen] = useState(false);
  const general = view.general;
  if (!general) return null;
  const on = general.level !== 'none';
  const idle = action.busy === null;
  const set = (level: TGeneralLevel) =>
    void action.run('general', async () => onNext(await putGeneral(target, level)));
  const scope = t('next.access.general.scope');
  return (
    <section className="hx-acc-sec" data-section="general">
      <h3 className="hx-acc-sec__title">{t('next.access.general.title')}</h3>
      <ul className="hx-acc-rows">
        <li className="hx-acc-row hx-acc-general" data-on={on || undefined}>
          <span className="hx-acc-av hx-acc-link__ic" data-live={on || undefined} aria-hidden="true">
            <Icon name={on ? 'globe' : 'lock'} size="sm" />
          </span>
          <span className="hx-acc-row__who">
            {view.canManage ? (
              <Popover
                open={open}
                onOpenChange={setOpen}
                label={scope}
                side="bottom"
                align="start"
                className="hx-acc-pop"
                trigger={
                  <button
                    type="button"
                    className="hx-acc-role hx-acc-general__scope"
                    aria-label={scope}
                    disabled={!idle}
                    data-busy={action.busy === 'general' || undefined}
                  >
                    <span className="hx-acc-row__nm">
                      {t(on ? 'next.access.general.anyone' : 'next.access.general.restricted')}
                    </span>
                    <Icon name="chevron-down" size="sm" />
                  </button>
                }
              >
                <div role="menu" aria-label={scope} className="hx-acc-menu">
                  {([false, true] as const).map((anyone) => (
                    <button
                      key={String(anyone)}
                      type="button"
                      role="menuitemradio"
                      aria-checked={anyone === on}
                      className="hx-acc-mi"
                      onClick={() => {
                        setOpen(false);
                        if (anyone !== on) set(anyone ? 'view' : 'none');
                      }}
                    >
                      <Icon name={anyone ? 'globe' : 'lock'} size="sm" className="hx-acc-mi__icon" />
                      <span className="hx-acc-mi__text">
                        {t(anyone ? 'next.access.general.anyone' : 'next.access.general.restricted')}
                      </span>
                      <Icon name="check" size="sm" className="hx-acc-mi__check" />
                    </button>
                  ))}
                </div>
              </Popover>
            ) : (
              <span className="hx-acc-row__nm">
                {t(on ? 'next.access.general.anyone' : 'next.access.general.restricted')}
              </span>
            )}
            <span className="hx-acc-row__sub">
              {t(on ? 'next.access.general.anyoneLead' : 'next.access.general.restrictedLead')}
            </span>
          </span>
          {on &&
            general.level !== 'none' &&
            (view.canManage ? (
              <RoleMenu
                label={scope}
                role={general.level}
                roles={LEVELS}
                disabled={!idle}
                onRole={(next) => set(next)}
              />
            ) : (
              <span className="hx-acc-row__fixed">{canLabel(general.level)}</span>
            ))}
        </li>
      </ul>
    </section>
  );
};

export default ResourceGeneral;
