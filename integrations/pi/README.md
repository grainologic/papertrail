# pi-papertrail

Pi package (`pi-papertrail`) for papertrail, the markdown work tracker. See the [papertrail README](../../README.md) and [CLI reference](../../docs/CLI.md) for the item model and commands.

## Install

Install from npm:

```bash
pi install npm:pi-papertrail
```

Or install from a local clone of the `papertrail` repository:

```bash
git clone https://github.com/grainologic/papertrail.git
pi install ./papertrail/integrations/pi
```

Add `-l` to write the package entry to `.pi/settings.json` for a single project instead of `~/.pi/agent/settings.json`.

## Installing `pt` and initializing a workspace

Once the extension is loaded, you or the agent can install `pt` and initialize a workspace without leaving pi:

- `/papertrail install [--url URL] [--tag TAG] [--dir DIR]` (or the `papertrail_install` tool): fetches and runs `install.sh` or `install.ps1` from GitHub, installs `pt` to `~/.papertrail/bin` (Linux/macOS) or `%LOCALAPPDATA%\papertrail\bin` (Windows), adds that directory to `PATH`, and updates the current pi session's `PATH`.
- `/papertrail init` (or the `papertrail_init` tool): runs `pt init` in the current working directory. If `pt` is not installed yet, it runs the installer first.

The `/papertrail` command provides tab completion for both `/papertrail` and its `init` / `install` subcommands.

## What it does during a session

- **`session_start`:** walks up from the working directory looking for `.papertrail/config.toml`; when found, runs `pt sync` (10 second timeout) and notifies on failure.
- **`before_agent_start`:** resolves the configured item root (`root` in `.papertrail/config.toml`, default `.papertrail`) and appends a system prompt block with the item root, path to `FRONTIER.md`, command summary, pointer to the `papertrail` skill, and the first 50 lines of `HEALTH.md`.
- **Native Pi tools:** inside a papertrail workspace Pi exposes `pt_section_set` (replace one item section with multiline content) and `pt_batch` (create many items from one JSON array). They are gated on the workspace: `session_start` and `before_agent_start` add them to the active tool set only when `.papertrail/config.toml` is found above the working directory, and scrub them again when it is not.
- **Skill (`src/skills/papertrail/`):** the project-management doctrine. `SKILL.md` sets the working rules for picking work, types, statuses, and handoffs; the reference guides go deeper (`cli.md`, `decomposition.md`, `debate.md`, `spike.md`, `research.md`, `feedback.md`). `references/cli.md` is the authoritative command, flag, and value reference. Agents pick work from `FRONTIER.md`, set `in_progress`, write results via `pt section set`, and close items with outcomes.

## Development

Run typecheck and tests inside `integrations/pi/`:

```bash
npm run check
npm test
```