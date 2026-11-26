import { join } from "node:path";

const MAX_HEALTH_LINES = 50;
const TYPES = "bug, chore, debate, feature, feedback, milestone, question, research, spike, task";
const STATUSES = "open, in_progress, review, blocked, closed";

/**
 * System prompt block describing pt, its commands, valid values, and where FRONTIER.md lives.
 * `health` is the raw item-root HEALTH.md content, or null when unreadable.
 */
export function buildPromptBlock(itemRoot: string, health: string | null): string {
	const findings = health === null ? "HEALTH.md could not be read." : capHealth(health).trim() || "No findings.";
	return [
		"## Papertrail",
		"",
		`This workspace is tracked with \`pt\`, a local markdown work tracker rooted at ${itemRoot}. Track all work as pt items.`,
		`Pick work from ${join(itemRoot, "FRONTIER.md")} (unclosed items whose dependencies are closed). Read ${join(itemRoot, "HEALTH.md")} when it reports findings.`,
		"Run `pt --help` for the command list and `pt help <command>` for a command's flags; never guess flags. Every command accepts `--json`, and item IDs accept any unique prefix.",
		`Types: ${TYPES}. Statuses: ${STATUSES}. Priority: 0-4 (default 2). Mode: auto|hitl; do not execute hitl items unilaterally.`,
		"Prefer the native `pt_*` tools to hand-editing: pt_create, pt_show, pt_list, pt_update, pt_delete, pt_link, pt_unlink, pt_insert, pt_frontier, pt_sync, pt_section_get, pt_where, pt_section_set, and pt_batch shell out to `pt ... --json`, so flags and validation stay correct. Use `pt_section_set` for multiline section text and `pt_batch` for creating many items. Run `pt sync` after editing item markdown by hand.",
		"If `pt` is missing, bootstrap with the papertrail_install and papertrail_init tools, or the `/papertrail` command (subcommands install and init).",
		"The papertrail skill holds the management rules; load it before creating, splitting, or closing items.",
		"",
		"HEALTH.md findings:",
		findings,
	].join("\n");
}

/** Keep HEALTH.md short in the system prompt; state how much was cut. */
function capHealth(health: string): string {
	const lines = health.split("\n");
	if (lines.length <= MAX_HEALTH_LINES) return health;
	return `${lines.slice(0, MAX_HEALTH_LINES).join("\n")}\n... (${lines.length - MAX_HEALTH_LINES} more lines truncated)`;
}