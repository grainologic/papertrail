<div align="center">

# papertrail

<img src="resources/papertrail.png" width="120" align="center" alt="">

#### A task tracker for LLM workers, kept in plain markdown files you can read too.

</div>

How many times have you used a harness, and after a while you realise that your agents have been going down a rabbit hole of one single task, and have forgotten the bigger picture?

Agents lose context. Humans lose track of what the agents did. Papertrail gives both of you the same place to look: one markdown file per work item.

## At a glance

Essentially, papertrail generates forms for LLMs to fill out. What the work is, what does it depend on. The rest is just details.

### The good news is: you don't have to do ANY of this manually

If you don't want to, that is. The [CLI](docs/CLI.md) is expressive enough that your LLM harness can handle it. And its simple enough that you can use it. Right now, [Pi](https://pi.dev/) has first class support for using papertrail natively, more coming soon.

## What does it look like?

```pwsh
pt init
initialized .papertrail

pt create task "Write parser" --priority 1
1x4fhcy2 /path/to/project/.papertrail/1x4fhcy2/write-parser.md

pt create bug "Fix crash" --depends-on 1x4fhcy2
bz84zhp4 /path/to/project/.papertrail/bz84zhp4/fix-crash.md

pt list
1x4fhcy2  p1  task       open         Write parser
bz84zhp4  p2  bug        open         Fix crash

pt frontier
1x4fhcy2  p1  task       Write parser

pt sync
sync ok
```

The frontier is the list of items you can start now. The bug stays off it until the parser closes. (`create` prints an absolute path; it is shortened here.)

## What is a work item?

Any cohesive unit of work is a work item. Papertrail generates it from different templates for different *types* of work, stored as one markdown file each. Its kind (`task`, `bug`, `feature`, `research`, `spike`, `debate`, `feedback`, `question`, `chore`, or `milestone`) is stored in its `type` field.

Each item lives in its own folder under `.papertrail/<id>/<slug>.md`, with an optional `<id>/data/` folder for attachments or artifacts that `pt` leaves alone. Here is the file behind the sample `Write parser` work item:

```markdown
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
# Write parser

## Goal

## Acceptance Criteria

## Notes
```

Each file has two parts:

- **Frontmatter.**: The block tracks `id`, `title`, `type`, `status`, `priority`, `mode`, `created`, and link lists (`dependsOn`, `requiredBy`, `relatedTo`).
- **The body is yours.** After the `# Title` line and an optional overview, `pt` adds type-specific `##` headings when creating the item (for example, `Goal` and `Acceptance Criteria` for a `task`).

See [docs/WORKITEM.md](docs/WORKITEM.md) for the full frontmatter and structure breakdown.

> [!TIP]
> Templates are generated from template markdown files per type, found [here](internal/template/templates/).

### Why markdown?

- **Readable by both LLMs and humans.** Agents generate and parse markdown natively, and you can open any item in an editor or code review without a web UI or an export step.
- **Lives right next to the code.** Because every item is a plain file under `.papertrail/`, decisions, reproduction steps, and settled answers travel with the project and show up in ordinary text search (`rg`, `grep`) and diffs.
- **Hand-editable and tool-backed.** You can edit an item directly in an editor or drive it through `pt`. Running `pt sync` afterward reconciles stale file names, `# Title` headings, reverse links, and the `INDEX.md`, `FRONTIER.md`, and `HEALTH.md` summary views.
- **Keeps working when tooling is absent.** If an agent switches harnesses or you want to triage without running `pt`, work continues by reading and editing the markdown files directly.

## Install

<details>
<summary>Linux / macOS</summary>

Installs to `~/.papertrail/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/grainologic/papertrail/main/install.sh | bash
```

Pass flags (`--url`, `--tag`, `--dir`, `--dry-run`, `--help`) after `bash -s --`:

```bash
curl -fsSL https://raw.githubusercontent.com/grainologic/papertrail/main/install.sh | bash -s -- --tag v1.0.0
```
</details>

<details>
<summary>Windows</summary>

Installs to `%LOCALAPPDATA%\papertrail\bin`.

PowerShell:

```powershell
irm https://raw.githubusercontent.com/grainologic/papertrail/main/install.ps1 | iex
```

Pass flags (`-Url`, `-Tag`, `-Dir`, `-DryRun`, `-Help`) to the scriptblock:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/grainologic/papertrail/main/install.ps1))) -Tag v1.0.0
```

Git Bash / MSYS2 / Cygwin:

```bash
curl -fsSL https://raw.githubusercontent.com/grainologic/papertrail/main/install.sh | bash
```
</details>

<details>
<summary>Via Pi coding agent</summary>

Install the `pi-papertrail` package:

```bash
pi install npm:pi-papertrail
```

Then install `pt` (or initialize a workspace directly, which installs `pt` if missing) inside pi with `/papertrail install` or `/papertrail init`, or ask the agent to run `papertrail_install` or `papertrail_init`. See [integrations/pi/](integrations/pi/) for local-path installation and full extension details.
</details>

> The install scripts fetch the release binary for your OS and architecture and add the install directory to your `PATH` if it is not already there (restart open shells to pick it up).

Check it worked:

```sh
pt --version
pt v1.0.0
```

> [!TIP]
> To build from source with Go, see [Building from source](#building-from-source).

## CLI reference

See [docs/CLI.md](docs/CLI.md) for the full command reference: every command's usage and flags, `--json` and exit codes, ID prefixes, link kinds, and the batch input format.

## Integrations

- [pi](integrations/pi/)

## Development

### Building from source

Requires Go 1.25 or newer:

```bash
git clone https://github.com/grainologic/papertrail.git && cd papertrail
go install ./cmd/pt
```

`go install` puts `pt` in your Go bin directory. `go build ./cmd/pt` leaves a `pt` binary in the current directory instead (`pt.exe` on Windows).

### Tests

```bash
go test ./...
```
