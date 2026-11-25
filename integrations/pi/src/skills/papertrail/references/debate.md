# Debate

A debate settles a contested decision. One context argues one side only; no single context argues both sides.

Template headings, in order: `Question`, `Round 1` (with `Round 1 for` and `Round 1 against`), `Concessions`, `Round 2` (with `Round 2 for` and `Round 2 against`), `Arbitrator Settlements`, `Notes`. `pt section set <id> "<heading>"` matches the exact, case-sensitive heading text without the leading `#`: `pt section set <id> "Round 1 for"`. Setting `Round 1` replaces its two subsections too.

## Run the debate

1. Create a `debate` item and write the decision under `## Question`, stated so that "for" and "against" are well-defined.
2. Dispatch three separate contexts (subagent, peer agent, or whatever the harness offers): one argues for, one argues against, one arbitrates. Each gets only the question and its role.
3. Write each side's argument under its round heading (`Round 1 for`, `Round 1 against`). Then write what each side now accepts under `Concessions`, and dispatch round 2 against those concessions (`Round 2 for`, `Round 2 against`).
4. Arbitration is a separate context that reads the item and rules; it never argued a side. Write its ruling under `Arbitrator Settlements`.
5. Write the decision into the item before any work continues on it. A decision made in conversation goes into an item first; the debate item is that item. Make affected items depend on it with `--depends-on`.

## Done

`Arbitrator Settlements` holds the ruling, `Concessions` and the rounds hold the arguments, affected items depend on the debate item, and the debate item is `closed`. A `mode: hitl` debate moves to `review` until the human confirms the ruling.
