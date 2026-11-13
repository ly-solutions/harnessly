// What stands in the place of a built-in app's view while its service is down: an icon, one calm sentence and
// «Повторить», nothing technical. The chat beside it keeps working; the view comes back by itself (service-health.ts).
import type { TAppService } from '@/next/features/app-views/service-health';
import { Button } from '@/next/ui/button';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

interface IAppUnavailableProps {
  service: TAppService;
  /** An object is open (a document, a board), not the app's home. */
  object: boolean;
  icon: string;
  checking: boolean;
  onRetry: () => void;
}

const titleKey = (service: TAppService, object: boolean): string => {
  if (service === 'boards') return object ? 'next.appdown.board' : 'next.appdown.boards';
  return object ? 'next.appdown.doc' : 'next.appdown.docs';
};

const textKey = (service: TAppService, object: boolean): string => {
  if (!object) return 'next.appdown.homeText';
  return service === 'boards' ? 'next.appdown.boardText' : 'next.appdown.docText';
};

const AppUnavailable = ({ service, object, icon, checking, onRetry }: IAppUnavailableProps) => (
  <div className="hx-appdown" role="status">
    <span className="hx-appdown__ico">
      <Icon name={icon} />
    </span>
    <h3 className="hx-appdown__title">{t(titleKey(service, object))}</h3>
    <p className="hx-appdown__text">{t(textKey(service, object))}</p>
    <Button size="sm" icon="refresh-cw" busy={checking} onClick={onRetry}>
      {t('next.appfail.retry')}
    </Button>
  </div>
);

export default AppUnavailable;
