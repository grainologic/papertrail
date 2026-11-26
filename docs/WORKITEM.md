# Work items

A work item is one markdown file split into two parts:
- YAML frontmatter (mostly tool facing data)
- A markdown body (mostly user facing data)

`pt` calls read and write the frontmatter and preserves the body's bytes, changing only the `# Title` heading and the sections you edit.

## File layout

Each work item is defined by its own folder, and the `.md` file inside it, for example `.papertrail/<id>/<slug>.md`.

- `id` is 8 lowercase Crockford base32 characters, generated at creation.
- `slug` is derived from the title; `pt sync` renames the file when the title changes.
- Exactly one `.md` sits at the folder root. Attachments and artifacts go in `<id>/data/`, which `pt` never reads, and removes together with the item.

## Frontmatter

```yaml
---
id: 1x4fhcy2
title: Write parser
type: task
status: open
priority: 1
mode: auto
created: "2026-11-23T16:03:49Z"
requiredBy:
    - bz84zhp4
---
```

| Field | What it holds |
| --- | --- |
| `id` | The item's unique 8-character ID. Matches the folder name. |
| `title` | The item title. Drives the file slug and the `# Title` heading. |
| `type` | The kind of work; selects the creation template. |
| `status` | Where the item is in its lifecycle. |
| `priority` | `0` (highest) to `4`, default `2`. |
| `mode` | `auto` (default) or `hitl`, marking an item that needs a human in the loop. |
| `created` | RFC 3339 UTC timestamp, set at creation. |
| `dependsOn` | IDs of items that block this one. |
| `requiredBy` | IDs of items that depend on this one. Derived from `dependsOn` by `pt sync`. |
| `relatedTo` | IDs of loosely related items; a symmetric soft relation. |

Empty link lists are omitted when `pt` writes the file.

### Values

- type: `bug`, `chore`, `debate`, `feature`, `feedback`, `milestone`, `question`, `research`, `spike`, `task`
- status: `open`, `in_progress`, `review`, `blocked`, `closed`

`blocked` is a hand-set external blocker. An item whose `dependsOn` items are not yet closed is held back regardless of its status.

## Links

`dependsOn` is authoritative: `pt sync` recomputes every item's `requiredBy` from it, so the two ends always agree. `relatedTo` is made symmetric the same way. Dependency cycles are refused. Editing a link by hand is fine; run `pt sync` to reconcile the other end.

## Body and sections

After the frontmatter, the body starts with a `# Title` heading and an optional overview, then type-specific `##` sections seeded from the item's template, ending with `Notes`. The body is yours to write (or your LLMs).

`pt section get` and `pt section set` address a section by its heading text. A heading matches exactly and case-sensitively, must be unique, and ignores headings inside fenced code blocks. A section runs to the next heading of the same or higher level, including subheadings.