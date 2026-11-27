// Settings → «Провайдеры» → «Локальная модель»: a model served on this very machine, with no internet and no
// key. The machine looks at the well-known ports (Ollama, LM Studio, llama.cpp, vLLM) and the card lists what answered
// with its models; «Подключить» makes it a provider of the list above, and a chat of Claude Code can start on it. The
// hint is honest about the price: an agent needs a big model for code, a small one loses the thread.
import { useState, type FormEvent } from 'react';
import {
  LOCAL_PRESET,
  type ILocalFound,
  type ILocalServer,
  type IExtProvider,
} from '@/next/features/settings/ext-providers-api';
import { FormError, SetButton } from '@/next/features/settings/rows';
import { plural, t } from '@/next/i18n';

/** How many models of a server are named before «и ещё N». */
const SHOWN_MODELS = 4;
/** Under this many tokens of context an agent has no room for its own instructions: the card warns. */
const MIN_CONTEXT = 16 << 10;

export type TLocalState = { k: 'busy' } | { k: 'failed'; text: string } | { k: 'ready'; found: ILocalFound };

interface ILocalModelCardProps {
  state: TLocalState;
  /** The providers made of this card, whatever their state. */
  connected: readonly IExtProvider[];
  /** Looks again; `address` is what the person typed, '' for the well-known ports only. */
  onScan: (address: string) => void;
  /** Makes a provider of a server; resolves the words of why not, or null. */
  onConnect: (server: ILocalServer) => Promise<string | null>;
}

/** The provider of this card that is at the server's address, if any. */
export const connectedTo = (
  connected: readonly IExtProvider[],
  server: ILocalServer,
): IExtProvider | undefined =>
  connected.find((p) => p.preset === LOCAL_PRESET && p.baseUrl.replace(/\/+$/, '') === server.baseUrl);

const modelsLine = (server: ILocalServer): string => {
  if (server.models.length === 0) return t('next.localModel.noModels');
  const shown = server.models.slice(0, SHOWN_MODELS).map((m) => m.id);
  const more = server.models.length - shown.length;
  return more > 0 ? `${shown.join(', ')} ${plural(more, 'next.localModel.more')}` : shown.join(', ');
};

const ServerRow = ({
  server,
  on,
  onConnect,
}: {
  server: ILocalServer;
  on: boolean;
  onConnect: (server: ILocalServer) => Promise<string | null>;
}) => {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const connect = async () => {
    setBusy(true);
    setError(null);
    const refused = await onConnect(server);
    setBusy(false);
    if (refused !== null) setError(refused);
  };
  return (
    <div className="hx-local__server" data-on={on || undefined}>
      <div className="hx-local__text">
        <div className="hx-local__name">
          {server.name}
          <span className="hx-local__addr">{server.baseUrl.replace(/^https?:\/\//, '')}</span>
        </div>
        <div className="hx-local__models">{modelsLine(server)}</div>
        {server.context > 0 && server.context < MIN_CONTEXT && (
          <div className="hx-local__warn" role="note">
            {t('next.localModel.smallContext', { n: server.context })}
          </div>
        )}
        {error && <FormError>{error}</FormError>}
      </div>
      {on ? (
        <span className="hx-local__on">{t('next.localModel.connected')}</span>
      ) : (
        <SetButton
          tone="primary"
          busy={busy}
          disabled={server.models.length === 0}
          onClick={() => void connect()}
        >
          {t('next.localModel.connect')}
        </SetButton>
      )}
    </div>
  );
};

const LocalModelCard = ({ state, connected, onScan, onConnect }: ILocalModelCardProps) => {
  const [address, setAddress] = useState('');
  const busy = state.k === 'busy';
  const scan = (event?: FormEvent) => {
    event?.preventDefault();
    onScan(address.trim());
  };
  const found = state.k === 'ready' ? state.found : null;

  return (
    <div className="hx-set__group">
      <div className="hx-set__group-title">{t('next.localModel.title')}</div>
      <div className="hx-set__card hx-local">
        <p className="hx-local__lead">{t('next.localModel.lead')}</p>
        {state.k === 'failed' && <FormError>{state.text || t('next.settings.failed')}</FormError>}
        {found && found.servers.length === 0 && (
          <p className="hx-local__none" role="status">
            {t('next.localModel.none')}
          </p>
        )}
        {found?.servers.map((server) => (
          <ServerRow
            key={server.baseUrl}
            server={server}
            on={connectedTo(connected, server) !== undefined}
            onConnect={onConnect}
          />
        ))}
        {found?.custom && !found.custom.found && (
          <FormError>{t('next.localModel.notFound', { address: found.custom.address })}</FormError>
        )}
        <form className="hx-local__form" onSubmit={scan}>
          <input
            className="hx-set__input hx-set__input--mono"
            value={address}
            placeholder={t('next.localModel.addressPlaceholder')}
            aria-label={t('next.localModel.address')}
            spellCheck={false}
            autoCapitalize="off"
            onChange={(e) => setAddress(e.target.value)}
          />
          <SetButton tone="ghost" busy={busy} disabled={busy} onClick={() => scan()}>
            {t('next.localModel.refresh')}
          </SetButton>
        </form>
      </div>
      <div className="hx-set__foot">{t('next.localModel.hint')}</div>
    </div>
  );
};

export default LocalModelCard;
