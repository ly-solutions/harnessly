// Settings → Plugins: how the built-in apps show up outside their own screens. «Code review»: the card of the chat's
// merge requests under a finished task. The switch is the feed's `mrDone` kind, kept where the other feed switches are
// (stores/feed-kinds), so there is one place that decides whether the card is drawn. «Hide the bots» is the person's
// review rule, kept on the machine: the switch waits for its answer.
import { useState } from 'react';
import { Group, SetButton, SwitchRow } from '@/next/features/settings/rows';
import { setFeedKind, useFeedKinds } from '@/next/stores/feed-kinds';
import { changeReviewRules, useReviewRules } from '@/next/legacy/review-rules';
import { toast } from '@/next/ui/toast';
import { t } from '@/next/i18n';

const namesOf = (text: string): string[] =>
  text
    .split(',')
    .map((name) => name.trim())
    .filter(Boolean);

/** The authors the person calls bots too: names through commas, kept when «Сохранить» is pressed. */
const OwnBots = ({ bots }: { bots: readonly string[] }) => {
  const kept = bots.join(', ');
  const [text, setText] = useState(kept);
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    const ok = await changeReviewRules({ bots: namesOf(text) });
    setBusy(false);
    if (!ok) toast(t('next.settings.plugins.review.notSaved'), { tone: 'error' });
  };
  return (
    <form
      className="hx-key-form hx-voice-words"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <label className="hx-voice-words__field">
        <span className="hx-nb__label">{t('next.settings.plugins.review.bots')}</span>
        <input
          className="hx-set__input"
          value={text}
          autoComplete="off"
          spellCheck={false}
          placeholder={t('next.settings.plugins.review.bots.placeholder')}
          onChange={(e) => setText(e.target.value)}
        />
      </label>
      <div className="hx-key-form__row">
        <SetButton
          tone="primary"
          busy={busy}
          disabled={namesOf(text).join(', ') === kept}
          onClick={() => void save()}
        >
          {t('next.settings.plugins.review.bots.save')}
        </SetButton>
      </div>
    </form>
  );
};

export const PluginsPage = () => {
  const kinds = useFeedKinds();
  const rules = useReviewRules();
  const [busy, setBusy] = useState(false);
  const hideBots = async (on: boolean) => {
    setBusy(true);
    const ok = await changeReviewRules({ hideBots: on });
    setBusy(false);
    if (!ok) toast(t('next.settings.plugins.review.notSaved'), { tone: 'error' });
  };
  return (
    <Group title={t('next.settings.plugins.review')}>
      <SwitchRow
        label={t('next.settings.plugins.review.mrCard')}
        hint={t('next.settings.plugins.review.mrCard.hint')}
        on={kinds.mrDone}
        onChange={(on) => setFeedKind('mrDone', on)}
      />
      <SwitchRow
        label={t('next.settings.plugins.review.hideBots')}
        hint={t('next.settings.plugins.review.hideBots.hint')}
        on={rules.hideBots}
        busy={busy}
        onChange={(on) => void hideBots(on)}
      />
      {rules.hideBots && <OwnBots key={rules.bots.join('\n')} bots={rules.bots} />}
    </Group>
  );
};
