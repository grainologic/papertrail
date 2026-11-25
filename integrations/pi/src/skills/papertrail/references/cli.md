# pt CLI and template reference

## Self-help first

- `pt --help` lists every command. `pt help <command>` and `pt <command> --help` print one command's usage and flags.
- Every command accepts `--json`: structured output on success, `{"error": "..."}` on failure. Prefer it for reads.
- Exit codes: `0` success, `1` the operation failed (unknown ID, missing or ambiguous heading, `pt sync` found health errors), `2` bad flags or arguments (usage is printed).
- `pt` walks up from the current directory to find `.papertrail/config.toml`.

## Commands

| Command | Flags and notes |
|---|---|
| `pt init` | Create `.papertrail/` with `config.toml` and generated files in the current directory |
| `pt create <type> <title>` | `--priority 0-4` (default 2), `--mode auto\|hitl` (default auto), `--depends-on ID`, `--required-by ID`, `--related-to ID` (link flags repeat) |
| `pt list` | `--type T`, `--status S`. Sorted by priority, closed last |
| `pt show <id>` | One item, its links, and each linked item's status |
| `pt update <id>` | `--title`, `--type`, `--status`, `--priority`, `--mode`. A new title renames the file |
| `pt frontier` | Items ready to work on |
| `pt sync` | Repairs links and file names, rebuilds `INDEX.md`, `FRONTIER.md`, `HEALTH.md`. Run it after editing item files by hand; every writing command already runs it |
| `pt link <id>` / `pt unlink <id>` | Exactly one of `--depends-on ID`, `--required-by ID`, `--related-to ID` per call |
| `pt insert <type> <title> --after A --before B` | New item that depends on A; B then depends on it, dropping B's direct link to A. Both flags required. Also `--priority`, `--mode` |
| `pt batch <file\|->` | Create many items from a JSON array, all or nothing. `--dry-run` validates and prints the plan |
| `pt section get <id> <heading>` | Print one section |
| `pt section set <id> <heading>` | Replace a section with text from stdin, or from `--file PATH` (the file is left in place) |
| `pt delete <id>` | Without `-y`, previews what would go. `-y` removes the item and strips links to it |
| `pt version` | Also `--version`, `-v` |

IDs are 8 lowercase Crockford base32 characters. Any unique prefix works: `pt show 1x4` resolves `1x4fhcy2`.

## Values

- Types: `bug`, `chore`, `debate`, `feature`, `feedback`, `milestone`, `question`, `research`, `spike`, `task`.
- Statuses: `open`, `in_progress`, `review`, `blocked`, `closed`.
- Priority: `0` (highest) to `4`.
- Mode: `auto` or `hitl`. `pt` stores it and never filters on it, so a `hitl` item still appears on the frontier.

## Sections

`pt section get/set` match the exact, case-sensitive heading text without the leading `#` characters: `pt section set <id> "Acceptance Criteria"`. A match runs to the next heading of the same or higher level and includes subheadings, so setting `Round 1` replaces `Round 1 for` and `Round 1 against` with it. A missing or duplicated heading exits 1. Headings inside fenced code blocks are ignored.

Multiline text: pipe it on stdin, write it to a temp file and pass `--file`, or use the `pt_section_set` tool.

```text
pt section set <id> "Outcome" --file outcome.md
```

## Batch input

```json
[
  {"key": "a", "type": "task", "title": "Parse"},
  {"key": "b", "type": "bug", "title": "Crash", "dependsOn": ["a"]}
]
```

- Each entry needs `type` and `title`. Optional: `key`, `priority`, `mode`, `dependsOn`, `requiredBy`, `relatedTo`.
- Links name another entry's `key` or an existing item ID.
- The whole array is validated first; one bad entry creates nothing.
- After a successful run from a file, `pt batch` deletes that file. Stdin and `--dry-run` delete nothing.

## Attachments

Put artifacts for an item in `<id>/data/` inside the item root. `pt` leaves the folder alone and `pt delete -y` removes it with the item. Each item folder holds exactly one `.md` file at its top level.

## Native Pi tools

Inside a papertrail workspace, Pi exposes `pt_section_set` (write one section) and `pt_batch` (create many items). Use them instead of shell quoting for multiline text or JSON.

## Section headings per type

Copy headings exactly. Case differs between types.

| Type | Headings |
|---|---|
| `feature` | `Goal`, `Acceptance criteria`, `Non-goals`, `Notes` |
| `task` | `Goal`, `Acceptance Criteria`, `Notes` |
| `chore` | `Goal`, `Acceptance Criteria`, `Notes` |
| `bug` | `Problem`, `Reproduction`, `Expected behaviour`, `Resolution`, `Notes` |
| `milestone` | `Notes` |
| `spike` | `Question`, `Approach`, `Outcome`, `Notes` |
| `research` | `Question`, `Findings`, `Conclusion`, `Notes` |
| `question` | `Question`, `Answer`, `Notes` |
| `debate` | `Question`, `Round 1` (`Round 1 for`, `Round 1 against`), `Concessions`, `Round 2` (`Round 2 for`, `Round 2 against`), `Arbitrator Settlements`, `Notes` |
| `feedback` | `Source` (`Verbatim`, `Details`), `Discussion settled details`, `Notes` |

`feature` uses `Acceptance criteria` (lowercase c); `task` and `chore` use `Acceptance Criteria`. Level-3 headings (`Round 1 for`, `Verbatim`, and so on) are subsections of the level-2 heading above them.
