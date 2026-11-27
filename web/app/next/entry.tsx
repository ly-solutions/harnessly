// The new interface's lazy root: it boots v2 (boot.ts: cache → me → machines → sockets) and draws the shell on
// its stores. main.tsx loads this chunk only for a person who chose the new interface; when the person goes (another
// account, sign-out) the boot lets everything of theirs go.

import '@/next/styles/order';

import { useEffect } from 'react';
import { offlineBase } from '@/next/offline/client';
import { netStore, startNet } from '@/next/offline/net';
import { openThisComputer } from '@/next/offline/this-computer';
import Shell, { type IShellProps } from '@/next/bindings/shell';
import { bootNext, shutdownNext } from '@/next/boot';
import { installPhoneFocusGuard } from '@/next/ui/phone-focus-guard';

/** How long after the page opens a found «offline» still counts as opening it offline. */
const COLD_MS = 8000;

const NextEntry = (props: IShellProps) => {
  // Booted once per mount: the owner may arrive after the first draw (a home machine slow to give the start data), and
  // the boot finds who is signed in by itself (me), so a late owner is no other person.
  useEffect(() => {
    void bootNext();
    if (import.meta.env.PROD && 'serviceWorker' in navigator)
      void navigator.serviceWorker
        .register(offlineBase() + 'sw.js', { scope: offlineBase() })
        .catch(() => undefined);
    return () => void shutdownNext();
  }, []);
  // Whether the device reaches the server: offline the page draws what it kept and says so.
  useEffect(() => {
    const stop = startNet();
    // Opened without a network on a computer that runs a machine: the app goes on there, on the loopback.
    const opened = Date.now();
    const cold = netStore.subscribe(() => {
      if (!netStore.get().online && Date.now() - opened < COLD_MS) openThisComputer();
    });
    const done = setTimeout(cold, COLD_MS);
    return () => {
      stop();
      cold();
      clearTimeout(done);
    };
  }, []);
  // A sheet on the phone never raises the keyboard by itself: only a tap on its field does.
  useEffect(() => installPhoneFocusGuard(), []);
  return <Shell {...props} />;
};

export default NextEntry;
