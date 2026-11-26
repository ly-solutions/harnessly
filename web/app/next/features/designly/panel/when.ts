// A moment as the design's panel says it: «сегодня, 14:20», «вчера, 18:02», else the date.
import { t } from '@/next/i18n';

const DAY_MS = 86_400_000;

const dayStart = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

export const whenOf = (at: number | string, now = Date.now()): string => {
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return '';
  const time = d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  const days = Math.round((dayStart(new Date(now)) - dayStart(d)) / DAY_MS);
  if (days === 0) return t('next.ds.today', { time });
  if (days === 1) return t('next.ds.yesterday', { time });
  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return d.toLocaleDateString(undefined, {
    day: 'numeric',
    month: 'long',
    ...(sameYear ? {} : { year: 'numeric' }),
  });
};

/** The two letters of a name for its round avatar. */
export const initialOf = (name: string): string => {
  const words = name
    .trim()
    .split(/[\s@._-]+/)
    .filter(Boolean);
  if (words.length === 0) return '?';
  return words[0].charAt(0).toUpperCase();
};

const AVATAR_COLORS = [
  '#8b7cff',
  '#2f9e6e',
  '#e2774a',
  '#3b82f6',
  '#d946ef',
  '#0ea5a4',
  '#f59e0b',
  '#ef4444',
];

/** A person's colour: the same name, the same colour. */
export const colorOf = (name: string): string => {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return AVATAR_COLORS[h % AVATAR_COLORS.length];
};
