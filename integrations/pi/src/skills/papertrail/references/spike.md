# Spike

A spike is throwaway code answering one feasibility question. Its outcome is the deliverable; its code is not.

Template headings: `Question`, `Approach`, `Outcome`, `Notes`. `pt section set <id> "Outcome"` matches the exact, case-sensitive heading text without the leading `#`.

## Run a spike

1. Create a `spike` item whose `## Question` names the one thing unknown, in a form code can answer.
2. State the smallest experiment that answers it under `## Approach` before writing code.
3. Write the code outside the real change, run it, and record the answer under `## Outcome` with the measurement or failure that settles it.
4. Delete the spike code. It is never merged and never kept "in case". Keep logs or data worth retaining in `<id>/data/`.
5. Turn what the answer implies into work items, linked to the spike with `--related-to`, or `--depends-on` where the real work builds on the finding.
6. Close the spike once the outcome is written and the follow-up items exist.

## Done

`Outcome` holds the answer with evidence, the throwaway code is gone, follow-up items are linked, and the item is `closed`.
