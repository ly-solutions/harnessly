// The microphone of an annotation's row: the composer's dictation (bindings/use-voice.ts) writing into the
// row's field. At rest it is one button in the foot; while it listens the foot is the recording — cancel, the time, the
// wave, stop — as the composer's bar is, in the row's size.
import { RecordingTime, RecordingWave } from '@/next/features/composer/recording';
import type { Recording } from '@/next/features/composer/types';
import { t } from '@/next/i18n';
import Icon from '@/next/ui/icon';
import { PRESENCE, type IPresence } from '@/next/ui/presence';
import '@/next/styles/composer.css';

interface INoteVoiceProps {
  /** The recording with its exit: the bar leaves softly and keeps what it showed meanwhile. */
  bar: IPresence<Recording>;
  /** Without it the machine or the browser cannot record: no microphone. */
  onMic?: () => void;
  onCancel: () => void;
  onFinish: () => void;
}

const NoteVoice = ({ bar, onMic, onCancel, onFinish }: INoteVoiceProps) => {
  const rec = bar.value;
  if (!rec) {
    if (!onMic) return null;
    return (
      <button
        type="button"
        className="hx-note__btn hx-note__mic hx-tipped"
        aria-label={t('next.composer.voice')}
        onClick={onMic}
      >
        <Icon name="mic" size="sm" />
        <span className="hx-tip" data-pos="below" data-align="end">
          {t('next.composer.voice')}
        </span>
      </button>
    );
  }
  return (
    <span className="hx-note__recbar" data-state={bar.state === PRESENCE.closing ? 'closing' : 'open'}>
      <button
        type="button"
        className="hx-note__btn hx-tipped"
        aria-label={t('next.composer.cancelRecording')}
        onClick={onCancel}
      >
        <Icon name="x" size="sm" />
        <span className="hx-tip" data-pos="below">
          {t('next.composer.cancelRecording')}
        </span>
      </button>
      <span className="hx-note__rec">
        <span className="hx-recbar__dot" />
        {rec.busy ? (
          <span className="hx-recbar__time">{t('next.voice.recognizing')}</span>
        ) : (
          <RecordingTime startedAt={rec.startedAt} />
        )}
      </span>
      <RecordingWave level={rec.level} listening={!rec.busy} />
      <button
        type="button"
        className="hx-note__btn hx-tipped"
        aria-label={t('next.composer.finishRecording')}
        aria-disabled={rec.busy || undefined}
        aria-busy={rec.busy || undefined}
        onClick={() => !rec.busy && onFinish()}
      >
        <Icon name="square" size="sm" />
        <span className="hx-tip" data-pos="below" data-align="end">
          {t('next.composer.finishRecording')}
        </span>
      </button>
    </span>
  );
};

export default NoteVoice;
