# Sinopia with the machine off

The machine is for the agent's edits only; the hub is the single truth for everything else (epic HRN-1790). While the
owner's machine has no link, and the space has its store, the hub answers these itself. There is no switch: this is
how Sinopia works.

| What | How the hub answers | Task |
| --- | --- | --- |
| Library, boards, guest links | from the project's revisions in the store (`designview`) | HRN-1795 |
| A board on a UI kit | the pinned kit is laid out from the kits collection at its newest revision and given to the board's library as `KitsDir`, so the board keeps its styles on a guest link, in the render worker and in the hub's library | HRN-1799 |
| Kits' reads (`kit/*`, `kits/state`) | from the space's kits collection, the kits the person may see (`designaccess/kitsread.go`, also with `?from=store`) | HRN-1799 |
| Library statuses (checks, release) | from the facts the machine left in the project's record | HRN-1799 |
| Publishing a site | `POST <platform>/design/publish` builds the site from the newest revision in the store (or `revision`) and deploys it as the machine's own site: managers only, and only with the apps' verdict the machine left for that revision (`share_unchecked` / `share_blocked` otherwise); `GET <platform>/design/published` names it | HRN-1799 |
| A comment that calls the agent | `POST <platform>/design/site/comments/agent` answers 202 `{"queued":true}`; the comment waits in `design_agent_comments` and goes to the machine as its author when the machine is back | HRN-1799 |
| Board pictures and covers | the render worker (`render-worker.md`) | HRN-1798 |
| Versions, history, compare, restore | from the store | HRN-1797 |

With the machine on, the machine answers all of it, as before.
