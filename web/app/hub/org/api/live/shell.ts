// The shell and the onboarding step on GET/POST /api/orgs, GET /api/orgs/{org} (docs/hub/orgs.md). The role is worked
// out per scope from `me.scopes`: the same person may run one tenant and be a member of another.
import type {
  AppsConsent,
  CapabilitiesResponse,
  Capabilities,
  Me,
  Org,
  OrgLinks,
  OrgList,
  OrgView,
  Scope as ApiScope,
} from '../../../../hub-api/orgs';
import type { Me as HubMe } from '../../../lib/types';
import { hubCache } from '../../../lib/cache';
import type { OnboardingApi } from '../onboarding';
import type { ShellApi } from '../shell';
import { OrgApiError } from '../errors';
import { ALL_SCOPE, type Role, type ShellData, type TeamRef, type Viewer } from '../../types';
import { http, orgPath } from './http';

export const capsOf = (me: Me, scope: ApiScope): Capabilities | undefined =>
  me.scopes[`${scope.kind}:${scope.id}`];

/** The scope a slug of the address means: the organization (`all`) or a tenant. */
export const resolveScope = (view: OrgView, slug: string): ApiScope => {
  if (slug === ALL_SCOPE) return { kind: 'org', id: view.org.id };
  const tenant = view.tenants.find((t) => t.slug === slug);
  if (!tenant) throw new OrgApiError('not_found', 'tenant not found');
  return { kind: 'tenant', id: tenant.id };
};

export const roleIn = (view: OrgView, scope: ApiScope): Role => {
  if (capsOf(view.me, scope)?.manage) return 'admin';
  const teams = view.teams.filter((t) => scope.kind === 'org' || t.tenantId === scope.id);
  return teams.some((t) => capsOf(view.me, { kind: 'team', id: t.id })?.manage) ? 'lead' : 'member';
};

/** The team a person runs or belongs to in the scope: the one they lead, else the primary, else any. */
export const viewerTeam = (view: OrgView, scope: ApiScope): string | null => {
  const here = view.teams.filter((t) => scope.kind === 'org' || t.tenantId === scope.id);
  const led = here.find((t) => t.leadIds.includes(view.me.userId));
  if (led) return led.id;
  const mine = view.me.teams.filter((m) => here.some((t) => t.id === m.teamId));
  return (mine.find((m) => m.isPrimary) ?? mine[0])?.teamId ?? null;
};

const ROLE_WORD = { admin: 'админ', lead: 'лид', member: 'участник' } as const;

/** Team leads have ids in the contract; their names come from the scope's people, which a member may not read. */
const leadNames = async (org: string, view: OrgView, scope: ApiScope): Promise<Map<string, string>> => {
  if (scope.kind === 'org' || !capsOf(view.me, scope)?.view) return new Map();
  const { members } = await http.get<{ members: { userId: string; name: string; email: string }[] }>(
    orgPath(org, `/scopes/${scope.kind}/${scope.id}/members`),
  );
  return new Map(members.map((m) => [m.userId, m.name || m.email]));
};

/** What the shell is made of, as the hub answered: the copy kept for the next load is this, not the result. */
interface ShellRaw {
  view: OrgView;
  list: OrgList;
  me: HubMe;
  names: [string, string][];
}

const shellKey = (org: string, tenant: string) => `shell/${org}/${tenant}`;

/** A copy that is not shaped like the answers it came from is no copy (a changed build, a hand-edited store). */
const isShellRaw = (x: unknown): x is ShellRaw => {
  const r = x as Partial<ShellRaw> | null;
  return (
    !!r &&
    typeof r.view?.org?.slug === 'string' &&
    Array.isArray(r.view.tenants) &&
    Array.isArray(r.view.teams) &&
    typeof r.view.me?.userId === 'string' &&
    !!r.view.me.scopes &&
    Array.isArray(r.list?.orgs) &&
    typeof r.me?.user === 'object' &&
    Array.isArray(r.names)
  );
};

export const liveShell = (remember: (view: OrgView, scope: ApiScope) => void): ShellApi & OnboardingApi => {
  // The same code makes the shell from a live answer and from a kept copy.
  const build = ({ view, list, me, names }: ShellRaw, scope: ApiScope): ShellData => {
    const role = roleIn(view, scope);
    const lead = new Map(names);
    const teams: TeamRef[] = view.teams
      .filter((t) => scope.kind === 'org' || t.tenantId === scope.id)
      .map((t) => {
        const first = t.leadIds[0];
        return { id: t.id, slug: t.slug, name: t.name, leadName: (first && lead.get(first)) || null };
      });
    const viewer: Viewer = {
      id: view.me.userId,
      name: me.user.name || me.user.email,
      role,
      teamId: viewerTeam(view, scope),
    };
    return {
      viewer,
      org: {
        id: view.org.id,
        slug: view.org.slug,
        name: view.org.name,
        memberMachines: view.org.memberMachines ?? true,
      },
      tenants: view.tenants.map((t) => ({ id: t.id, slug: t.slug, name: t.name })),
      teams,
      orgs: list.orgs.map((o) => ({ slug: o.slug, name: o.name, roleLabel: ROLE_WORD[o.role] })),
      features: view.features ?? [],
    };
  };
  return {
    cachedShell: (org, tenant) => {
      const key = shellKey(org, tenant);
      const raw = hubCache.get<unknown>(key);
      if (!isShellRaw(raw)) {
        if (raw !== null) hubCache.remove(key);
        return null;
      }
      try {
        const scope = resolveScope(raw.view, tenant);
        const data = build(raw, scope);
        remember(raw.view, scope);
        return data;
      } catch {
        hubCache.remove(key);
        return null;
      }
    },
    shell: async (org, tenant): Promise<ShellData> => {
      const [view, list, me] = await Promise.all([
        http.get<OrgView>(orgPath(org)),
        http.get<OrgList>('/api/orgs'),
        http.get<HubMe>('/api/me'),
      ]);
      const scope = resolveScope(view, tenant);
      remember(view, scope);
      const names =
        roleIn(view, scope) === 'member' ? new Map<string, string>() : await leadNames(org, view, scope);
      const raw: ShellRaw = { view, list, me, names: [...names] };
      hubCache.set(shellKey(org, tenant), raw);
      return build(raw, scope);
    },
    appsConsent: async (org) => {
      const view = await http.get<OrgView>(orgPath(org));
      return view.org.appsConsent ?? 'ask';
    },
    setAppsConsent: async (org, value: AppsConsent) => {
      const out = await http.patch<{ org: Org }>(orgPath(org), { appsConsent: value });
      return out.org.appsConsent ?? 'ask';
    },
    setMemberMachines: async (org, on) => {
      const out = await http.patch<{ org: Org }>(orgPath(org), { memberMachines: on });
      return out.org.memberMachines ?? true;
    },
    links: async (org) => {
      const view = await http.get<OrgView>(orgPath(org));
      return view.org.links ?? {};
    },
    setLinks: async (org, value: OrgLinks) => {
      const out = await http.patch<{ org: Org }>(orgPath(org), { links: value });
      return out.org.links ?? {};
    },
    capabilities: (org) => http.get<CapabilitiesResponse>(orgPath(org, '/capabilities')),
    setCapability: async (org, source, id, enabled) => {
      if (source === 'org') {
        await http.put<unknown>(orgPath(org, `/features/${encodeURIComponent(id)}`), { enabled });
        return;
      }
      await http.put<unknown>('/api/admin/switches', { feature: id, enabled });
    },
    leaveOrg: async (org) => {
      await http.post<unknown>(orgPath(org, '/leave'));
    },
    looseSpaces: async () => {
      const { looseSpaces } = await http.get<OrgList>('/api/orgs');
      return looseSpaces.map((s) => ({ id: s.id, slug: s.slug, name: s.name, owner: s.role === 'owner' }));
    },
    createOrg: async (input) => {
      const view = await http.post<OrgView>('/api/orgs', input);
      return { slug: view.org.slug };
    },
  };
};
