import { existsSync } from "node:fs";
import { dirname, join } from "node:path";

/** Closest ancestor of `dir` (inclusive) holding .papertrail/config.toml, or null.
 * Matches pt's discovery: the directory alone is not a project. */
export function findPapertrailRoot(dir: string): string | null {
	let cur = dir;
	for (;;) {
		if (existsSync(join(cur, ".papertrail", "config.toml"))) return cur;
		const parent = dirname(cur);
		if (parent === cur) return null;
		cur = parent;
	}
}
