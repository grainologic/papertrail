---
name: papertrail
description: Manage work through pt items in .papertrail/. Load when starting a session, picking work, creating, splitting, updating, closing, or deleting items, or when a decision, question, debate, spike, research task, or feedback needs to be recorded.
---

# Papertrail

Work lives in `pt` items under `.papertrail/`. An item's file is the record: results, decisions, and handoff state go into its sections, never into conversation alone.

## Use the CLI's own help

- `pt --help` lists commands and item values. `pt help <command>` (or `pt <command> --help`) prints one command's flags.
- Add `--json` to any command for structured output; prefer it for reads.
- Use the native `pt_*` tools Pi exposes for every command (they shell out to `pt ... --json`, so flags and validation stay correct), especially `pt_section_set` for multiline section text and `pt_batch` for batches; else pipe stdin or use `--file`.
- Run `pt sync` after editing an item file by hand.
- Exact flags, batch schema, and the section headings per type: `references/cli.md`. Copy headings from there; case differs between types.

## Start a session

1. Read `.papertrail/HEALTH.md`. Triage errors first: an item with an error is excluded from the frontier. Warnings (an item past `open` with an unclosed dependency) deserve a look before trusting the frontier.
2. Read `.papertrail/FRONTIER.md` or run `pt frontier --json`. Pick the lowest priority number (`0` is highest). Prefer `mode: auto` items; a `hitl` item needs a human before you execute or close it.
3. An `in_progress` item may be someone's unfinished work. Run `pt show <id>` and read its file for handoff notes before continuing or restarting it.
4. Set `pt update <id> --status in_progress` when you start.

## Choose the item type

| Type | Use when |
|---|---|
| `feature` | New user-visible capability that needs goal, acceptance criteria, and non-goals |
| `task` | A concrete unit of work one session finishes |
| `chore` | Maintenance with no behaviour change: upgrades, cleanup, config |
| `bug` | Behaviour differs from expectation; record reproduction and expected behaviour |
| `milestone` | A checkpoint that closes when everything it depends on is closed |
| `spike` | Feasibility is unknown and throwaway code would answer it |
| `research` | The answer comes from reading, with no code changes |
| `question` | Only the user (or an outside party) can answer; work depends on the answer |
| `debate` | A contested decision needs for, against, and arbitration |
| `feedback` | A user or stakeholder gave input to record verbatim |

Unsure between `task` and `feature`: if you can write acceptance criteria for a user-visible change, it is a `feature`.

## Move an item through its status

`open` -> `in_progress` -> `review` -> `closed`, with `blocked` as a side state.

- `in_progress`: you are working on it now.
- `review`: the work is done and awaits verification or approval. Use it when someone other than you must check the result, or when `mode: hitl` needs a human to confirm. A `review` dependency still holds dependents back.
- `blocked`: an external blocker you cannot clear (an outage, a vendor, a missing credential). Write the cause and what unblocks it under `Notes`. `blocked` is a status note for outside causes. Use `dependsOn` when another item blocks this one, and a `question` item when you need a user decision.
- `closed`: acceptance criteria are verified and the outcome is written. Close with content; delete (`pt delete <id> -y`) only items created by mistake.

A `hitl` item is never executed or closed without human confirmation.

## Split work and link it

- `dependsOn` is a hard blocker: the item stays off the frontier until the dependency closes. `relatedTo` is a soft, symmetric link for context.
- Break a large goal into items upfront with `pt batch` / `pt_batch`: one call, all or nothing, `--dry-run` to preview.
- A prerequisite found mid-flight goes between two items: `pt insert <type> <title> --after A --before B` makes the new item depend on A and B depend on it.
- Work discovered while doing another item becomes a new item linked to the item that surfaced it. Never fix it silently.
- Put artifacts (logs, data, drafts) in `<id>/data/`.

## Hand off and verify

1. Check each acceptance criterion against the real result before moving an item to `closed`.
2. Before pausing or setting `review`, `blocked`, or `closed`, write the concrete state into the item's sections: what changed, what was verified, what remains, and the next step. A later session starts from this text alone.
3. After closing, read `FRONTIER.md` to see what the closure unblocked and pick the next item.

## Branch guides

Read the one that matches your situation:

- `references/decomposition.md`: the item is too large for one session, a user choice appeared, or a milestone is ready to close.
- `references/debate.md`: a contested decision needs for, against, and arbitration.
- `references/spike.md`: feasibility is unknown and code would answer it.
- `references/research.md`: the task is reading and reporting, with no code changes.
- `references/feedback.md`: a user or stakeholder gave feedback to record.