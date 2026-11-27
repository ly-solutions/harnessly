// Everything the design's side panel needs, put together once for the design's screen: the tab the toolbar
// opens, the board's settings taken from the page and kept for everybody, the people's threads and their layer over
// the page, the history for «Обзор» and «Версии». designly-site.tsx passes what it knows and puts the parts in place.
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import type { ConvId, MachineId } from '@/next/protocol/v2';
import type { ITweak, TTweakValue } from '@/next/features/designly/page-bridge';
import type { IDesignComments, IDesignPanel, TDesignTab } from '@/next/features/designly/design-panel';
import {
  askLayout,
  offerLayout,
  setLayoutPart,
  useLayoutEntry,
  type TLayoutPart,
} from '@/next/features/designly/panel/layout-tab';
import type { ITweaksPort } from '@/next/features/designly/site-view';
import type { IProjectAccess, IGuestLink } from '@/next/features/designly/access/api';
import { roleLabel } from '@/next/features/designly/access/labels';
import { useGuestLinks } from '@/next/features/designly/access/use-guest-links';
import { useHistory } from '@/next/features/designly/versions/versions-panel';
import type { IVersionsVia } from '@/next/features/designly/versions/versions-api';
import { goTo, ROUTE, routePath } from '@/next/routes/routes';
import { useMe } from '@/next/stores/me';
import { t } from '@/next/i18n';
import LayoutBody from '@/next/features/designly/panel/design-side';
import SettingsTab from '@/next/features/designly/panel/settings-tab';
import OverviewTab, {
  type IOverviewLink,
  type IOverviewPerson,
} from '@/next/features/designly/panel/overview-tab';
import CommentsTab from '@/next/features/designly/panel/comments-tab';
import PagesTab, { type IPageActions, type IPanelPage } from '@/next/features/designly/panel/pages-tab';
import VersionsTab from '@/next/features/designly/panel/versions-tab';
import CommentsLayer, {
  CommentsLayerInfo,
  type IMentionable,
} from '@/next/features/designly/panel/comments-layer';
import { SiteCommentsContext, useSiteComments } from '@/next/features/designly/panel/use-site-comments';
import { useSiteProps } from '@/next/features/designly/panel/use-site-props';
import { whenOf } from '@/next/features/designly/panel/when';
import { changeSitePages } from '@/next/features/designly/panel/api';
import { toast } from '@/next/features/designly/sdk/toast';

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
  /** Where the versions are read from: the hub's store while the page shows its copy. */
  versions?: IVersionsVia;
  mayEdit: boolean;
  onPage: (path: string) => void;
  onShare?: () => void;
  onRestored: () => void;
  /** The chat's tools panel beside the design; null where the design itself stands in that panel. */
  toolPane: { open: boolean; onToggle: () => void } | null;
  /** The person who made the project, for «Создан» and the edits made on this machine. */
  owner: string | null;
  /** The pages were made, renamed or removed: the project is read again. */
  onPagesChanged?: () => void;
  /** A page's own address, for «Открыть в новой вкладке». */
  hrefOf?: (path: string) => string;
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
  // «Макет» is a tab of the chat's tools panel: the design offers it while it is open beside the chat.
  const conv = o.chat;
  useEffect(() => (conv ? offerLayout(conv) : undefined), [conv]);
  const entry = useLayoutEntry(conv);
  const paneOpen = o.toolPane ? o.toolPane.open : entry !== null && entry.shows !== 'shut';
  const tab: TDesignTab | null =
    entry === null || !paneOpen
      ? null
      : entry.shows === 'layout'
        ? entry.part
        : entry.shows === 'tools'
          ? 'tools'
          : null;
  const panel: IDesignPanel = {
    tab,
    open: (next) => {
      if (conv === null) return;
      // The panel's own button opens and closes it on the tab it had: «Макет» first while the design is shown.
      if (next === 'tools' && o.toolPane) {
        o.toolPane.onToggle();
        return;
      }
      if (next === tab) {
        if (o.toolPane?.open) o.toolPane.onToggle();
        return;
      }
      if (o.toolPane && !o.toolPane.open) o.toolPane.onToggle();
      if (next === 'tools') askLayout(conv, 'tools');
      else askLayout(conv, 'layout', next);
    },
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
  const history = useHistory(o.machine, o.id, entry?.slot != null, o.versions);
  const guests = useGuestLinks(o.access?.canManage ? o.id : null);
  const me = useMe();
  const myName = me.k === 'ready' ? (me.me.name ?? me.me.email ?? '') : '';
  const { people, mention } = peopleOf(o.access);
  const linkOf = (thread: string) => {
    const url = new URL(o.address ?? window.location.href, window.location.href);
    url.searchParams.set('comment', thread);
    return url.toString();
  };
  // The owner is the gateway's (who shares the project), not the name a copy of it carries on a machine.
  const owner =
    o.access?.ownerName || (o.access?.owner ? o.access.owner.split('@')[0] : '') || o.owner || null;
  const nameOf = (email: string): string | null => (o.access?.owner?.toLowerCase() === email ? owner : null);
  const openThreads = comments.threads.filter((th) => !th.root.resolved).length;
  const boardName = (path: string) => o.pages.find((p) => p.path === path)?.name ?? path;

  // What «Страницы» does with the pages, for whoever may edit: the machine does it, the project is read again.
  const pageActions: IPageActions | undefined = o.mayEdit
    ? {
        create: async () => {
          const file = await changeSitePages(o.machine, o.id, {
            action: 'create',
            title: t('next.ds.pages.newTitle'),
          });
          o.onPagesChanged?.();
          if (file) o.onPage(file);
        },
        duplicate: async (page) => {
          const file = await changeSitePages(o.machine, o.id, { action: 'duplicate', page });
          o.onPagesChanged?.();
          if (file) o.onPage(file);
        },
        rename: async (page, title) => {
          await changeSitePages(o.machine, o.id, { action: 'rename', page, title });
          o.onPagesChanged?.();
        },
        remove: async (page) => {
          if (page === o.board) {
            const at = o.pages.findIndex((p) => p.path === page);
            const next = o.pages[at + 1] ?? o.pages[at - 1];
            if (next) o.onPage(next.path);
          }
          await changeSitePages(o.machine, o.id, { action: 'remove', page });
          o.onPagesChanged?.();
        },
        failed: (error) =>
          toast(
            t('next.ds.pages.failed', { error: error instanceof Error ? error.message : String(error) }),
            {
              tone: 'error',
            },
          ),
      }
    : undefined;

  const render = (at: TLayoutPart): ReactNode => {
    switch (at) {
      case 'info':
        return (
          <OverviewTab
            title={o.title}
            pages={o.pages.length}
            current={o.board ? boardName(o.board) : null}
            history={history.history}
            owner={owner}
            nameOf={nameOf}
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
        return (
          <PagesTab
            pages={o.pages}
            current={o.board}
            changed={o.changed}
            onPick={o.onPage}
            hrefOf={o.hrefOf}
            actions={pageActions}
          />
        );
      case 'versions':
        return (
          <VersionsTab
            machine={o.machine}
            id={o.id}
            origin={o.origin}
            page={o.board}
            mayEdit={o.mayEdit}
            history={history.history}
            owner={owner}
            nameOf={nameOf}
            error={history.error}
            reload={history.reload}
            onRestored={o.onRestored}
            via={o.versions}
          />
        );
    }
  };

  const part: TLayoutPart = entry?.part ?? 'info';
  const layoutBody = (
    <LayoutBody
      part={part}
      onPart={(next) => conv && setLayoutPart(conv, next)}
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
  // The content goes into the place the panel gives it, keeping the design's state and contexts.
  const portal = entry?.slot ? createPortal(wrap(layoutBody), entry.slot) : null;
  return { panel, tweaksPort, portal, commentsLayer: CommentsLayer, commentMode, wrap };
};
