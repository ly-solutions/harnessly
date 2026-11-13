// A vendor of the «Модели» page: its name, one line of what is connected, and on opening the presets with the key form.
// The card follows the «Модели и ключи» cards (.hx-acct) so the two pages read as one family.
import LimitsLine from '@/next/features/limits/limits-line';
import { profileLimitsOf, type MachineLimits } from '@/next/features/limits/model';
import { useState } from 'react';
import {
  defaultPreset,
  presetName,
  presetsOfVendor,
  type IPresetActions,
  type IPresetState,
  type PresetId,
  type Vendor,
} from '@/next/features/providers/model';
import PresetPanel from '@/next/features/providers/preset-panel';
import { t } from '@/next/i18n';
import '@/next/styles/providers.css';
import Icon from '@/next/ui/icon';
import Segmented from '@/next/ui/segmented';

/** What the card says under the vendor's name. */
export const vendorStatus = (vendor: Vendor, states: readonly IPresetState[]): string => {
  const own = presetsOfVendor(vendor, states);
  const first = own.find((s) => s.key.configured);
  if (!first) {
    // No key, but the vendor's CLI is signed in: the profile runs on that subscription.
    const signed = own.find((s) => s.login !== undefined);
    if (!signed?.login) return t('next.providers.off');
    const account = signed.login.account;
    if (own.length === 1) return t('next.providers.onLogin', { account });
    return t('next.providers.onLoginPreset', { preset: presetName(signed), account });
  }
  const tail = first.key.tail ?? '';
  if (own.length === 1) return t('next.providers.on', { tail });
  return t('next.providers.onPreset', { preset: presetName(first), tail });
};

interface IVendorCardProps extends IPresetActions {
  vendor: Vendor;
  states: readonly IPresetState[];
  open: boolean;
  /** The machine's limits; the card's head shows what is left of the vendor's first account that has any. */
  limits?: MachineLimits | null;
  onToggle: (vendor: Vendor) => void;
}

const VendorCard = ({ vendor, states, open, limits, onToggle, ...actions }: IVendorCardProps) => {
  const presets = presetsOfVendor(vendor, states);
  const left = limits
    ? (presets.map((p) => profileLimitsOf(limits, p.id)).find((entry) => entry !== null) ?? null)
    : null;
  const [picked, setPicked] = useState<PresetId | null>(() => defaultPreset(vendor, states));
  const current = presets.find((p) => p.id === picked) ?? presets[0];
  const name = t(`next.providers.vendor.${vendor}`);
  return (
    <div
      className="hx-acct hx-prov"
      data-open={open}
      data-on={presets.some((p) => p.key.configured || p.login !== undefined)}
    >
      <button type="button" className="hx-acct__head" aria-expanded={open} onClick={() => onToggle(vendor)}>
        <span className="hx-acct__logo">
          <i className="hx-agent" data-agent={vendor} aria-hidden="true" />
        </span>
        <span className="hx-acct__text">
          <span className="hx-acct__name">{name}</span>
          <span className="hx-acct__status">{vendorStatus(vendor, states)}</span>
        </span>
        <LimitsLine limits={left} />
        <Icon name="chevron-down" size="sm" className="hx-acct__chev" />
      </button>
      <div className="hx-acct__body">
        <div className="hx-acct__inner" inert={!open || undefined}>
          <div className="hx-prov__body">
            {presets.length > 1 && (
              <Segmented
                label={t('next.providers.presetAria')}
                value={current.id}
                options={presets.map((p) => ({ value: p.id, label: presetName(p) }))}
                onChange={setPicked}
              />
            )}
            <PresetPanel key={current.id} state={current} vendor={name} {...actions} />
          </div>
        </div>
      </div>
    </div>
  );
};

export default VendorCard;
