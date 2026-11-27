// The refusal of an action in the access and sharing screens. It used to be a line at the bottom of a body
// that scrolls, under a form taller than the window, so the owner pressed «Create link» and saw nothing happen
// (GitHub #190). Every such screen says its refusal through this one line: it is announced, and brought into view the
// moment it appears or changes.
import { useEffect, useRef } from 'react';

interface IProps {
  message: string | null | undefined;
  className?: string;
}

const ActionError = ({ message, className = 'hx-create__err' }: IProps) => {
  const ref = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (message) ref.current?.scrollIntoView?.({ block: 'nearest' });
  }, [message]);
  if (!message) return null;
  return (
    <p ref={ref} className={className} role="alert">
      {message}
    </p>
  );
};

export default ActionError;
