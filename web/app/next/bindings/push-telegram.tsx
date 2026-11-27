// The way out when this device's web push does not answer: Settings → Notifications, its Telegram bots.
import { t } from '@/next/i18n';
import { goTo, ROUTE } from '@/next/routes/routes';
import { TELEGRAM_BOTS_ID } from '@/next/stores/push-offer';
import { Button } from '@/next/ui/button';

export const openTelegramBots = () => {
  goTo({ kind: ROUTE.settings, section: 'notifications' });
  // The section draws after the route changes: the bots' part is scrolled to once it is there.
  window.setTimeout(() => document.getElementById(TELEGRAM_BOTS_ID)?.scrollIntoView({ block: 'start' }), 120);
};

const PushTelegramLink = () => (
  <Button size="sm" variant="ghost" onClick={openTelegramBots}>
    {t('next.connectors.push.telegram')}
  </Button>
);

export default PushTelegramLink;
