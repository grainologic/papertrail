import type { Exec } from "./session.ts";
import type { SubprocessOutcome } from "./install.ts";
import { runSubprocess, PROBE_TIMEOUT_MS } from "./install.ts";

export interface InitOutcome {
	ok: boolean;
	report: string;
	/** true when this run installed pt before initializing. */
	installed: boolean;
}

const INIT_TIMEOUT_MS = 60_000;

/**
 * Ensure pt is installed, then run `pt init` in `cwd`. `install` fetches and runs the install script when no pt binary is on PATH. Never throws.
 */
export async function runInit(exec: Exec, cwd: string, install: (exec: Exec, cwd: string) => Promise<SubprocessOutcome>, timeoutMs: number = INIT_TIMEOUT_MS): Promise<InitOutcome> {
	if (await isPtInstalled(exec, cwd)) {
		const outcome = await runSubprocess(exec, "pt", ["init"], cwd, "pt init", timeoutMs);
		return { ...outcome, installed: false };
	}
	const installed = await install(exec, cwd);
	if (!installed.ok) {
		return { ok: false, report: `pt was not installed: ${installed.report}`, installed: false };
	}
	const outcome = await runSubprocess(exec, "pt", ["init"], cwd, "pt init", timeoutMs);
	return { ...outcome, installed: true };
}

/** A pt binary that responds to `--version` exists on PATH. */
async function isPtInstalled(exec: Exec, cwd: string): Promise<boolean> {
	try {
		const result = await exec("pt", ["--version"], { cwd, timeout: PROBE_TIMEOUT_MS });
		return result.code === 0;
	} catch {
		return false;
	}
}