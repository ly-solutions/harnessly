# Guest links of designs and `harnessly doctor share`

A guest link opens a design to people without a session. Since hub 0.309.0 a guest is served from the hub's
store and nowhere else, never from the owner's machine, so a link to a project the store holds works whatever the
machine's runtime is, a sandbox such as gVisor included. With the store on, the machine's check below does not stop a
link either (GitHub #187): a live link to a project the store has nothing of yet is made at once, and its guest is told
«Проект ещё не опубликован» until the machine's sync puts the project there; a pinned link waits for that (409
`not_published`). The check matters only on a hub without a store, where the machine itself would serve the guest.

Where a machine still reads a guest's path, the only thing between it and the rest of the disk is how the runtime opens
it:

- **one confined lookup**: the path is resolved beneath the project folder (`os.Root`), so `..`, a relative or an
  absolute symlink, and a symlinked directory that point out of it are refused;
- **one stable handle**: the file is served from the handle that lookup returned, so a path swapped after the lookup
  (a rename over it) does not change what the guest gets.

A runtime whose filesystem does not keep these guarantees cannot serve guests safely. A PNG and an MP4 that render
prove nothing here: `harnessly doctor render` passes in such a runtime.

## The probe

Before it serves a guest or lets the hub make a link, the runtime checks the guarantees themselves in a throwaway
folder made inside the design library (the same filesystem the projects are on) and removes it after. The result is
kept an hour when every check holds and a minute when one fails, so a fixed runtime is seen again soon.

While a check fails, what depends on the machine is **closed**, never weakened:

- the runtime answers every guest read 503 with `X-Design-Share-Refused: share_unsafe_runtime`;
- on a hub without a store (`HUB_STORE=off`) the hub makes no link: the owner's dialog gets 409 `share_unsafe_runtime`
  with what to run instead. With the store on, links are made and served from the store as above.

Run the same check by hand, on the machine, as the runtime's user:

```sh
harnessly doctor share                 # RUNTIME_DESIGN_LIBRARY_DIR, else RUNTIME_WORKSPACE_DIR/design-artifacts
harnessly doctor share --library DIR --json
```

It exits 1 when a check fails.

| Code    | Check               | What failed                                                                  |
| ------- | ------------------- | ---------------------------------------------------------------------------- |
| SHR-100 | `setup`             | the probe could not make its folder in the library (missing, not writable)   |
| SHR-101 | `inside`            | a file inside the folder does not open through the confined lookup           |
| SHR-201 | `dotdot`            | `..` leaves the folder                                                       |
| SHR-202 | `relative-symlink`  | a relative symlink out of the folder is followed                             |
| SHR-203 | `absolute-symlink`  | an absolute symlink out of the folder is followed                            |
| SHR-204 | `directory-symlink` | a symlinked directory out of the folder is followed                          |
| SHR-301 | `swap`              | an open file reads what was renamed over its path after it was opened        |

A check that cannot run counts as failed.

## Where guest links work

Checked to pass: Linux run directly, a virtual machine, a container under the default `runc` or `crun` runtime.

Known to fail: sandboxes that emulate the kernel's filesystem, such as gVisor (`runsc`). There guest links work through
the hub's store (`HUB_STORE` on); there is no switch that skips the probe.

Other runtimes are not listed until they are checked: run `harnessly doctor share` there. If a probe result looks wrong
for a runtime, report it privately to the maintainers, not in a public issue: the details of a containment failure are
security-sensitive.
