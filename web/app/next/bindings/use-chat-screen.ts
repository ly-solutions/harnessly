// A chat remembers its full screen (stores/chat-screen.ts): a screen opened from a chat is kept as that chat's; the chat
// itself, opened again, puts the screen back — unless the person has just come from that screen to the chat, which is
// leaving it. The screen may arrive after the chat opened (the view from another device, stores/chat-view.ts): then it
// is put back the same way.
import { useEffect, useRef } from 'react';
import { forgetScreen, keepScreen, keptScreen, screenOf } from '@/next/stores/chat-screen';
import { onViewApplied } from '@/next/stores/chat-view';
import { goTo, ROUTE, type TRoute } from '@/next/routes/routes';

export const useChatScreen = (route: TRoute) => {
  const from = useRef<TRoute | null>(null);
  /** The chat the person has just left a screen for: its screen stays forgotten while they are in it. */
  const left = useRef<string | null>(null);
  const now = useRef(route);
  useEffect(() => {
    now.current = route;
  }, [route]);
  useEffect(() => {
    const was = from.current;
    from.current = route;
    const screen = screenOf(route);
    if (screen) {
      left.current = null;
      keepScreen(screen.conv, screen.path);
      return;
    }
    if (route.kind !== ROUTE.chat) {
      left.current = null;
      return;
    }
    if (was !== null && screenOf(was)?.conv === route.id) {
      left.current = route.id;
      forgetScreen(route.id);
      return;
    }
    const kept = keptScreen(route.id);
    if (kept) goTo(kept, { replace: true });
  }, [route]);
  useEffect(
    () =>
      onViewApplied(() => {
        const at = now.current;
        if (at.kind !== ROUTE.chat || left.current === at.id) return;
        const kept = keptScreen(at.id);
        if (kept) goTo(kept, { replace: true });
      }),
    [],
  );
};
