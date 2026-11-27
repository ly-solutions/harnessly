// A chat row's offline mark: offline, a chat this device did not keep says so before it is opened. Online
// nothing is drawn and nothing of the chat's feed is listened to.
import { t } from '@/next/i18n';
import { useChatKept } from '@/next/offline/kept';
import { useOffline } from '@/next/offline/net';
import OfflineMark from '@/next/ui/offline-mark';
import type { ConvId } from '@/next/protocol/v2';

const NotKept = ({ conv }: { conv: ConvId }) =>
  useChatKept(conv) ? null : <OfflineMark label={t('next.offline.notKept')} />;

const ChatOffline = ({ conv }: { conv: ConvId }) => (useOffline() ? <NotKept conv={conv} /> : null);

export default ChatOffline;
