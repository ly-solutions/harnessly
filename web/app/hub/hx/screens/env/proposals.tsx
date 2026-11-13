// «Предложения агентов»: what agents on the members' machines proposed for the organization's environment. A machine
// changes nothing itself: an admin of the organization reads here what would change (the secrets by their names, never a
// value), which machine and which chat it came from, and applies or declines it under their own session. The block is
// there only while something waits. The name, the hint and the chat's title are the agent's words, shown as text.
import { useCallback, useEffect, useState } from 'react';

import type { CatalogSource, EnvProposal, EnvProposalChange } from '../../../../hub-api/env';
import { orgErrorText } from '../../../org/api/errors';
import { useOrg } from '../../../org/context';
import { useClosing } from '../../../org/ui/closing';
import { Btn, closingAttr, Icon } from '../../ui';
import { whenText } from './format';

const TITLE: Record<EnvProposalChange, string> = {
  add: 'Добавить провайдера моделей',
  update: 'Изменить провайдера моделей',
  remove: 'Убрать провайдера моделей',
};

const DONE: Record<EnvProposalChange, string> = {
  add: 'Провайдер добавлен в окружение организации',
  update: 'Провайдер изменён',
  remove: 'Провайдер убран из окружения организации',
};

/** The names of the variables: a value of a plain one is shown, a secret one says the secret's name. */
const linesOf = (source: CatalogSource): { label: string; text: string }[] => {
  const out: { label: string; text: string }[] = [];
  if (source.baseUrl) out.push({ label: 'Адрес', text: source.baseUrl });
  if (source.models && source.models.length > 0)
    out.push({ label: 'Модели', text: source.models.join(', ') });
  if (source.key) out.push({ label: 'Ключ', text: `секрет «${source.key}»` });
  if (source.env && source.env.length > 0) out.push({ label: 'Переменные', text: source.env.join(', ') });
  if (source.secretEnv && source.secretEnv.length > 0)
    out.push({
      label: 'Секретные переменные',
      text: source.secretEnv
        .map((line) => {
          const at = line.indexOf('=');
          return at < 0 ? line : `${line.slice(0, at)} — секрет «${line.slice(at + 1)}»`;
        })
        .join(', '),
    });
  return out;
};

const fromText = (p: EnvProposal): string => {
  const chat = p.convTitle || p.conv;
  return [`машина «${p.machine}»`, p.owner === '' ? null : p.owner, chat === '' ? null : `чат «${chat}»`]
    .filter((x) => x !== null)
    .join(' · ');
};

const Lines = ({ title, source }: { title: string | null; source: CatalogSource }) => (
  <dl className="ev-prop__lines">
    {title !== null && <dt className="ev-prop__was">{title}</dt>}
    {linesOf(source).map((l) => (
      <div key={l.label}>
        <dt>{l.label}</dt>
        <dd>{l.text}</dd>
      </div>
    ))}
  </dl>
);

const Card = ({
  proposal,
  now,
  busy,
  onApply,
  onDecline,
}: {
  proposal: EnvProposal;
  now: number;
  busy: 'apply' | 'decline' | null;
  onApply: () => void;
  onDecline: () => void;
}) => (
  <li className="ev-prop">
    <div className="ev-prop__t">
      <b>
        {TITLE[proposal.change]} «{proposal.name}»
      </b>
      {proposal.hint !== '' && <p>{proposal.hint}</p>}
      {proposal.change === 'update' && proposal.before && <Lines title="Сейчас" source={proposal.before} />}
      {proposal.source && (
        <Lines title={proposal.change === 'update' ? 'Станет' : null} source={proposal.source} />
      )}
      {proposal.change === 'remove' && proposal.before && <Lines title={null} source={proposal.before} />}
      <small>
        Предложил агент: {fromText(proposal)}. Ждёт до{' '}
        {whenText(new Date(proposal.expiresAt).toISOString(), now)}
      </small>
    </div>
    <div className="ev-prop__acts">
      <Btn variant="primary" size="sm" busy={busy === 'apply'} disabled={busy !== null} onClick={onApply}>
        Применить
      </Btn>
      <Btn variant="ghost" size="sm" busy={busy === 'decline'} disabled={busy !== null} onClick={onDecline}>
        Отклонить
      </Btn>
    </div>
  </li>
);

export const AgentProposals = ({
  onChanged,
  say,
  fail,
}: {
  /** The environment changed: the screen reads it again. */
  onChanged: () => Promise<void>;
  say: (text: string) => void;
  fail: (text: string) => void;
}) => {
  const { api, shell } = useOrg();
  const org = shell.org.slug;
  // null: not read yet. The block is not drawn until there is something to decide.
  const [list, setList] = useState<EnvProposal[] | null>(null);
  const [busy, setBusy] = useState<{ id: string; act: 'apply' | 'decline' } | null>(null);
  const [last, setLast] = useState<EnvProposal[]>([]);
  const [now] = useState(() => Date.now());
  const shown = list !== null && list.length > 0;
  const anim = useClosing(shown, 280);
  if (shown && last !== list) setLast(list);

  const read = useCallback(async () => {
    setList(await api.envProposals(org));
  }, [api, org]);

  useEffect(() => {
    let alive = true;
    api.envProposals(org).then(
      (got) => {
        if (alive) setList(got);
      },
      // Nothing to decide is shown when the list did not come: the environment itself says what is wrong.
      () => undefined,
    );
    return () => {
      alive = false;
    };
  }, [api, org]);

  const decide = async (p: EnvProposal, act: 'apply' | 'decline') => {
    setBusy({ id: p.id, act });
    try {
      if (act === 'apply') {
        await api.envProposalApply(org, p.id);
        await onChanged();
        say(DONE[p.change]);
      } else {
        await api.envProposalDecline(org, p.id);
        say('Предложение отклонено');
      }
    } catch (e) {
      fail(orgErrorText(e));
    }
    try {
      await read();
    } catch (e) {
      fail(orgErrorText(e));
    }
    setBusy(null);
  };

  if (!anim.mounted) return null;
  const rows = shown ? list : last;
  return (
    <div className="ev-intro-w" data-closing={closingAttr(anim.closing)} data-block="Предложения агентов">
      <div>
        <section className="ev-props" aria-label="Предложения агентов">
          <h2>
            <Icon i="sparkles" size="sm" />
            Предложения агентов
          </h2>
          <ul>
            {rows.map((p) => (
              <Card
                key={p.id}
                proposal={p}
                now={now}
                busy={busy !== null && busy.id === p.id ? busy.act : null}
                onApply={() => void decide(p, 'apply')}
                onDecline={() => void decide(p, 'decline')}
              />
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
};
