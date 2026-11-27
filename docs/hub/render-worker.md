# Render worker: design pictures without the machine

The gallery of a design library shows a picture of every board. A machine draws them for its own projects while it
runs. When the machine is off, nobody did, so a board changed in the hub's store (by another person's machine, by a
restore, by an import) kept its old picture or none. The render worker draws those (epic HRN-1790: the hub is
the single truth, and the viewer works without the machine).

## How it goes

1. Someone opens a library's gallery. For each picture the browser asks, the hub compares the board's picture in the
   store's thumbs collection with the project's newest revision (`designview.View.ThumbWant`). Missing, deleted or
   older than the project's last change: the hub queues a job in `design_render_jobs` (once per board and revision; one
   look per board in 30 s, four at a time) and answers with what it has.
2. The worker (`runtime render-worker`, the runtime image, which carries Chrome) long-polls
   `POST /agent/render/claim` with `Authorization: Bearer $HUB_RENDER_TOKEN`. The hub gives the oldest job and, at
   that moment, a keyed address of the board in its store, made for the machine's owner:
   `/m/<machine>/<prefix>/design/store/<project>/k/<key>/f/<board>`. The key lives 12 hours, so a job that waited
   long still gets a live one. The job says nothing about the space or the person.
3. The worker opens `HUB_URL` + that address in a headless Chrome at the board's size (its own `w`×`h` when the project
   gives one, a phone or a desktop otherwise, as on the machine) and posts the JPEG to
   `POST /agent/render/done?id=<job>`. A failure is posted as `&error=<why>`.
4. The hub checks the picture (a JPEG, under `filestore.MaxThumbBytes`) and commits it to the thumbs collection as
   author `render`. Every viewer gets it from the store from then on, machine on or off.

The project's cover is drawn the same way. When the gallery asks for the picture of the project's first board (the
board whose shot the machine's check keeps as the cover, `design.CoverFile`) and the cover in the store's covers
collection is missing or older than the project (`designview.View.CoverWant`), the hub also queues a job of kind
`cover` for that board. The worker draws it like any other picture, and the hub commits it at `filestore.CoverPath`,
from which the library's cards take the cover.

A page of a guest link's snapshot (kind `page`) is drawn the same way, with three differences. The hub pins the
snapshot to the store's newest revision (with the apps' checks, or the verdict the machine left when it is off), lists
the boards from the store at that revision, and asks the worker for each page at the pinned address
`…/k/<key>/v/<revision>/f/<board>`, so a later change of the project does not leak into the snapshot. The worker
draws it only when the page drew cleanly (loaded, no script errors, no file failed), as the machine does. The
picture is put as a blob into the thumbs collection and its sha into the job (`result`): nothing is committed, and the
hub that waits (`designrender.Service.Page`) reads the blob from there, whichever replica the worker answered. Page
jobs are given out before the gallery's pictures, a failed page is not tried again until the owner asks again, and
the whole snapshot waits at most 2 minutes. A hub without the worker, or a machine outside a space, still asks the
owner's machine for the snapshot.

A job claimed by a worker that went quiet is given out again after 2 minutes; a job that failed 3 times is left; rows
older than 24 hours are swept. Any number of workers can run: a claim is one conditional `UPDATE`.

## Running it

Helm: `renderWorker.enabled: true` and `renderWorker.image.tag` (a runtime release that has `render-worker`). The chart
makes the token's Secret (or takes `renderWorker.existingSecret`, key `token`) and gives it to both the hub and the
worker. The worker calls the hub's Service inside the cluster unless `renderWorker.hubUrl` names another address.

By hand:

```sh
HUB_URL=https://hub.example.com HUB_RENDER_TOKEN=... runtime render-worker
```

`HUB_RENDER_TOKEN` must be at least 32 characters. A hub without it queues nothing and serves no worker routes, as
before. Chrome is found as the design engine finds it (`RUNTIME_DESIGN_CHROME`, then the usual places).

## Checks and exports

With the owner's machine off, the board check and the exports (PDF, PPTX, PNG, HTML, ZIP, bundle, MP4) run on the
worker too. The hub (`designaccess.serveWork`) answers the machine's own routes — `library/check`, `library/shot`,
`deck/export`, `deck/export/file` — from the queue, so the page does not change; answers carry `X-Harnessly-From: store`.

- A job is `kind=check` (`board` is `*` or the boards one per line) or `kind=export` (`board` is
  `<format>\n<rec>\n<board>`), pinned to the store's head revision. A check leases for 20 minutes, an export for 45.
- The worker takes the project's copy from `GET /agent/render/bundle?id=<job>` (its token; a gzipped tar of
  `library/<project>/…` and `kits/…`), runs the machine's own check or export in a passive library of its own, and
  posts the check's folder (gzipped tar) or the export's file to the done path, with `X-Render-Disposition`.
  The hub keeps the result as a blob and the job's `disposition`.
- A work job that errors is failed at once; asking again requeues the same job.
- While a check or an export runs the worker claims with `?pictures=1`, so thumbs and snapshots do not wait.
- An export file whose job id is the hub's (24 hex) is served by the hub even after the machine came back.

Not from the store: publishing a PDF for guests (409 «включите машину») and an MP4's screen recordings (`.media` is
not in the store).
