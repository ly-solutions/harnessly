// The message of a drawing or an area, made when the person sends it from the card: the blocks under each
// shape asked of the page, their lines in the board's source asked of the library, the two pictures made and put into
// the chat's files, the block written.
import { HUB_ATTACH_MAX_BYTES, keepInHub, uploadFile } from '@/next/commands/uploads';
import { platformPath } from '@/next/core/net/machine';
import { postJson, runtimeJson } from '@/next/core/net/runtime-json';
import { boardShot, drawingPng, overShot } from '@/next/features/designly/draw/capture';
import { formatDrawing, type IDrawnBlock, type IDrawnShape } from '@/next/features/designly/draw/format';
import { shapeBox } from '@/next/features/designly/draw/shapes';
import type { IDrawView, IPageRect, IShape } from '@/next/features/designly/draw/types';
import type { IPicked } from '@/next/features/designly/page-bridge';
import { connectionStore } from '@/next/stores/connection';
import { appConfig } from '@/platform/config';
import type { AttachmentRef, MachineId } from '@/next/protocol/v2';

/** The page's blocks inside a rectangle of its viewport. */
export type TElementsIn = (rect: IPageRect) => Promise<IPicked[]>;

export interface IDrawSendInput {
  machine: MachineId;
  artifact: string;
  kind: 'drawing' | 'area';
  view: IDrawView;
  shapes: readonly IShape[];
  area?: IPageRect;
  elementsIn: TElementsIn;
  comment: string;
  style?: string;
  url: string;
  port: number;
}

const MAX_SHAPES_ASKED = 12;
const MAX_BLOCKS = 4;
const MAX_LOCATE = 16;

/** The page rectangle in the viewport scrolled to (sx, sy). */
const inView = (b: IPageRect, sx: number, sy: number): IPageRect => ({
  x: b.x - sx,
  y: b.y - sy,
  w: b.w,
  h: b.h,
});

/** «file.dc.html:42» for a block's words, asked of the library once per text. */
const locator = (machine: MachineId, url: string) => {
  const seen = new Map<string, Promise<string | null>>();
  return (text: string): Promise<string | null> => {
    const key = text.trim();
    if (key === '' || seen.size >= MAX_LOCATE) return Promise.resolve(seen.get(key) ?? null);
    const have = seen.get(key);
    if (have) return have;
    const asked = runtimeJson<{ matches?: { kind: string; file: string; line: number }[] }>(
      machine,
      platformPath('/design/library/locate'),
      postJson({ path: url, text: key }),
    )
      .then((a) => {
        const m = (a.matches ?? []).find((x) => x.kind === 'template');
        return m ? `${m.file.split('/').pop() ?? m.file}:${m.line}` : null;
      })
      .catch(() => null);
    seen.set(key, asked);
    return asked;
  };
};

const named = (blob: Blob, name: string) => new File([blob], name, { type: 'image/png' });

/** A picture put into the chat's files: the machine takes it, or the hub when the machine is away. */
export const attachPicture = async (machine: MachineId, file: File): Promise<AttachmentRef | null> => {
  const hub =
    appConfig().centralUploads &&
    (connectionStore.get(machine)?.k !== 'live' || file.size <= HUB_ATTACH_MAX_BYTES);
  const outcome = hub
    ? await keepInHub(machine, file, () => undefined)
    : await uploadFile(machine, file, () => undefined);
  return outcome.ok ? outcome.ref : null;
};

export const drawMessage = async (
  input: IDrawSendInput,
): Promise<{ text: string; attachments: AttachmentRef[] }> => {
  const { view } = input;
  const locate = locator(input.machine, input.url);
  const withLines = async (picked: readonly IPicked[]): Promise<IDrawnBlock[]> =>
    Promise.all(
      picked.slice(0, MAX_BLOCKS).map(async (p) => ({ picked: p, template: await locate(p.text) })),
    );
  const ask = (b: IPageRect) => input.elementsIn(inView(b, view.sx, view.sy)).catch(() => [] as IPicked[]);

  const shapes: IDrawnShape[] = await Promise.all(
    input.shapes.map(async (shape, i) => {
      if (i >= MAX_SHAPES_ASKED) return { shape, blocks: [] };
      // An arrow names what its tip points at; any other shape what it covers.
      const b =
        shape.kind === 'arrow'
          ? { x: shape.pts[1][0] - 6, y: shape.pts[1][1] - 6, w: 12, h: 12 }
          : shapeBox(shape);
      return { shape, blocks: await withLines(await ask(b)) };
    }),
  );
  const areaBlocks = input.area ? await withLines(await ask(input.area)) : undefined;

  // The pictures: the marks alone, and the board's visible part with them over it (when the machine can take it).
  const stamp = new Date().toISOString().replace(/[-:]/g, '').slice(0, 15);
  const attachments: AttachmentRef[] = [];
  const pictures: string[] = [];
  try {
    const marks = await drawingPng(input.shapes, view.vw, view.vh, view.sx, view.sy, input.area);
    const shot = await boardShot(input.machine, input.artifact, view.board, view.vw, view.vh, view.sy)
      .then((b) => overShot(b, marks))
      .catch(() => null);
    const files = [
      ...(shot ? [named(shot, `${input.kind}-${stamp}-board.png`)] : []),
      named(marks, `${input.kind}-${stamp}-marks.png`),
    ];
    for (const file of files) {
      const ref = await attachPicture(input.machine, file);
      if (ref) {
        attachments.push(ref);
        pictures.push(ref.name);
      }
    }
  } catch {
    // The words go without the pictures rather than not at all.
  }
  const text = formatDrawing({
    kind: input.kind,
    view,
    area: input.area,
    areaBlocks,
    shapes,
    pictures,
    comment: input.comment,
    style: input.style,
    url: input.url,
    port: input.port,
  });
  return { text, attachments };
};
