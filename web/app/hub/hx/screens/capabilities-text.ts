// The words of the «Возможности» screen that depend on a row: its state and where it is decided.
import type { CapabilityItem } from '../../../hub-api/orgs';

/** What stands in place of the toggle when the row cannot be switched here. */
export const stateOf = (item: CapabilityItem): string =>
  item.enabled ? (item.detail ?? 'Включено') : 'Выключено';

/** Where the row is decided and who changes it, in one sentence. */
export const whereOf = (item: CapabilityItem): string => {
  const byDefault = item.defaultOn ? 'По умолчанию включено.' : 'По умолчанию выключено.';
  if (item.source === 'env' || item.forced) {
    return `Задано в настройках хаба (${item.setBy ?? item.id}), здесь не меняется. ${byDefault}`;
  }
  if (item.source === 'org') {
    return item.canToggle
      ? `Включается для организации прямо здесь. ${byDefault}`
      : `Включается для организации; переключает администратор установки. ${byDefault}`;
  }
  return item.canToggle
    ? `Включается для всех пользователей хаба прямо здесь. ${byDefault}`
    : `Действует для всех пользователей хаба; переключает администратор установки. ${byDefault}`;
};
