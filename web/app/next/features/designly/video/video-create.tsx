// The create screen of a video (the deck's «Создание»), a field as the chat's: the brief growing with its text and its
// attachments; under it the clip and one «Настройки» button on the left (the design system, the formats, the length and
// the subtitles, video-settings.tsx), the model and the round send on the right. The button says only what differs
// from the start. A brief sent waits for the runtime; a refusal is said by the caller and the brief stays.
import { useRef, useState, type KeyboardEvent } from 'react';
import { useUploads } from '@/next/bindings/use-uploads';
import { deckTitleOf } from '@/next/features/designly/deck/create-settings';
import ModelPicker, { type IModelPickerProps } from '@/next/features/composer/model-picker';
import type { IDesignSystem } from '@/next/features/designly/api';
import {
  DEFAULT_VIDEO_SETTINGS,
  toWireVideo,
  type IVideoCreateSettings,
} from '@/next/features/designly/video/video-create-settings';
import VideoSettings, { formatLabel, secondsLabel } from '@/next/features/designly/video/video-settings';
import type { DesignlyVideo, MachineId } from '@/next/protocol/v2';
import { clearDraft, setDraftText, useDraft, type DesignDraftKey } from '@/next/stores/drafts';
import { RoundButton } from '@/next/ui/button';
import Icon from '@/next/ui/icon';
import { useFitField } from '@/next/ui/use-fit-field';
import { t } from '@/next/i18n';

/** The draft of the create screen: the brief typed on Designly's start goes with the person here. */
export const VIDEO_NEW_DRAFT: DesignDraftKey = 'design:new-video';

export interface IVideoCreateInput {
  title: string;
  brief: string;
  video: DesignlyVideo;
  /** The machine's design system the video is built on; null for none. */
  designSystem: string | null;
  attachmentIds: string[];
}

export interface IVideoCreateProps {
  machine: MachineId;
  /** The design systems the machine has (the same offer as a site's start). */
  systems: readonly IDesignSystem[];
  onBack: () => void;
  /** The agent that draws and the machine it runs on, as a new chat picks them; null until the machine's agents are read. */
  model?: IModelPickerProps | null;
  /** Sends the brief; resolves once the runtime made the project, rejects when it did not (the screen stays). */
  onCreate: (input: IVideoCreateInput) => Promise<void>;
}

/** What the settings button says: «Настройки» alone while all is as it starts, else what differs, joined by « · ». */
const settingsLabel = (s: IVideoCreateSettings, look: string | null): string => {
  const parts: string[] = [];
  if (look !== null) parts.push(look);
  if (s.formats !== DEFAULT_VIDEO_SETTINGS.formats) parts.push(formatLabel(s.formats));
  if (s.seconds !== null) parts.push(secondsLabel(s.seconds));
  if (s.subs !== DEFAULT_VIDEO_SETTINGS.subs)
    parts.push(s.subs ? t('next.video.create.subs') : t('next.video.create.noSubs'));
  return parts.length > 0 ? parts.join(' · ') : t('next.video.create.settings');
};

const VideoCreate = ({ machine, systems, model = null, onBack, onCreate }: IVideoCreateProps) => {
  const draft = useDraft(VIDEO_NEW_DRAFT);
  const brief = draft?.text ?? '';
  const uploads = useUploads(machine, VIDEO_NEW_DRAFT, draft?.attachments ?? []);
  const [settings, setSettings] = useState<IVideoCreateSettings>(DEFAULT_VIDEO_SETTINGS);
  const [ds, setDs] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const field = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [box, setBox] = useState<HTMLElement | null>(null);
  useFitField(field, brief);

  const send = () => {
    const text = brief.trim();
    if (sending) return;
    // An empty field is not an error: the button takes the person back to it, as Designly's start does.
    if (!text) {
      field.current?.focus();
      return;
    }
    setSending(true);
    void onCreate({
      title: deckTitleOf(text),
      brief: text,
      video: toWireVideo(settings),
      designSystem: ds,
      attachmentIds: uploads.ready.map((a) => a.id),
    })
      .then(() => {
        clearDraft(VIDEO_NEW_DRAFT);
        uploads.reset();
      })
      // A refusal is said by the caller; the brief stays in the field to send again.
      .catch(() => undefined)
      .finally(() => setSending(false));
  };
  const onKey = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault();
      send();
    }
  };

  const busy = sending || uploads.uploading;
  const label = settingsLabel(settings, systems.find((x) => x.id === ds)?.label ?? null);
  const changed = label !== t('next.video.create.settings');
  return (
    <section className="hx-deck-new hx-video-new" data-screen="create">
      <header className="hx-deck-new__bar">
        <button
          type="button"
          className="hx-btn"
          data-variant="ghost"
          data-size="sm"
          data-shape="icon"
          aria-label={t('next.video.shell.back')}
          onClick={onBack}
        >
          <Icon name="arrow-left" />
        </button>
        <div className="hx-deck-new__addr">
          <b>{t('next.tabs.designly')}</b>
          <span>/</span>
          <em>{t('next.video.create.title')}</em>
        </div>
      </header>

      <div className="hx-deck-new__stage">
        <h2>{t('next.video.create.title')}</h2>
        <div className="hx-deck-box" ref={setBox}>
          <textarea
            ref={field}
            aria-label={t('next.video.create.brief')}
            placeholder={t('next.video.create.placeholder')}
            value={brief}
            disabled={sending}
            onChange={(event) => setDraftText(VIDEO_NEW_DRAFT, event.target.value)}
            onKeyDown={onKey}
          />
          {uploads.attachments.length > 0 && (
            <div className="hx-deck-files">
              {uploads.attachments.map((a) => (
                <span key={a.id} className="hx-deck-file" data-state={a.status} title={a.error ?? undefined}>
                  <Icon name={a.status === 'uploading' ? 'loader-circle' : 'file-text'} size="sm" />
                  {a.name}
                  {a.status === 'error' && (
                    <button
                      type="button"
                      aria-label={t('next.composer.retryAttachment', { name: a.name })}
                      onClick={() => uploads.onRetry(a.id)}
                    >
                      <Icon name="rotate-ccw" size="sm" />
                    </button>
                  )}
                  <button
                    type="button"
                    aria-label={t('next.composer.removeAttachment', { name: a.name })}
                    onClick={() => uploads.onRemove(a.id)}
                  >
                    <Icon name="x" size="sm" />
                  </button>
                </span>
              ))}
            </div>
          )}
          <div className="hx-deck-boxrow">
            <input
              ref={picker}
              type="file"
              multiple
              hidden
              tabIndex={-1}
              onChange={(event) => {
                uploads.onFiles([...(event.target.files ?? [])]);
                event.target.value = '';
              }}
            />
            <button
              type="button"
              className="hx-btn"
              data-variant="ghost"
              data-size="sm"
              data-shape="icon"
              aria-label={t('next.video.create.attach')}
              onClick={() => picker.current?.click()}
            >
              <Icon name="paperclip" />
            </button>
            <button
              type="button"
              className="hx-deck-chip"
              data-on={open || changed ? '1' : '0'}
              aria-expanded={open}
              aria-haspopup="dialog"
              onClick={() => setOpen(true)}
            >
              <Icon name="sliders-horizontal" size="sm" />
              <span className="hx-deck-chip__text">{label}</span>
            </button>
            <span className="hx-deck-boxrow__grow" />
            {model && <ModelPicker {...model} />}
            <RoundButton
              icon="arrow-up"
              variant="accent"
              label={t('next.video.create.submit')}
              keys={['↵']}
              busy={busy}
              disabled={busy}
              onClick={send}
            />
          </div>
        </div>
      </div>

      <VideoSettings
        open={open}
        onClose={() => setOpen(false)}
        anchor={box}
        settings={settings}
        onChange={setSettings}
        systems={systems}
        ds={ds}
        onDs={setDs}
      />
    </section>
  );
};

export default VideoCreate;
