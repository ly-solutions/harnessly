# Harnessly for Obsidian

The notes you choose, for your harnessly agents. Nothing is shared until you switch it on.

## Install

1. In harnessly: Settings → Connections → Files → **Obsidian**. Download `harnessly-obsidian.zip` from the steps.
2. Unpack it into `<vault>/.obsidian/plugins/` (it makes a `harnessly/` folder), then in Obsidian: Settings →
   Community plugins → turn on **Harnessly**.
3. Open the pairing link from the steps (or scan its QR code on a phone; or run «Harnessly: Paste the pairing code»).
4. Choose: **Choose what to share** (nothing goes yet) or **Share the whole vault** (all but notes tagged `#private`).

## Choosing

- The **Harnessly panel** (ribbon icon or status bar): the folder tree with a switch per folder and note, how many
  notes are shared, search, «Shared» filter, the tags.
- **Right click** a note or folder → «Share with harnessly» / «Stop sharing».
- **Tags**: «never share» (`private` by default) beats everything but frontmatter; «always share» shares wherever the
  note is.
- **Frontmatter** `harnessly: true` or `harnessly: false` decides for that note, above all else.

A note you stop sharing is removed from harnessly at the next sync. Links between notes are sent only between shared
notes. Notes over 512 KB are not sent; the panel lists them.

## How it works

Plain JavaScript, no build: `main.js` is the plugin. It sends the chosen notes, signed with the connection's key
(HMAC-SHA256), to your harnessly machine through your hub; see «Obsidian» in `docs/connectors.md`. Updates come from
your hub: the plugin offers them in its settings.

Tests: `internal/connectors/obsidian` runs this plugin in node (`testdata/run.js`, with `testdata/obsidian-mock.js`
for Obsidian) against the connector.
