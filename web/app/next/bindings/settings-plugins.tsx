// Settings → Plugins: how the built-in apps show up outside their own screens. «Code review»: the card of the chat's
// merge requests under a finished task. The switch is the feed's `mrDone` kind, kept where the other feed switches are
// (stores/feed-kinds), so there is one place that decides whether the card is drawn.
import { Group, SwitchRow } from '@/next/features/settings/rows';
import { setFeedKind, useFeedKinds } from '@/next/stores/feed-kinds';
import { t } from '@/next/i18n';

export const PluginsPage = () => {
  const kinds = useFeedKinds();
  return (
    <Group title={t('next.settings.plugins.review')}>
      <SwitchRow
        label={t('next.settings.plugins.review.mrCard')}
        hint={t('next.settings.plugins.review.mrCard.hint')}
        on={kinds.mrDone}
        onChange={(on) => setFeedKind('mrDone', on)}
      />
    </Group>
  );
};
