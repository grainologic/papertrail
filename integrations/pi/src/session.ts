import { readFileSync } from "node:fs";
import { join } from "node:path";

export interface SyncOutcome {
	/** True when `pt sync` exited zero without being terminated. */
	ok: boolean;
	/** Human-readable summary for the user; the report tail when sync failed. */
	report: string;
}

export interface ExecResult {
	stdout: string;
	stderr: string;
	code: number;
	killed: boolean;
}

export type Exec = (command: string, args: string[], options: { cwd: string; timeout: number }) => Promise<ExecResult>;

export const SYNC_TIMEOUT_MS = 10_000;

/**
 * Run `pt sync` in `root`. Never throws: a missing binary, a timeout, or a non-zero exit becomes a failed SyncOutcome so the session keeps running.
 */
export async function runSync(exec: Exec, root: string, timeoutMs: number = SYNC_TIMEOUT_MS, command: string = "pt"): Promise<SyncOutcome> {
	let result: ExecResult;
	try {
		result = await exec(command, ["sync"], { cwd: root, timeout: timeoutMs });
	} catch (error) {
		return { ok: false, report: `pt sync could not start: ${errorMessage(error)}` };
	}
	if (result.killed) {
		return { ok: false, report: `pt sync was terminated (exit ${result.code}) after ${timeoutMs} ms; likely a timeout` };
	}
	if (result.code !== 0) {
		return { ok: false, report: `pt sync exited ${result.code}:\n${outputTail(result)}` };
	}
	return { ok: true, report: result.stdout.trim() };
}

/** .papertrail item-root HEALTH.md content, or null when it cannot be read. */
export function readHealth(itemRoot: string): string | null {
	try {
		return readFileSync(join(itemRoot, "HEALTH.md"), "utf8");
	} catch {
		return null;
	}
}

function outputTail(result: ExecResult): string {
	const combined = `${result.stderr.trim()}\n${result.stdout.trim()}`.trim();
	return combined.length === 0 ? "(no output)" : combined.split("\n").slice(-5).join("\n");
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}