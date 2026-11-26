// A person's round avatar: the first letter on their colour (the same name, the same colour).
import { useLayoutEffect, useRef } from 'react';
import { colorOf, initialOf } from '@/next/features/designly/panel/when';

const Avatar = ({ name, title, className }: { name: string; title?: string; className?: string }) => {
  const ref = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    ref.current?.style.setProperty('--c', colorOf(name));
  }, [name]);
  return (
    <span ref={ref} className={className ?? 'hx-ds-av'} title={title} aria-label={title ?? name}>
      {initialOf(name)}
    </span>
  );
};

export default Avatar;
