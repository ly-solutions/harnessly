// The agent of the Settings window (the design's .isle): a field at the bottom of the page where the person says in
// words what to change, and over it what the agent answered — what it changed, from what to what, with the way back —
// or what it asks before changing.
import { useState, type FormEvent } from 'react';
import { t } from '@/next/i18n';
import { Button } from '@/next/ui/button';
import Icon from '@/next/ui/icon';

export const ISLAND = {
  idle: 'idle',
  thinking: 'thinking',
  done: 'done',
  ask: 'ask',
  failed: 'failed',
} as const;
export type TIslandPhase = (typeof ISLAND)[keyof typeof ISLAND];

export interface IIslandChange {
  id: string;
  title: string;
  was: string;
  now: string;
  busy: boolean;
  onUndo: () => void;
}

export interface IIslandOption {
  label: string;
  hint: string;
  onPick: () => void;
}

interface IAgentIslandProps {
  phase: TIslandPhase;
  /** What the agent said, or why it did not answer. */
  text: string;
  changes: readonly IIslandChange[];
  options: readonly IIslandOption[];
  /** The card over the field is leaving. */
  closing: boolean;
  suggestions: readonly string[];
  onSend: (text: string) => void;
  onDismiss: () => void;
  /** Opens the agent's chat; absent while there is none. */
  onOpenChat?: () => void;
}

const AgentIsland = ({
  phase,
  text,
  changes,
  options,
  closing,
  suggestions,
  onSend,
  onDismiss,
  onOpenChat,
}: IAgentIslandProps) => {
  const [draft, setDraft] = useState('');
  const busy = phase === ISLAND.thinking;
  const send = (words: string) => {
    const said = words.trim();
    if (said === '' || busy) return;
    setDraft('');
    onSend(said);
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    send(draft);
  };
  return (
    <div className="hx-isle">
      {phase === ISLAND.thinking && (
        <div className="hx-isle__reply" role="status">
          <span className="hx-isle__think">
            <Icon name="loader-circle" size="sm" />
            {t('next.settings.agentIsle.thinking')}
          </span>
        </div>
      )}
      {(phase === ISLAND.done || phase === ISLAND.failed) && (
        <div className="hx-isle__reply" data-state={closing ? 'closing' : undefined} role="status">
          <div className="hx-isle__text">{text}</div>
          {changes.map((change) => (
            <div key={change.id} className="hx-isle__diff">
              <span className="hx-isle__diff-title">{change.title}</span>
              <s>{change.was}</s>
              <Icon name="arrow-right" size="sm" />
              <b>{change.now}</b>
              <Button size="sm" variant="ghost" icon="undo-2" busy={change.busy} onClick={change.onUndo}>
                {t('next.settingsChanges.undo')}
              </Button>
            </div>
          ))}
          <div className="hx-isle__acts">
            <Button size="sm" variant="ghost" onClick={onDismiss}>
              {t('next.settings.agentIsle.ok')}
            </Button>
            {phase === ISLAND.failed && onOpenChat && (
              <Button size="sm" variant="ghost" onClick={onOpenChat}>
                {t('next.settings.agentIsle.openChat')}
              </Button>
            )}
          </div>
        </div>
      )}
      {phase === ISLAND.ask && (
        <div className="hx-isle__reply" role="status">
          <div className="hx-isle__text">{text}</div>
          {options.length > 0 && (
            <div className="hx-isle__opts">
              {options.map((option) => (
                <button key={option.label} type="button" className="hx-isle__opt" onClick={option.onPick}>
                  <span className="hx-isle__opt-name">{option.label}</span>
                  {option.hint !== '' && <span className="hx-isle__opt-hint">{option.hint}</span>}
                </button>
              ))}
            </div>
          )}
          {options.length === 0 && onOpenChat && (
            <div className="hx-isle__acts">
              <Button size="sm" variant="ghost" onClick={onOpenChat}>
                {t('next.settings.agentIsle.openChat')}
              </Button>
            </div>
          )}
        </div>
      )}
      <form className="hx-isle__box" onSubmit={submit}>
        <div className="hx-isle__row">
          <input
            className="hx-isle__input"
            value={draft}
            placeholder={t('next.settings.agentIsle.placeholder')}
            aria-label={t('next.settings.agentIsle.placeholder')}
            onChange={(event) => setDraft(event.target.value)}
          />
          <button
            type="submit"
            className="hx-isle__send"
            aria-label={t('next.settings.agentIsle.send')}
            disabled={busy || draft.trim() === ''}
          >
            <Icon name="arrow-up" size="sm" />
          </button>
        </div>
        {phase === ISLAND.idle && draft === '' && suggestions.length > 0 && (
          <div className="hx-isle__sugg">
            {suggestions.map((words) => (
              <button key={words} type="button" onClick={() => send(words)}>
                {words}
              </button>
            ))}
          </div>
        )}
      </form>
    </div>
  );
};

export default AgentIsland;
