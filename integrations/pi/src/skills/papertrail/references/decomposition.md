# Decomposition

An item is finished by one session or it is split. A session is one agent run with one working context; if finishing needs memory of more than that run holds, split.

## Split an item

1. Decide the natural pieces and check each can end closed: outcome written, acceptance met, one session.
2. Create the children in one `pt batch` / `pt_batch` call (or `pt create`), linking them with `--depends-on` so the parent stays open until every child closes. Dry-run first with `pt batch --dry-run`.
3. Write the split into the parent item: what was carved out and why, in the item's own sections.
4. Close the parent only when its last dependency closes. A milestone is exactly this: it closes when everything it depends on is closed.

A prerequisite found mid-flight goes between existing items with `pt insert <type> <title> --after A --before B`.

## A choice only the user can make

Create a `question` item with `mode: hitl` (`pt create question "<title>" --mode hitl`). Write the options and what each costs under `## Question`, and make the affected work depend on it with `--depends-on`. Blocked-on-user work shows up on the frontier instead of stalling invisibly. `pt` does not filter on `hitl`, so the item still appears on the frontier: do not answer it yourself.

Say so to the user in the same turn; the item is the record, the mention is the prompt. When the user answers, write the answer verbatim under `## Answer` and close the question.

## Done

Every resulting item can be finished by one session, the parent's `dependsOn` covers all of them, and each open user choice is a `hitl` `question` item that the affected work depends on.
