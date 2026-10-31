// «Копировать» under a message: copies its source Markdown; the icon turns into a check for a moment.
import { useEffect, useRef, useState } from 'react';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

export const COPIED_MS = 1200;

const CopyButton = ({ text }: { text: string }) => {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(
    () => () => {
      if (timer.current !== null) clearTimeout(timer.current);
    },
    [],
  );
  const copy = () => {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      if (timer.current !== null) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), COPIED_MS);
    });
  };
  const label = t(copied ? 'next.message.copied' : 'next.message.copy');
  return (
    <button
      type="button"
      className="hx-turnacts__btn hx-tipped"
      data-copied={copied || undefined}
      aria-label={label}
      onClick={copy}
    >
      <span className="hx-turnacts__swap">
        <Icon name="copy" size="sm" />
        <Icon name="check" size="sm" />
      </span>
      <span className="hx-tip">{label}</span>
    </button>
  );
};

export default CopyButton;
