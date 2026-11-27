// A machine handed over to a person gets their own data only after they accept it: who may still control it is said
// first, and the yes is a second press.
import { useState } from 'react';

import type { OrgMachine } from '../../../org/api/machines';
import { Btn, Icon } from '../../ui';

export const AcceptMachine = ({
  m,
  onAccept,
}: {
  m: OrgMachine;
  /** Accepts the machine for the viewer; true once the hub did. */
  onAccept: () => Promise<boolean>;
}) => {
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  if (!m.awaitsOwner) return null;
  const who = m.awaitsOwner.heldBy ?? 'неизвестный нам человек';
  const go = async () => {
    if (busy) return;
    setBusy(true);
    try {
      if (await onAccept()) setAsking(false);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="hxa__msg hxa__trust" data-tone="warn" role="region" aria-label={`Принять ${m.name}`}>
      <Icon i="shield-check" />
      <div className="hxa__stack">
        <strong>
          {m.mine ? `${m.name} ждёт, когда вы её примете` : `${m.name} ждёт, когда её примет владелец`}
        </strong>
        {!m.mine ? (
          <span>
            Сейф, память и другие личные данные владельца{m.ownerName ? ` (${m.ownerName})` : ''} на неё не
            приходят, пока он не нажмёт «Принять машину».
          </span>
        ) : asking ? (
          <span>
            Приняв машину, вы разрешаете передавать на неё ваш сейф, память, историю и другие личные данные.
            Подключал её {who}: он может по-прежнему управлять машиной и увидеть эти данные. Если не
            доверяете, не принимайте: переустановите машину и подключите своим кодом (команда установки с
            --re-register).
          </span>
        ) : (
          <span>
            Сейф, память и другие ваши личные данные на неё пока не приходят: её ключи регистрировал {who}.
          </span>
        )}
        {m.mine && (
          <div className="hxa__actions">
            {asking ? (
              <>
                <Btn variant="primary" size="sm" busy={busy} onClick={() => void go()}>
                  Да, принять
                </Btn>
                <Btn variant="ghost" size="sm" disabled={busy} onClick={() => setAsking(false)}>
                  Отмена
                </Btn>
              </>
            ) : (
              <Btn variant="primary" size="sm" onClick={() => setAsking(true)}>
                Принять машину
              </Btn>
            )}
          </div>
        )}
      </div>
    </div>
  );
};
