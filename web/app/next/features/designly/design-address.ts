import { appConfig } from '@/platform/config';
import { ROUTE, routePath } from '@/next/routes/routes';

/** An address for a reader. Only the project and the selected board travel with it. */
export const designAddress = (id: string, page: string | null) => {
  const { basePath, hubUrl } = appConfig();
  const machineBound = /^\/m\/[^/]+(?:\/|$)/.test(basePath);
  const base = machineBound ? '/agents' : basePath;
  return new URL(
    base.replace(/\/$/, '') + routePath({ kind: ROUTE.site, id, ...(page ? { page } : {}) }),
    machineBound && hubUrl ? hubUrl : window.location.origin,
  ).href;
};
