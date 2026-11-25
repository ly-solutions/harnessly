// Fixtures for the shell: the organization «Акме», its tenants and teams, and the three people who look at them.
import type { AppsConsent, CapabilitiesResponse, OrgLinks } from '../../../hub-api/orgs';
import type { ShellApi } from '../api/shell';
import { OrgApiError } from '../api/errors';
import type { OrgChoice, TeamRef, TenantRef, Viewer } from '../types';
import { later, type MockCtx } from './ctx';
import { renamedTenants } from './tenant';

export const TENANTS: TenantRef[] = [
  { id: 't-product', slug: 'product', name: 'Продукт', recent: true },
  { id: 't-platform', slug: 'platform', name: 'Платформа', recent: true },
  { id: 't-clients', slug: 'clients', name: 'Клиенты', recent: true },
  { id: 't-billing', slug: 'billing', name: 'Биллинг', pinned: true },
  { id: 't-scanner', slug: 'scanner', name: 'Сканер', pinned: true },
];

export const TEAMS: TeamRef[] = [
  { id: 'tm-front', slug: 'front', name: 'Фронт', leadName: 'Аня' },
  { id: 'tm-back', slug: 'back', name: 'Бэкенд', leadName: 'Дима' },
  { id: 'tm-design', slug: 'design', name: 'Дизайн', leadName: 'Лиза' },
];

const ORGS: OrgChoice[] = [
  { slug: 'acme', name: 'Акме', roleLabel: 'владелец' },
  { slug: 'harnessly', name: 'Харнесли', roleLabel: 'владелец' },
];

export const VIEWERS: Record<'admin' | 'lead' | 'member', Viewer> = {
  admin: { id: 'u-andrey', name: 'Андрей', role: 'admin', teamId: null },
  lead: { id: 'u-anya', name: 'Андрей', role: 'lead', teamId: 'tm-front' },
  member: { id: 'u-andrey-m', name: 'Андрей', role: 'member', teamId: 'tm-front' },
};

/** The mock organization has every feature on, so the gallery shows the whole new hub. */
export const MOCK_FEATURES = [
  'ui.hx',
  'env',
  'catalog',
  'machine.resources',
  'org.events',
  'ui.hx.live',
  'secrets.vault',
];

let mockAppsConsent: AppsConsent = 'ask';
let mockLinks: OrgLinks = {};
let mockMemberMachines = true;

// The gallery shows the screen «Возможности» as an administrator of the installation sees it: the toggles work.
let mockCapabilities: CapabilitiesResponse = {
  groups: ['Регулярные задачи и автоматизация', 'Расходы и лимиты', 'Окружение агентов'],
  canChange: true,
  capabilities: [
    {
      id: 'HUB_AUTOMATIONS',
      group: 'Регулярные задачи и автоматизация',
      name: 'Регулярные задачи и автоматизации',
      description:
        'Вы просите агента в чате: «каждое утро присылай дайджест», и хаб сам запускает такую задачу на вашей машине.',
      source: 'env',
      enabled: true,
      defaultOn: true,
      setBy: 'HUB_AUTOMATIONS',
      canToggle: false,
    },
    {
      id: 'automation-triggers',
      group: 'Регулярные задачи и автоматизация',
      name: 'Запуск по расписанию и по ожиданию',
      description: 'Регулярные задачи и правила срабатывают по времени.',
      source: 'rollout',
      enabled: true,
      defaultOn: true,
      needs: 'Включённые «Регулярные задачи и автоматизации».',
      canToggle: true,
    },
    {
      id: 'HUB_LIMITS_ENFORCE',
      group: 'Расходы и лимиты',
      name: 'Лимиты расходов на машинах',
      description: 'Что делают машины с лимитами.',
      source: 'env',
      enabled: true,
      detail: 'Только предупреждает',
      defaultOn: false,
      why: 'Может остановить людям работу, поэтому включается осознанно.',
      setBy: 'HUB_LIMITS_ENFORCE',
      canToggle: false,
    },
    {
      id: 'env',
      group: 'Окружение агентов',
      name: 'Окружение машин',
      description: 'Экран «Окружение»: что получают машины организации.',
      source: 'org',
      enabled: false,
      defaultOn: false,
      why: 'Меняет то, что ставится на машины людей, поэтому включается осознанно.',
      canToggle: true,
    },
  ],
};

export const shellMock = (ctx: MockCtx): ShellApi => ({
  capabilities: async () => {
    if (ctx.role !== 'admin') throw new OrgApiError('forbidden', '');
    return later(mockCapabilities);
  },
  setCapability: async (_org, _source, id, enabled) => {
    if (ctx.role !== 'admin') throw new OrgApiError('forbidden', '');
    mockCapabilities = {
      ...mockCapabilities,
      capabilities: mockCapabilities.capabilities.map((c) => (c.id === id ? { ...c, enabled } : c)),
    };
    return later(undefined);
  },
  appsConsent: async () => later(mockAppsConsent),
  setAppsConsent: async (_org, value) => {
    if (ctx.role !== 'admin') throw new OrgApiError('forbidden', '');
    mockAppsConsent = value;
    return later(mockAppsConsent);
  },
  setMemberMachines: async (_org, on) => {
    if (ctx.role !== 'admin') throw new OrgApiError('forbidden', '');
    mockMemberMachines = on;
    return later(mockMemberMachines);
  },
  links: async () => later(mockLinks),
  setLinks: async (_org, value) => {
    if (ctx.role !== 'admin') throw new OrgApiError('forbidden', '');
    mockLinks = value;
    return later(mockLinks);
  },
  leaveOrg: async () => later(undefined),
  shell: async (org, tenant) => {
    const known = ORGS.find((o) => o.slug === org);
    if (!known) throw new OrgApiError('not_found', 'organization not found');
    if (tenant !== 'all' && !TENANTS.some((t) => t.slug === tenant)) {
      throw new OrgApiError('not_found', 'tenant not found');
    }
    const viewer = VIEWERS[ctx.role];
    return later({
      viewer,
      org: {
        id: `org-${known.slug}`,
        slug: known.slug,
        name: known.name,
        memberMachines: mockMemberMachines,
      },
      tenants: (ctx.role === 'admin' ? TENANTS : TENANTS.slice(0, 1)).map((t) => ({
        ...t,
        name: renamedTenants.get(t.id) ?? t.name,
      })),
      teams: ctx.role === 'member' ? TEAMS.filter((t) => t.id === viewer.teamId) : TEAMS,
      orgs: ORGS,
      features: MOCK_FEATURES,
    });
  },
});
