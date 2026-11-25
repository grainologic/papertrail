# CLI reference

Run `pt init` once per project. `pt` walks up from the current directory to find `.papertrail/config.toml`. Every command accepts `--json` for structured output (or `{"error": "..."}` on failure); bad flags or arguments print usage and exit 2.

## Help

`pt help [command]` prints help for `pt` or one command. `pt --help` and `pt -h` print the same overview as `pt help`; `pt <command> --help` and `pt <command> -h` print one command's usage and flags. Two-word commands take a space in the `help` form: `pt help section get`. Help exits `0`; `pt help <unknown>` exits `2`.

## Commands

| Command | What it does |
|---|---|
| `pt init` | Create `.papertrail/` with a config and the generated files |
| `pt create <type> <title>` | New item. Flags: `--priority 0-4`, `--mode auto\|hitl`, `--depends-on`, `--required-by`, `--related-to` (links repeat) |
| `pt list [--type T] [--status S]` | List items. Sorted by priority, closed items last |
| `pt show <id>` | One item, its links, and each linked item's status |
| `pt update <id> [--title] [--type] [--status] [--priority] [--mode]` | Change fields. A new title renames the file |
| `pt frontier` | Items ready to work on |
| `pt sync` | Repair links and file names, rebuild the generated files. See [Sync](#sync) |
| `pt link <id>` / `pt unlink <id>` | Add or remove a link (`--depends-on`, `--required-by`, `--related-to`) |
| `pt insert <type> <title> --after A --before B` | New item between two items: it depends on A, and B depends on it, replacing their direct link. Both flags are required |
| `pt batch <file\|->` | Create many items from a JSON array. All or nothing; `--dry-run` previews. See [Batch input](#batch-input) |
| `pt section get <id> <heading>` | Print one section of an item |
| `pt section set <id> <heading> [--file PATH]` | Replace a section with text from stdin, or from `--file PATH` (the file is left in place). This is how an agent writes results back |
| `pt delete <id> [-y]` | Without `-y`, preview what would go. With `-y`, remove the item and strip links to it |
| `pt version` | Print the version (`--version` and `-v` work too) |

## Sync

Every writing command (`create`, `update`, `delete -y`, `link`, `unlink`, `insert`, `batch`, `section set`) and `pt frontier` runs the full sync pass automatically at the end. Run `pt sync` directly after editing item files by hand.

> [!NOTE]
> LLMs have been instructed to do this periodically regardless.

### What `pt sync` repairs

- Renames item files whose slug no longer matches frontmatter `title`.
- Rewrites each `# Title` heading from frontmatter `title`.
- Recomputes `requiredBy` from `dependsOn`.
- Makes `relatedTo` symmetric across linked items.
- Rebuilds `INDEX.md`, `FRONTIER.md`, and `HEALTH.md` (writing each file only when its contents change).

### Errors, warnings, and exit codes

| Finding | Examples | `pt sync` exit | Effect on item and index |
|---|---|---|---|
| **Error** | Dependency cycle, dangling link, ID/folder mismatch, invalid frontmatter, folder without exactly one `.md` file | `1` | Reported in `HEALTH.md`; item is left untouched and excluded from `INDEX.md` and `FRONTIER.md` |
| **Warning** | Item in `in_progress`, `review`, or `closed` with an unclosed dependency | `0` | Reported in `HEALTH.md`; item stays in `INDEX.md` |

Writing commands exit `0` whenever their own change succeeds, even if the trailing sync pass reports existing `HEALTH.md` errors.

## Batch input

`pt batch <file|->` reads a JSON array of items to create:

```json
[
  {"key": "a", "type": "task", "title": "Parse"},
  {"key": "b", "type": "bug", "title": "Crash", "dependsOn": ["a"]}
]
```

Each entry takes `type` and `title`, plus optional `key`, `priority`, `mode`, `dependsOn`, `requiredBy`, and `relatedTo`. Links point at another entry's `key` or at an existing ID. The whole array is validated before anything is written; if any entry fails, nothing is created. After a successful run, `batch` deletes the input file. Stdin (`-`) leaves nothing to delete, and `--dry-run` validates and prints the plan without writing or deleting.

## Values

- Types: `bug`, `chore`, `debate`, `feature`, `feedback`, `milestone`, `question`, `research`, `spike`, `task`.
- Statuses: `open`, `in_progress`, `review`, `blocked`, `closed`.
- Priority: `0` (highest) to `4`. The default is `2`.
- Mode: `auto` (default) or `hitl`, a label for items that need a human in the loop. `pt` stores it and does not filter on it, so a `hitl` item still shows up in the frontier.

## IDs

IDs are 8 lowercase Crockford base32 characters. Any unique prefix works: `pt show 1x4` is the same as `pt show 1x4fhcy2`. A prefix that matches nothing gives `unknown id prefix`.

## Links and the frontier

Items link to each other in three ways. `dependsOn` means another item blocks this one. `requiredBy` is the other end of `dependsOn`; `pt sync` fills it in, so you rarely write it by hand. `relatedTo` is symmetric.

The frontier is the `open` or `in_progress` items whose `dependsOn` items are all `closed`, sorted by priority then creation time. A dependency in `review` or `blocked` still holds an item back. When an agent asks "what next?", this is the answer.

A bad ID or a dependency cycle makes `create`, `link`, `insert`, or `batch` fail without writing anything.

## Sections

`pt section get` and `pt section set` match an exact heading line in the item body (ignoring headings inside fenced code blocks) and include everything up to the next heading of the same or higher level, including subheadings. If a heading matches more than once or is missing, the command exits 1. `pt section set` reads the replacement text from stdin, or from `--file PATH` (which is left in place), and updates only the lines under that heading, leaving frontmatter and all other sections untouched.

## What's in `.papertrail/`?

```text
.papertrail/
  config.toml
  cache.json
  INDEX.md
  FRONTIER.md
  HEALTH.md
  1x4fhcy2/
    write-parser.md
    data/
```

`.papertrail/config.toml` holds a single `root` key (default `.papertrail/`), resolved relative to the workspace root. Each item gets a folder named by its ID with one `.md` file at the root of that folder; extra artifacts or attachments go in `<id>/data/` (created as needed, left alone by `pt`, and removed by `pt delete -y`).

> [!NOTE]
> `.papertrail/` is always created on `pt init` (to hold `config.toml`), while the work item root (where item folders, `INDEX.md`, `FRONTIER.md`, `HEALTH.md`, and `cache.json` live) is configurable per project via `root` in `.papertrail/config.toml`.

The generated files in the item root:

- `INDEX.md`: a table of every healthy item, closed items last.
- `FRONTIER.md`: the ready items, sorted by priority then creation time. Agents read this file when picking work.
- `HEALTH.md`: errors and warnings from the latest sync pass.
- `cache.json`: parsed frontmatter cached by path, size, and modification time for this machine. Keep `cache.json` out of git; committing `INDEX.md`, `FRONTIER.md`, and `HEALTH.md` is optional because `pt sync` rebuilds them.