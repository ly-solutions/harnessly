// A design opened without a network: its offline copy, the HTML export the page saved while online, drawn
// in place of the live viewer with a strip that says what it is; without a copy, a card that says why and when it
// opens. The copy runs sandboxed in an opaque origin, as the export runs anywhere else.
import { useEffect, useState } from 'react';
import { t } from '@/next/i18n';
import { checkNet } from '@/next/offline/net';
import { readDesignCopy } from '@/next/offline/kept';
import type { Copy } from '@/next/offline/storage';
import LoadFailed, { LOAD_FAILED_SIZE } from '@/next/ui/load-failed';
import OfflineMark from '@/next/ui/offline-mark';
import Icon from '@/next/ui/icon';

interface IOfflineDesignProps {
  machine: string;
  id: string;
  /** In the right pane: the way back to the library. */
  onBack?: () => void;
}

type TRead = { k: 'reading' } | { k: 'none' } | { k: 'copy'; copy: Copy; html: string };

const OfflineDesign = ({ machine, id, onBack }: IOfflineDesignProps) => {
  const [read, setRead] = useState<TRead>({ k: 'reading' });
  useEffect(() => {
    let alive = true;
    void readDesignCopy(machine, id).then(async (copy) => {
      if (!alive) return;
      if (!copy || copy.kind !== 'html') {
        setRead({ k: 'none' });
        return;
      }
      const html = await copy.body.text();
      if (alive) setRead({ k: 'copy', copy, html });
    });
    return () => {
      alive = false;
    };
  }, [machine, id]);

  if (read.k === 'reading') return <div className="hx-offdesign" aria-busy="true" />;
  if (read.k === 'none')
    return (
      <div className="hx-offdesign hx-offdesign--none">
        <LoadFailed
          size={LOAD_FAILED_SIZE.page}
          title={t('next.offline.designTitle')}
          reason={t('next.offline.designBody')}
          onRetry={() => checkNet()}
        />
      </div>
    );
  const when = new Date(read.copy.at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  return (
    <div className="hx-offdesign">
      <div className="hx-offdesign__bar" role="status">
        {onBack && (
          <button
            type="button"
            className="hx-btn"
            data-variant="ghost"
            data-size="sm"
            data-shape="icon"
            aria-label={t('next.site.back')}
            onClick={onBack}
          >
            <Icon name="arrow-left" size="sm" />
          </button>
        )}
        <OfflineMark label={t('next.offline.hint')} word={t('next.offline.badge')} />
        <span className="hx-offdesign__title">{read.copy.title}</span>
        <span className="hx-offdesign__note">{t('next.offline.designCopy', { when })}</span>
      </div>
      <iframe
        className="hx-offdesign__frame"
        title={read.copy.title}
        sandbox="allow-scripts allow-popups"
        srcDoc={read.html}
      />
    </div>
  );
};

export default OfflineDesign;
