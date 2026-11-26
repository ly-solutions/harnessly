// The note's card over an area of the page or a drawing on it: the same card as a picked block's, its words
// sent as the area's owner writes them. Where the area holds blocks, the card's hand changes the first of them.
import { createContext } from 'react';
import { noteLeft } from '@/next/features/designly/annotation';
import type { IStyleEdit } from '@/next/features/designly/element-style';
import type { IHandWritten } from '@/next/features/designly/hand-edit';
import type { IPicked } from '@/next/features/designly/page-bridge';
import type { AttachmentRef } from '@/next/protocol/v2';

export interface IAreaWords {
  /** The words typed; empty where the area goes to the agent without any. */
  comment: string;
  /** The look still only tried on the first block. */
  style: IStyleEdit;
  /** The hand's changes the source already has. */
  saved: readonly string[];
  /** The page's path on the library server, and that server's port on the machine. */
  url: string;
  port: number;
}

export interface INoteArea {
  kind: 'area' | 'drawing';
  /** In the page's viewport, in its CSS pixels, as a picked block's. */
  rect: IPicked['rect'];
  /** The blocks under it; without any the card has no row of the hand. */
  blocks: readonly IPicked[];
  /** The message for the agent, with the pictures it carries. */
  message: (words: IAreaWords) => Promise<{ text: string; attachments?: AttachmentRef[] }>;
  /** «Leave a comment»; without it the card has no such button. */
  onComment?: (text: string) => void | Promise<void>;
}

/** What the card of an area with no block under it stands on. */
export const areaPicked = (rect: IPicked['rect']): IPicked => ({
  id: null,
  look: null,
  selector: '',
  tag: 'area',
  text: '',
  rect,
  component: null,
  html: '',
});

/**
 * The message a closed card of an area sends; null where there is nothing to send: the card only saved the hand's
 * changes (it had no words and the source takes them).
 */
export const areaMessage = async (
  area: INoteArea,
  card: { words: string; written: Promise<IHandWritten> | null; style: IStyleEdit; look: IPicked['look'] },
  page: { url: string; port: number },
) => {
  const left = await noteLeft(card.words, card.written, card.style, card.look);
  if (card.written && !card.words) return null;
  return area.message({
    comment: card.words,
    style: left?.style ?? card.style,
    saved: left?.saved ?? [],
    ...page,
  });
};

/** Opens the card over an area: the view lends it to the layer that picks areas and draws. */
export const AreaCardContext = createContext<((area: INoteArea) => void) | null>(null);
