// The service blocks of a person's message (features/chat/markup.ts) as the chat draws them: a quiet card each, with an
// icon and a line that says what it is; a long body folds and opens in place, a command or an output copies.
import { useState, type ReactNode } from 'react';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';
import CopyButton from '@/next/features/chat/copy-button';
import { MARKUP, TASK_STATE, type TMarkupBlock, type TTaskState } from '@/next/features/chat/markup';

type TTone = 'quiet' | 'error' | 'ok';

const TASK_ICON: Record<TTaskState, string> = {
  [TASK_STATE.done]: 'circle-check',
  [TASK_STATE.failed]: 'circle-alert',
  [TASK_STATE.running]: 'loader-circle',
};

const TASK_TONE: Record<TTaskState, TTone> = {
  [TASK_STATE.done]: 'ok',
  [TASK_STATE.failed]: 'error',
  [TASK_STATE.running]: 'quiet',
};

const TASK_LABEL: Record<TTaskState, string> = {
  [TASK_STATE.done]: 'next.markup.taskDone',
  [TASK_STATE.failed]: 'next.markup.taskFailed',
  [TASK_STATE.running]: 'next.markup.task',
};

/** A body longer than this many lines or characters folds behind «Показать целиком». */
const FOLD_LINES = 4;
const FOLD_CHARS = 320;

interface ICardProps {
  kind: string;
  icon: string;
  label: string;
  /** What the head says after the label, on its line. */
  title?: string;
  tone?: TTone;
  /** The text the copy button takes; none, no button. */
  copy?: string;
  /** The body under the head, folded when long; none for a card that is only its head. */
  body?: string;
  mono?: boolean;
  children?: ReactNode;
}

const Card = ({
  kind,
  icon,
  label,
  title,
  tone = 'quiet',
  copy,
  body,
  mono = false,
  children,
}: ICardProps) => {
  const [open, setOpen] = useState(false);
  const long = body !== undefined && (body.split('\n').length > FOLD_LINES || body.length > FOLD_CHARS);
  return (
    <div className="hx-mk" data-kind={kind} data-tone={tone} data-open={open || !long || undefined}>
      <div className="hx-mk__head">
        <Icon name={icon} size="sm" className="hx-mk__icon" />
        <span className="hx-mk__label">{label}</span>
        {title && <span className="hx-mk__title">{title}</span>}
        {copy && (
          <span className="hx-mk__copy">
            <CopyButton text={copy} />
          </span>
        )}
      </div>
      {body && (
        <div className="hx-mk__fold">
          <div className="hx-mk__body" data-mono={mono || undefined}>
            {body}
          </div>
        </div>
      )}
      {children}
      {long && (
        <button
          type="button"
          className="hx-mk__more"
          aria-expanded={open}
          onClick={() => setOpen((value) => !value)}
        >
          <Icon name="chevron-down" size="sm" className="hx-mk__chev" />
          {open ? t('next.message.pastedLess') : t('next.message.pastedMore')}
        </button>
      )}
    </div>
  );
};

export const MarkupBlock = ({ block }: { block: TMarkupBlock }) => {
  switch (block.kind) {
    case MARKUP.reminder:
      return <Card kind={block.kind} icon="info" label={t('next.markup.reminder')} body={block.body} />;
    case MARKUP.task:
      return (
        <Card
          kind={block.kind}
          icon={TASK_ICON[block.state]}
          label={t(TASK_LABEL[block.state])}
          title={block.summary}
          tone={TASK_TONE[block.state]}
          body={block.result || undefined}
          copy={block.output || undefined}
        >
          {block.output && <div className="hx-mk__path">{block.output}</div>}
        </Card>
      );
    case MARKUP.command:
      return (
        <Card
          kind={block.kind}
          icon="square-slash"
          label={block.name || t('next.markup.command')}
          title={block.args}
          copy={[block.name, block.args].filter(Boolean).join(' ')}
        />
      );
    case MARKUP.shell:
      return (
        <Card kind={block.kind} icon="terminal" label="$" title={block.command} copy={block.command} mono />
      );
    case MARKUP.output:
      return (
        <Card
          kind={block.kind}
          icon={block.error ? 'circle-alert' : 'square-terminal'}
          label={t(block.error ? 'next.markup.stderr' : 'next.markup.stdout')}
          tone={block.error ? 'error' : 'quiet'}
          body={block.body}
          copy={block.body}
          mono
        />
      );
    case MARKUP.teammate:
      return (
        <Card
          kind={block.kind}
          icon="users"
          label={block.from ? t('next.markup.teammateFrom', { name: block.from }) : t('next.markup.teammate')}
          title={block.summary}
          body={block.body}
          copy={block.body}
        />
      );
    case MARKUP.hook:
      return <Card kind={block.kind} icon="zap" label={t('next.markup.hook')} body={block.body} />;
    case MARKUP.ide:
      return (
        <Card
          kind={block.kind}
          icon={block.selection ? 'text-quote' : 'file-code'}
          label={t(block.selection ? 'next.markup.ideSelection' : 'next.markup.ideFile')}
          body={block.body}
          copy={block.body}
          mono={block.selection}
        />
      );
  }
};
