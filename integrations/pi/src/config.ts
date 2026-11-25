import { readFileSync } from "node:fs";
import { isAbsolute, join, resolve } from "node:path";

/** Item root used when .papertrail/config.toml is absent or has no root key. */
export const DEFAULT_ITEM_ROOT = ".papertrail";

const CONFIG_PATH = join(".papertrail", "config.toml");
// pt writes a single-key TOML: root = "<dir>". TOML also allows single-quoted literal strings, which pt accepts; match both, non-empty.
const ROOT_KEY = /^\s*root\s*=\s*(?:"([^"]+)"|'([^']+)')/m;

/**
 * The directory where items and HEALTH.md/FRONTIER.md live, resolved like pt: the root key in .papertrail/config.toml relative to the workspace root, or .papertrail when the file or key is absent. Uses a regex instead of a TOML dependency for one key; resolve normalizes trailing slashes.
 */
export function readItemRoot(workspaceRoot: string): string {
	const root = parseRoot(join(workspaceRoot, CONFIG_PATH)) ?? DEFAULT_ITEM_ROOT;
	return isAbsolute(root) ? root : resolve(workspaceRoot, root);
}

function parseRoot(configPath: string): string | null {
	let content: string;
	try {
		content = readFileSync(configPath, "utf8");
	} catch {
		return null;
	}
	const m = content.match(ROOT_KEY);
	return m ? (m[1] ?? m[2] ?? null) : null;
}