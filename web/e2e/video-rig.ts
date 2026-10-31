// What the video test needs beside the deck's rig: three scene boards as an agent writes them (adaptive, with timed
// build-ins), and the owner's view of the guests' comments, answered in the page from what the gateway's stand-in took.
// The guest side (the link's gateway, its media under /m/ with video.mp4 and video-9x16.mp4) is the deck's stand-in.
import type { Page } from '@playwright/test';

import type { IDeckGateway } from './deck-rig';

export const SCENES = [
  { title: 'Intro', head: 'One protocol', body: 'Keys stay with the team' },
  { title: 'Why', head: 'Fewer steps', body: 'Faster builds every day' },
  { title: 'Next', head: 'What comes next', body: 'Exports and sharing' },
];

/** When the body of each scene comes in, s from the scene's start; the title comes at 0.4 s. */
export const BODY_AT = 1.2;

/**
 * A scene as the guide asks for it: a plain board (no engine), one adaptive <main> laid out with @container for 16:9
 * and 9:16, colours from deck-theme.css, the parts timed with data-build-in and data-tk-at.
 */
export const sceneFile = (s: (typeof SCENES)[number]) => `<!doctype html>
<html><head><meta charset="utf-8"><link rel="stylesheet" href="deck-theme.css"><style>
html,body{margin:0;background:var(--deck-bg)}
main{position:relative;width:100vw;height:100vh;overflow:hidden;container-type:size;background:var(--deck-bg);color:var(--deck-ink);font-family:var(--deck-font)}
h1{position:absolute;left:8cqw;top:22cqh;width:84cqw;font-size:9cqh;line-height:1.1;margin:0}
p{position:absolute;left:8cqw;top:48cqh;width:84cqw;font-size:4.5cqh;margin:0;color:var(--deck-mute)}
@container (aspect-ratio < 1){h1{top:30cqh;font-size:5.5cqh}p{top:52cqh;font-size:3cqh}}
</style></head>
<body>
<main data-block="Scene">
  <h1 data-block="Title" data-build-in="fade" data-tk-at="0.4">${s.head}</h1>
  <p data-block="Body" data-build-in="rise" data-tk-at="${BODY_AT}">${s.body}</p>
</main>
</body></html>
`;

const ULID = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
export const scenePageId = (i: number) => `01JVDE00000000000000000${ULID[i + 1]}${ULID[i + 1]}Z`;

/**
 * The owner's list of guest comments (`GET …/guest-comments` of the gateway's access routes) from what the guests
 * posted to the stand-in. Registered after `stubAccess`, so it answers that one path first.
 */
export const stubGuestComments = async (p: Page, gw: IDeckGateway) => {
  await p.route('**/design/access/**/guest-comments', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(
        gw.guestComments.map((c) => ({
          id: c.id,
          linkId: 'l1',
          linkTitle: 'Video',
          board: c.board,
          x: null,
          y: null,
          text: c.text,
          name: c.name,
          at: c.at,
          anchor: c.anchor,
        })),
      ),
    }),
  );
};
