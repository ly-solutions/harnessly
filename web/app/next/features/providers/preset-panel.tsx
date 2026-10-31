// One preset of a vendor on the page: its key panel, or the sign-in of its program for a preset that runs through one
// (ChatGPT), fed by what the runtime lists of the preset.
import KeyPanel from '@/next/features/providers/key-panel';
import type { IPresetActions, IPresetState } from '@/next/features/providers/model';
import ProxyPanel from '@/next/features/providers/proxy-panel';
import { t } from '@/next/i18n';

interface IPresetPanelProps extends IPresetActions {
  state: IPresetState;
  /** The vendor's name, for the field's label. */
  vendor: string;
  onSaved?: () => void;
}

const IDLE = { k: 'idle' } as const;

const PresetPanel = ({
  state,
  vendor,
  onSave,
  onRemove,
  onTest,
  onSaved,
  proxyLogins,
  onProxyLogin,
  onProxyCancel,
  onProxyLogout,
}: IPresetPanelProps) => {
  if (state.proxy !== undefined)
    return (
      <ProxyPanel
        state={state}
        login={proxyLogins[state.id] ?? IDLE}
        onLogin={() => onProxyLogin(state.id)}
        onCancel={() => onProxyCancel(state.id)}
        onLogout={() => onProxyLogout(state.id)}
        onTest={() => onTest(state.id)}
      />
    );
  return (
    <KeyPanel
      hint={state.hint}
      label={t('next.providers.keyAria', { vendor })}
      saved={state.key}
      login={state.login}
      onSave={(key) => onSave(state.id, key)}
      onRemove={() => onRemove(state.id)}
      onTest={() => onTest(state.id)}
      onSaved={onSaved}
    />
  );
};

export default PresetPanel;
