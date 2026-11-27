// The mark of «offline»: the same glyph wherever the device's lack of a network matters — beside the
// workspace's name (with the word), on a chat or a design this device did not keep (the glyph alone). It comes and goes
// with a fade, so losing and finding the network never jolts the sidebar.
import Icon from '@/next/ui/icon';
import Tooltip from '@/next/ui/tooltip';

interface IOfflineMarkProps {
  /** What the hint says. */
  label: string;
  /** The word beside the glyph («Офлайн»); the glyph alone without it. */
  word?: string;
  /** «chip»: the glyph on a soft round, to be seen at a glance (beside the workspace's name). */
  tone?: 'quiet' | 'chip';
  /** Another glyph than «no network» (the laptop of «this computer»). */
  icon?: string;
}

const OfflineMark = ({ label, word, tone = 'quiet', icon = 'wifi-off' }: IOfflineMarkProps) => (
  <Tooltip label={label}>
    <span
      data-tone={tone}
      className={word ? 'hx-offmark hx-offmark--word' : 'hx-offmark'}
      role="img"
      aria-label={label}
      tabIndex={-1}
    >
      <Icon name={icon} size="sm" />
      {word && <span className="hx-offmark__word">{word}</span>}
    </span>
  </Tooltip>
);

export default OfflineMark;
