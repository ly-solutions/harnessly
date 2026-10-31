// The sidebar's designs at work on the open machine: the library as the last read left it at once, then read anew when
// the machine changes, the link comes back or the person goes to Sinopia Design (`fresh` changes), kept to the ones the
// live chats work on and the open one. null until the first read; a failed read keeps what is shown.
import { useEffect, useMemo, useState } from 'react';
import { fetchLibrary, keptLibrary, type IDesign } from '@/next/features/designly/api';
import { liveChats, workingDesigns, type IWorkingDesign } from '@/next/features/designly/working';
import { useLinkRestored } from '@/next/stores/connection';
import type { Conversation, MachineId } from '@/next/protocol/v2';

interface IRead {
  machine: MachineId;
  designs: readonly IDesign[];
  at: number;
}

export const useWorkingDesigns = (
  machine: MachineId | null,
  convs: readonly Conversation[] | null,
  current: string | null,
  fresh: unknown,
): readonly IWorkingDesign[] | null => {
  const [read, setRead] = useState<IRead | null>(null);
  // The day of the live chats counts back from the last read (or from the first look, while the kept library shows).
  const [shown] = useState(() => Date.now());
  const restored = useLinkRestored(machine);
  useEffect(() => {
    if (machine === null) return;
    const abort = new AbortController();
    fetchLibrary(machine, abort.signal).then(
      (library) => setRead({ machine, designs: library.designs, at: Date.now() }),
      () => undefined,
    );
    return () => abort.abort();
  }, [machine, restored, fresh]);
  const designs = designsOf(machine, read);
  const live = useMemo(() => liveChats(convs ?? [], read?.at ?? shown), [convs, read, shown]);
  return useMemo(
    () => (designs === null ? null : workingDesigns(designs, live, current)),
    [designs, live, current],
  );
};

/** This machine's own read, else what the last read of its library left, else nothing yet. */
const designsOf = (machine: MachineId | null, read: IRead | null): readonly IDesign[] | null => {
  if (machine === null) return null;
  if (read?.machine === machine) return read.designs;
  return keptLibrary(machine)?.designs ?? null;
};
