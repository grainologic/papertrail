import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Exec } from "./session.ts";
import { runSubprocess } from "./install.ts";

export interface SectionSetParams {
	id: string;
	heading: string;
	content: string;
}

export interface BatchItem {
	type: string;
	title: string;
	key?: string;
	priority?: number;
	mode?: string;
	dependsOn?: string[];
	requiredBy?: string[];
	relatedTo?: string[];
}

export interface BatchParams {
	items: BatchItem[];
	dryRun?: boolean;
}

export interface ToolOutcome {
	ok: boolean;
	report: string;
}

const SECTION_SET_TIMEOUT_MS = 10_000;
const BATCH_TIMEOUT_MS = 10_000;

/** Replace one item section by writing `content` to a temp file for `pt section set --file`. Never throws: runSubprocess turns a spawn error, a timeout, or a non-zero exit into a failed outcome. */
export async function runSectionSet(exec: Exec, params: SectionSetParams, cwd: string, timeoutMs: number = SECTION_SET_TIMEOUT_MS): Promise<ToolOutcome> {
	const tmpDir = mkdtempSync(join(tmpdir(), "papertrail-section-"));
	try {
		const tmpFile = join(tmpDir, "section.md");
		writeFileSync(tmpFile, params.content, "utf8");
		return await runSubprocess(exec, "pt", ["section", "set", params.id, params.heading, "--file", tmpFile, "--json"], cwd, "pt section set", timeoutMs);
	} finally {
		rmSync(tmpDir, { recursive: true, force: true });
	}
}

/** Create many items from one JSON payload written to a temp file for `pt batch`. Never throws, like runSectionSet. */
export async function runBatch(exec: Exec, params: BatchParams, cwd: string, timeoutMs: number = BATCH_TIMEOUT_MS): Promise<ToolOutcome> {
	const tmpDir = mkdtempSync(join(tmpdir(), "papertrail-batch-"));
	try {
		const tmpFile = join(tmpDir, "batch.json");
		writeFileSync(tmpFile, JSON.stringify(params.items), "utf8");
		const args = params.dryRun ? ["batch", tmpFile, "--dry-run", "--json"] : ["batch", tmpFile, "--json"];
		return await runSubprocess(exec, "pt", args, cwd, "pt batch", timeoutMs);
	} finally {
		rmSync(tmpDir, { recursive: true, force: true });
	}
}