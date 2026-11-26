// «Макет → Обзор»: what the project is — its name and pages, when it was made and changed and by whom (its
// history), the design system it is drawn with (a link that opens it in another tab), the chats that changed it, who
// has access (with «Поделиться») and every link to it, each copied in one press.
import Icon from '@/next/ui/icon';
import { t, plural } from '@/next/i18n';
import { useConversation } from '@/next/stores/conversations';
import type { ConvId } from '@/next/protocol/v2';
import type { IProjectHistory } from '@/next/features/designly/versions/versions-api';
import { copyText } from '@/next/features/designly/access/copy-link';
import { toast } from '@/next/features/designly/sdk/toast';
import { whenOf } from '@/next/features/designly/panel/when';
import Avatar from '@/next/features/designly/panel/avatar';

export interface IOverviewPerson {
  name: string;
  role: string;
}

export interface IOverviewLink {
  key: string;
  name: string;
  meta: string;
  url: string;
}

export interface IOverviewTabProps {
  title: string;
  pages: number;
  /** The page on screen, by its name. */
  current: string | null;
  history: IProjectHistory | null;
  /** The design system the project is drawn with; null when none. */
  kit: { name: string; href: string | null; onOpen?: () => void } | null;
  /** The chats that changed the project: started from it, then the others that wrote its files. */
  chats: readonly string[];
  onOpenChat?: (id: string) => void;
  /** Who has access; null when the person may not see it. */
  people: readonly IOverviewPerson[] | null;
  /** «Только приглашённые», «Вся организация может смотреть»… */
  access: string;
  onShare?: () => void;
  links: readonly IOverviewLink[];
}

const Row = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <div className="hx-ds-row">
    <span className="hx-ds-row__label">{label}</span>
    <b className="hx-ds-row__value">{children}</b>
  </div>
);

const ChatLine = ({ id, onOpen }: { id: string; onOpen?: (id: string) => void }) => {
  const conv = useConversation(id as ConvId);
  const title = conv?.title || t('next.ds.overview.chatUntitled');
  return (
    <button type="button" className="hx-ds-line" onClick={() => onOpen?.(id)} disabled={!onOpen}>
      <span className="hx-ds-line__ic">
        <Icon name="message-square" size="sm" />
      </span>
      <span className="hx-ds-line__n">{title}</span>
      {conv && <span className="hx-ds-line__m">{whenOf(conv.lastActivityAt)}</span>}
    </button>
  );
};

const copy = (url: string) =>
  copyText(Promise.resolve(url)).then(
    () => toast(t('next.ds.copied')),
    (e: unknown) => toast(e instanceof Error ? e.message : t('next.site.copyFailed'), { tone: 'error' }),
  );

const OverviewTab = (p: IOverviewTabProps) => {
  const points = p.history?.points ?? [];
  const newest = points[0] ?? null;
  const oldest = points.length > 0 ? points[points.length - 1] : null;
  return (
    <div className="hx-ds__pane">
      <div className="hx-ds-ov__t">{p.title}</div>
      <div className="hx-ds-ov__s">
        {plural(p.pages, 'next.ds.overview.pages')}
        {p.current !== null && ` · ${t('next.ds.overview.current', { page: p.current })}`}
      </div>
      {oldest && (
        <Row label={t('next.ds.overview.created')}>
          {whenOf(oldest.at)} · {oldest.who}
        </Row>
      )}
      {newest && (
        <Row label={t('next.ds.overview.changed')}>
          {whenOf(newest.at)} · {newest.agent ? t('next.ds.overview.agent') : newest.who}
        </Row>
      )}
      <div className="hx-ds-row">
        <span className="hx-ds-row__label">{t('next.ds.overview.kit')}</span>
        {p.kit ? (
          p.kit.href ? (
            <a className="hx-ds-link" href={p.kit.href} target="_blank" rel="noreferrer">
              {p.kit.name}
              <Icon name="external-link" size="sm" />
            </a>
          ) : (
            <button type="button" className="hx-ds-link" onClick={p.kit.onOpen}>
              {p.kit.name}
              <Icon name="external-link" size="sm" />
            </button>
          )
        ) : (
          <b className="hx-ds-row__value">{t('next.ds.overview.noKit')}</b>
        )}
      </div>

      {p.chats.length > 0 && (
        <>
          <div className="hx-ds-h">{t('next.ds.overview.chats')}</div>
          {p.chats.map((id) => (
            <ChatLine key={id} id={id} onOpen={p.onOpenChat} />
          ))}
        </>
      )}

      <div className="hx-ds-h">
        {t('next.ds.overview.access')}
        {p.onShare && (
          <button type="button" className="hx-ds-h__act" onClick={p.onShare}>
            {t('next.ds.overview.share')}
          </button>
        )}
      </div>
      <div className="hx-ds-people">
        {(p.people ?? []).slice(0, 8).map((u) => (
          <Avatar key={u.name} name={u.name} title={`${u.name} · ${u.role}`} />
        ))}
        <span className="hx-ds-line__m hx-ds-people__note">{p.access}</span>
      </div>
      {p.links.map((l) => (
        <div key={l.key} className="hx-ds-line">
          <span className="hx-ds-line__ic">
            <Icon name="link" size="sm" />
          </span>
          <span className="hx-ds-line__n" title={l.url}>
            {l.name}
          </span>
          <span className="hx-ds-line__m">{l.meta}</span>
          <button
            type="button"
            className="hx-ds-icon-btn"
            aria-label={t('next.ds.copy')}
            title={t('next.ds.copy')}
            onClick={() => void copy(l.url)}
          >
            <Icon name="copy" size="sm" />
          </button>
        </div>
      ))}
    </div>
  );
};

export default OverviewTab;
