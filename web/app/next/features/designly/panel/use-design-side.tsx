// Everything the design's side panel needs, put together once for the design's screen: the tab the toolbar
// opens, the board's settings taken from the page and kept for everybody, the people's threads and their layer over
// the page, the history for «Обзор» and «Версии». designly-site.tsx passes what it knows and puts the parts in place.
import { useMemo, useRef, useState, type ReactNode } from 'react';
import type { ConvId, MachineId } from '@/next/protocol/v2';
import type { ITweak, TTweakValue } from '@/next/features/designly/page-bridge';
import type { IDesignComments, IDesignPanel, TDesignTab } from '@/next/features/designly/design-panel';
import type { ITweaksPort } from '@/next/features/designly/site-view';
import type { IProjectAccess, IGuestLink } from '@/next/features/designly/access/api';
import { roleLabel } from '@/next/features/designly/access/labels';
import { useGuestLinks } from '@/next/features/designly/access/use-guest-links';
import { useHistory } from '@/next/features/designly/versions/versions-panel';
import { goTo, ROUTE, routePath } from '@/next/routes/routes';
import { useMe } from '@/next/stores/me';
import { t } from '@/next/i18n';
import DesignSide, { useDesignTools } from '@/next/features/designly/panel/design-side';
import SettingsTab from '@/next/features/designly/panel/settings-tab';
import OverviewTab, {
  type IOverviewLink,
  type IOverviewPerson,
} from '@/next/features/designly/panel/overview-tab';
import CommentsTab from '@/next/features/designly/panel/comments-tab';
import PagesTab, { type IPanelPage } from '@/next/features/designly/panel/pages-tab';
import VersionsTab from '@/next/features/designly/panel/versions-tab';
import CommentsLayer, {
  CommentsLayerInfo,
  type IMentionable,
} from '@/next/features/designly/panel/comments-layer';
import { SiteCommentsContext, useSiteComments } from '@/next/features/designly/panel/use-site-comments';
import { useSiteProps } from '@/next/features/designly/panel/use-site-props';
import { whenOf } from '@/next/features/designly/panel/when';

export interface IDesignSideOptions {
  machine: MachineId;
  id: string;
  title: string;
  /** The board on screen. */
  board: string | null;
  pages: readonly IPanelPage[];
  /** The chat the design is open beside or came from: @harnessly reaches its agent. */
  chat: ConvId | null;
  /** The chats that changed the project. */
  chats: readonly string[];
  /** The pages the agent of this chat changed. */
  changed: ReadonlySet<string>;
  access: IProjectAccess | null;
  /** The design's own address. */
  address: string | null;
  kit: { kit: string; kitTitle: string; theme: string; version: string } | null;
  origin: string | null;
  mayEdit: boolean;
  onPage: (path: string) => void;
  onShare?: () => void;
  onRestored: () => void;
}

const peopleOf = (access: IProjectAccess | null): { people: IOverviewPerson[]; mention: IMentionable[] } => {
  if (!access) return { people: [], mention: [] };
  const people: IOverviewPerson[] = [];
  const mention: IMentionable[] = [];
  if (access.owner) {
    const name = access.ownerName || access.owner;
    people.push({ name, role: roleLabel('owner') });
    mention.push({ name, email: access.owner });
  }
  for (const p of access.people ?? []) {
    const name = p.email.split('@')[0] ?? p.email;
    people.push({ name, role: roleLabel(p.role) });
    mention.push({ name, email: p.email });
  }
  return { people, mention };
};

const accessLine = (access: IProjectAccess | null): string => {
  if (!access) return '';
  if (access.team === 'none') return t('next.ds.overview.invited');
  return t(`next.ds.overview.team.${access.team}`);
};

const linksOf = (address: string | null, guests: readonly IGuestLink[]): IOverviewLink[] => {
  const out: IOverviewLink[] = [];
  if (address)
    out.push({
      key: 'own',
      name: t('next.ds.overview.address'),
      meta: t('next.ds.overview.byAccess'),
      url: address,
    });
  for (const g of guests) {
    if (g.revokedAt !== null) continue;
    out.push({
      key: g.id,
      name: g.title || t('next.ds.overview.guestLink'),
      meta: g.expiresAt
        ? t('next.ds.overview.until', { when: whenOf(g.expiresAt) })
        : t('next.ds.overview.forever'),
      url: g.url,
    });
  }
  return out;
};

export const useDesignSide = (o: IDesignSideOptions) => {
  const [tab, setTab] = useState<TDesignTab | null>(null);
  const tools = useDesignTools();
  const panel: IDesignPanel = {
    tab,
    open: (next) => setTab((cur) => (cur === next ? null : next)),
  };

  // The board's settings: the page's offer, the way to send it a change.
  const [controls, setControls] = useState<ITweak[] | null>(null);
  const send = useRef<((name: string, value: TTweakValue) => void) | null>(null);
  const tweaksPort: ITweaksPort = useMemo(
    () => ({
      onOffer: setControls,
      onSend: (fn) => {
        send.current = fn;
      },
    }),
    [],
  );
  const props = useSiteProps(o.machine, o.id, o.board, controls, (name, value) =>
    send.current?.(name, value),
  );

  const comments = useSiteComments(o.machine, o.id, o.chat);
  const commentMode: IDesignComments = {
    on: comments.placing,
    toggle: () => comments.setPlacing(!comments.placing),
  };
  const history = useHistory(o.machine, o.id, tab !== null && tab !== 'tools');
  const guests = useGuestLinks(o.access?.canManage ? o.id : null);
  const me = useMe();
  const myName = me.k === 'ready' ? (me.me.name ?? me.me.email ?? '') : '';
  const { people, mention } = peopleOf(o.access);
  const linkOf = (thread: string) => {
    const url = new URL(o.address ?? window.location.href, window.location.href);
    url.searchParams.set('comment', thread);
    return url.toString();
  };
  const openThreads = comments.threads.filter((th) => !th.root.resolved).length;
  const boardName = (path: string) => o.pages.find((p) => p.path === path)?.name ?? path;

  const render = (at: Exclude<TDesignTab, 'tools'>): ReactNode => {
    switch (at) {
      case 'info':
        return (
          <OverviewTab
            title={o.title}
            pages={o.pages.length}
            current={o.board ? boardName(o.board) : null}
            history={history.history}
            kit={
              o.kit
                ? {
                    name: o.kit.version ? `${o.kit.kitTitle} ${o.kit.version}` : o.kit.kitTitle,
                    href: routePath({
                      kind: ROUTE.kit,
                      id: o.kit.kit,
                      theme: o.kit.theme,
                      ...(o.kit.version ? { version: o.kit.version } : {}),
                    }),
                  }
                : null
            }
            chats={o.chats}
            onOpenChat={(id) => goTo({ kind: ROUTE.chat, id })}
            people={people}
            access={accessLine(o.access)}
            onShare={o.onShare}
            links={linksOf(o.address, guests.state.k === 'ready' ? guests.state.links : [])}
          />
        );
      case 'tweaks':
        return <SettingsTab controls={controls} onChange={props.change} onReset={props.reset} />;
      case 'comments':
        return (
          <CommentsTab
            threads={comments.threads}
            boardName={boardName}
            current={o.board}
            onOpen={(th) => {
              if (th.root.board !== o.board) o.onPage(th.root.board);
              comments.setOpen(th.root.id);
            }}
            onStart={() => comments.setPlacing(true)}
          />
        );
      case 'pages':
        return <PagesTab pages={o.pages} current={o.board} changed={o.changed} onPick={o.onPage} />;
      case 'versions':
        return (
          <VersionsTab
            machine={o.machine}
            id={o.id}
            origin={o.origin}
            page={o.board}
            mayEdit={o.mayEdit}
            history={history.history}
            error={history.error}
            reload={history.reload}
            onRestored={o.onRestored}
          />
        );
    }
  };

  const side = (
    <DesignSide
      tab={tab}
      onTab={setTab}
      onClose={() => setTab(null)}
      tools={tools}
      badges={{ comments: openThreads }}
      render={render}
    />
  );
  const layerInfo = { board: o.board ?? '', people: mention, me: myName, linkOf };
  const wrap = (node: ReactNode) => (
    <SiteCommentsContext value={comments}>
      <CommentsLayerInfo value={layerInfo}>{node}</CommentsLayerInfo>
    </SiteCommentsContext>
  );
  return { panel, tweaksPort, side, commentsLayer: CommentsLayer, commentMode, wrap };
};
