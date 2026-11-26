// «Макет → Комментарии»: the people's threads on the boards, the newest first, the resolved ones faded; a
// press opens the thread on its board, and «Оставить комментарий на макете» turns the comment mode on.
import Icon from '@/next/ui/icon';
import { t, plural } from '@/next/i18n';
import type { IThread } from '@/next/features/designly/panel/api';
import Avatar from '@/next/features/designly/panel/avatar';
import { whenOf } from '@/next/features/designly/panel/when';

export interface ICommentsTabProps {
  threads: readonly IThread[];
  /** The board's name by its path, for a thread on another board. */
  boardName: (path: string) => string;
  current: string | null;
  onOpen: (thread: IThread) => void;
  onStart: () => void;
}

export const authorOf = (c: IThread['root']) => (c.byAgent ? 'harnessly' : c.by.name || c.by.email);

const CommentsTab = ({ threads, boardName, current, onOpen, onStart }: ICommentsTabProps) => (
  <div className="hx-ds__pane">
    {threads.length === 0 && <div className="hx-ds__empty">{t('next.ds.comments.none')}</div>}
    {threads.map((th) => (
      <button
        key={th.root.id}
        type="button"
        className="hx-ds-thread"
        data-resolved={th.root.resolved}
        onClick={() => onOpen(th)}
      >
        <Avatar name={authorOf(th.root)} />
        <div className="hx-ds-thread__main">
          <div className="hx-ds-thread__h">
            <b>{authorOf(th.root)}</b>
            <span>{whenOf(th.root.at)}</span>
            {th.root.board !== current && <span>· {boardName(th.root.board)}</span>}
          </div>
          <div className="hx-ds-thread__x">{th.root.text}</div>
          {th.replies.length > 0 && (
            <div className="hx-ds__muted">{plural(th.replies.length, 'next.ds.comments.replies')}</div>
          )}
        </div>
      </button>
    ))}
    <button type="button" className="hx-ds-plink" onClick={onStart}>
      <Icon name="message-square-plus" size="sm" />
      <span>{t('next.ds.comments.start')}</span>
    </button>
  </div>
);

export default CommentsTab;
