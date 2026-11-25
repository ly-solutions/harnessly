import type { AppsConsent, CapabilitiesResponse, CapabilitySource, OrgLinks } from '../../../hub-api/orgs';
import type { ShellData } from '../types';

export interface ShellApi {
  /** The person, the organization, the tenants and the teams the shell draws the menu from. */
  shell(org: string, tenant: string): Promise<ShellData>;
  /**
   * The shell as it was last read, made by the same code as a live one (and the api then answers the screens' calls
   * from it); null when nothing valid is kept or the api keeps nothing. The live `shell` follows behind it.
   */
  cachedShell?(org: string, tenant: string): ShellData | null;
  /** The organization's rule for apps on its machines. */
  appsConsent(org: string): Promise<AppsConsent>;
  /** Sets the rule; only the organization's admin may. Resolves with the rule the hub kept. */
  setAppsConsent(org: string, value: AppsConsent): Promise<AppsConsent>;
  /** Turns members' own machines on or off; only the organization's admin may. Resolves with what the hub kept. */
  setMemberMachines(org: string, on: boolean): Promise<boolean>;
  /** The organization's integrations for the chat's links. */
  links(org: string): Promise<OrgLinks>;
  /** Replaces them (an empty object clears); only the organization's admin may. Resolves with what the hub kept. */
  setLinks(org: string, value: OrgLinks): Promise<OrgLinks>;
  /** The screen «Возможности»: every part of the hub that is on or off, and where each is set (the organization's admin). */
  capabilities(org: string): Promise<CapabilitiesResponse>;
  /**
   * Switches one of them: a flag of the organization or a switch of the installation. Only an administrator of the
   * installation may; the hub says no to anyone else.
   */
  setCapability(
    org: string,
    source: Exclude<CapabilitySource, 'env'>,
    id: string,
    enabled: boolean,
  ): Promise<void>;
  /** Takes the viewer out of the organization: its tenants, teams and role; their machines there are revoked. */
  leaveOrg(org: string): Promise<void>;
}
