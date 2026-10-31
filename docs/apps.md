# Apps

An app is a package you write once and turn on where you work: in a chat, a pane tab, a full screen of its own or a
Sinopia document. A plugin is an app without views. The format is a Claude Code plugin plus one manifest of ours:

```text
acme-cashback/
  harnessly/app.json          # the manifest: required
  .claude-plugin/plugin.json  # written by the install from app.json
  instructions.md             # always in the agent's context while the app is on: keep it short
  skills/<name>/SKILL.md      # read by the agent on demand
  commands/*.md               # slash commands
  .mcp.json                   # MCP servers: remote (url); a trusted app may run local ones (command)
  ui/                         # views: static files
```

## Commands

```sh
harnessly app new acme.cashback --template pane-chat   # templates: chat, pane-chat
harnessly app validate ./cashback --strict              # every problem names its field; --strict fails on warnings
harnessly app dev ./cashback                            # install the folder itself while you write it
harnessly app install ./cashback                        # install a copy: later edits do not reach it
harnessly app list
harnessly app enable acme.cashback                      # in the chat this tab runs; disable turns it off
harnessly app remove acme.cashback
```

Installed apps live in `<data>/apps`: the list in `installed.json`, the copies in `packages/<id>/<version>`. A dev
install points at your folder and is never deleted by `remove`.

## Manifest

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | yes | `<publisher>.<name>`, lower case latin letters, digits and `-`; never changes |
| `name`, `description` | yes | What people see |
| `version` | yes | `1.2.0`; a published version never changes |
| `harnessly` | yes | Host versions it works with (apps came in 0.16.36): `>=0.16.36`, `>=0.17`, `^0.17`, `>=0.17 <1`, `0.16 \|\| >=0.18` |
| `trust` | yes | `declarative` (text only), `sandboxed` (views run scripts in a sandbox), `trusted` (local processes; installed by hand) |
| `targets` | yes | `chat`, `pane`, `app`, `sinopia.<kind>`, `sinopia.*` |
| `activation` | no | `on`: `manual`, `command:/x`, `view`, `document`, `always`; `openView` |
| `permissions` | yes | `chat.prompt`, `chat.read`, `chat.card`, `pane.open`, `storage`, `storage.space.write`, `ai`, `files.read:<mask>`, `files.write:<mask>`, `secrets:<name>`, `sinopia.read\|write\|comment\|export\|kind`, `users.read`, `notify` |
| `network` | no | `connect`, `resources`, `frames`: host names; `*` needs `reason` |
| `secrets` | no | `{name, description}`: the person types the value at install, the app never reads it |
| `settings` | no | JSON Schema of an object: the install form |
| `agent` | no | `instructions`, `skills`, `mcp`, `context`, `tools` (`allow`, `deny`, `confirm`), `validators`, `flow`, `exclusive` |
| `views` | no | `{id, title, entry, targets}`: `entry` is a file of the package or an `https://` address listed in `network.frames` |
| `chat` | no | `commands` (`/name`, a file `commands/name.md`), `cards` (views with the chat target) |

A target with nothing of the app in it is an error, and so is a contribution to a target the app does not list.

## In a chat

`harnessly app enable <id>` (or the chat's app list) turns an app on in a Claude Code chat; Codex chats get apps
later. An agent at rest is stopped at once and the next message brings it back with the app; an agent at work keeps
what it had until its terminal stops. The launch then gets:

- the package as a Claude Code plugin (`--plugin-dir`): its skills, commands and `.mcp.json` servers;
- the instructions files of all its apps in the system prompt, under `# Apps on in this chat`;
- a snapshot of the apps' tool rules (`<data>/apps/launch/policy-*.json`) and the chat's own settings plus the
  `harnessly hook app-guard --policy <snapshot>` PreToolUse hook, which decides each tool call by it, locally.

## Tool policy

`agent.tools` has three lists of rules: `allow`, `deny`, `confirm` (the person confirms the call in the chat). A rule
is a built-in tool (`Edit`), a built-in tool with a mask on its input (`Bash(git push*)`, Claude's `Bash(rm:*)` works
too), a tool of an MCP server (`bank-cashback.create`, `bank-cashback.*`), the host's `app.*` tools, `mcp:*` for every
MCP tool or `*`.

Within one app the most specific rule wins: an exact name beats a mask, a longer mask beats a shorter one, `mcp:*` and
`*` lose to everything; on a tie `deny` beats `confirm`, `confirm` beats `allow`. Across the apps of a chat a `deny` of
any app wins, then a `confirm`. A call no rule names is left to the chat's own permissions.

A `Bash(...)` rule is matched against each simple command of the line (`a && b`, `a; b`, a subshell, with variable
assignments and the program's path cut off). A line the guard cannot see into (`eval`, `sh -c`, `xargs`) is confirmed
by the person instead of passing a deny silently, and an `allow` holds only when every command matches. An app's
allow reaches only its own MCP servers. The rules are a guardrail for the agent, not a sandbox: the agent runs as the
same user as the runtime, and a determined one has ways around any check of its command lines.

The guard holds the rules the launch was given: editing a manifest, or running `harnessly app enable|disable|install|
dev|remove` (and their aliases) from inside the chat, is refused, and so is writing into `<data>/apps` or an app's
folder. A snapshot the guard cannot read, or one whose content no longer matches the hash in its name, refuses every
call.

## Examples

- [`apps/mr-review`](apps/mr-review): review without edits, a comment in the MR only after the person confirms.

## State

What ships now: the manifest and its check, the registry, `harnessly app`, the per-chat list of apps
(`conversation.apps`), and what a Claude Code chat's agent gets from them (plugin, instructions, tool policy). The
views (a pane tab, a full screen, cards in the chat), Sinopia and sharing come next.
