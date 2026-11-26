import { appConfig } from '@/platform/config';
import { ROUTE, routePath } from '@/next/routes/routes';

/** An address for a reader. The project, the selected board and, for a copy another space shared (GitHub #186), its
 *  card key travel with it: without the key the hub looks for the project in the reader's own space and finds nothing. */
export const designAddress = (id: string, page: string | null, card?: string) => {
  const { basePath, hubUrl } = appConfig();
  const machineBound = /^\/m\/[^/]+(?:\/|$)/.test(basePath);
  const base = machineBound ? '/agents' : basePath;
  return new URL(
    base.replace(/\/$/, '') +
      routePath({ kind: ROUTE.site, id, ...(page ? { page } : {}), ...(card ? { card } : {}) }),
    machineBound && hubUrl ? hubUrl : window.location.origin,
  ).href;
};
