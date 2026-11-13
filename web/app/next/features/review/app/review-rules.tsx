// The person's instructions for the agent's review, beside «Пройти ревью»: the general ones and, at a change of a
// repository, that repository's own. Both go to the agent with the request; the fields say which apply here.
import { useState } from 'react';
import { RULES_TEXT_MAX, type IReviewRulesApi } from '@/next/features/review/app/rules';
import { Button, RoundButton } from '@/next/ui/button';
import Popover from '@/next/ui/popover';
import { toast } from '@/next/ui/toast';
import { t } from '@/next/i18n';

interface IRulesFormProps {
  api: IReviewRulesApi;
  /** The repository of the change at hand (`repoKeyOf`); null where there is none. */
  repo: string | null;
  repoName: string | null;
  onDone: () => void;
}

const RulesForm = ({ api, repo, repoName, onDone }: IRulesFormProps) => {
  const [general, setGeneral] = useState(api.rules.instructions);
  const [own, setOwn] = useState(repo ? (api.rules.repos[repo] ?? '') : '');
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    const ok = await api.change({
      instructions: general,
      ...(repo ? { repo: { key: repo, text: own } } : {}),
    });
    setBusy(false);
    if (ok) onDone();
    else toast(t('next.reviewApp.rules.notSaved'), { tone: 'error' });
  };
  return (
    <div className="hx-rvrules__form">
      <label className="hx-rvrules__label">
        {t('next.reviewApp.rules.general')}
        <textarea
          className="hx-rvnote__field"
          value={general}
          maxLength={RULES_TEXT_MAX}
          placeholder={t('next.reviewApp.rules.generalHint')}
          onChange={(event) => setGeneral(event.target.value)}
        />
      </label>
      {repo && (
        <label className="hx-rvrules__label">
          {t('next.reviewApp.rules.repo', { repo: repoName ?? repo })}
          <textarea
            className="hx-rvnote__field"
            value={own}
            maxLength={RULES_TEXT_MAX}
            placeholder={t('next.reviewApp.rules.repoHint')}
            onChange={(event) => setOwn(event.target.value)}
          />
        </label>
      )}
      <div className="hx-rvnote__actions">
        <Button size="sm" variant="ghost" disabled={busy} onClick={onDone}>
          {t('next.reviewApp.rules.cancel')}
        </Button>
        <Button size="sm" variant="primary" busy={busy} disabled={busy} onClick={() => void save()}>
          {t('next.reviewApp.rules.save')}
        </Button>
      </div>
    </div>
  );
};

export const ReviewRules = (props: Omit<IRulesFormProps, 'onDone'>) => {
  const [open, setOpen] = useState(false);
  const { rules } = props.api;
  const applied = Boolean(rules.instructions || (props.repo && rules.repos[props.repo]));
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label={t('next.reviewApp.rules.title')}
      className="hx-rvrules"
      side="bottom"
      align="end"
      trigger={
        <RoundButton
          icon="sliders-horizontal"
          size="sm"
          variant="ghost"
          label={t(applied ? 'next.reviewApp.rules.applied' : 'next.reviewApp.rules.title')}
        />
      }
    >
      {open && <RulesForm {...props} onDone={() => setOpen(false)} />}
    </Popover>
  );
};
