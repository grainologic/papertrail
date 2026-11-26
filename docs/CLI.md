# CLI reference

```pwsh
`pt <command> [args]`
```
Every command takes `--json` for structured output, errors included. `pt help <command>` or `pt <command> --help` prints a command's flags.

> [!NOTE]
> `pt` walks up from wherever its invoked, to find `.papertrail/config.toml`.

## Commands

| Command | What it does |
| --- | --- |
| `pt init` | Create `.papertrail/`. Fails if it exists. |
| `pt where` | Print the item root directory. |
| `pt create <type> <title>` | Create an item. `--priority 0-4`, `--mode auto\|hitl`, repeatable `--depends-on`/`--required-by`/`--related-to ID`. Prints ID and path. |
| `pt list [--type T] [--status S]` | List items, priority order, closed last. |
| `pt show <id>` | One item with its links and their statuses. |
| `pt update <id>` | `--title`, `--type`, `--status`, `--priority`, `--mode`. Retitle renames the file. |
| `pt frontier` | Ready items: `open`/`in_progress` with all `dependsOn` closed; priority, then created. |
| `pt sync` | Repair slugs, headings, and links; rebuild `INDEX.md`, `FRONTIER.md`, `HEALTH.md`; report findings. Runs after every write. |
| `pt link <id>` / `pt unlink <id>` | Add / remove one `--depends-on`, `--required-by`, or `--related-to ID`. |
| `pt insert <type> <title> --after A --before B` | Insert an item between A and B in a dependency chain. |
| `pt batch <file\|->` | Create items from a JSON array (below). `--dry-run` validates only. |
| `pt section get <id> <heading>` | Print one body section. |
| `pt section set <id> <heading> [--file PATH]` | Replace one body section from stdin or `--file`. |
| `pt delete <id> [-y]` | Preview removal; `-y` deletes the folder and strips links. |
| `pt version` | Print the version (`--version`, `-v`). |
| `pt help [command]` | Help for `pt` or one command. |

> [!TIP]
> `id`s match by any unique prefix. For example, `pt show 1x4` works like `pt show 1x4fhcy2`

## Batch input

A JSON array. Each entry needs `type` and `title`, and may set `key`, `priority`, `mode`, `dependsOn`, `requiredBy`, `relatedTo`. A link names another entry's `key` or an existing ID. The whole array is validated before any write, so one bad entry creates nothing. On success the input file is deleted; `-` reads stdin.

```json
[
  {"key": "a", "type": "task", "title": "Parse"},
  {"key": "b", "type": "bug", "title": "Crash", "dependsOn": ["a"]}
]
```

## Updating the paper trail

Calling `pt sync` will repair the work item graph, and refresh the index. Calling `pt frontier` will print the '**frontier**', i.e. work items that are unblocked and ready to be worked on.

> [!NOTE]
> Certain commands like creation and link updating update the frontier and index automatically. Integrations run it automatically and at certain points, so the LLM will generally be good at keeping this up to date.
