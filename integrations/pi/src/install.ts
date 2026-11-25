import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, dirname, isAbsolute, join } from "node:path";
import type { Exec, ExecResult } from "./session.ts";

export interface InstallParams {
	/** Base repository URL for the pt release; defaults to the script default. */
	url?: string;
	/** Release tag; defaults to latest. */
	tag?: string;
	/** Install directory; defaults to %LOCALAPPDATA%\papertrail\bin on Windows, ~/.papertrail/bin elsewhere. */
	dir?: string;
}

export interface SubprocessOutcome {
	ok: boolean;
	report: string;
}

export type FetchText = (url: string, signal: AbortSignal) => Promise<string>;

const SCRIPT_BASE_URL = "https://github.com/grainologic/papertrail/releases/latest/download";
const FETCH_TIMEOUT_MS = 30_000;
const INSTALL_TIMEOUT_MS = 120_000;
export const PROBE_TIMEOUT_MS = 5_000;

/**
 * Run `command args` in `cwd`. Never throws: a spawn error, a timeout, or a non-zero exit becomes a failed outcome with a user-facing report.
 */
export async function runSubprocess(exec: Exec, command: string, args: string[], cwd: string, label: string, timeoutMs: number): Promise<SubprocessOutcome> {
	let result: ExecResult;
	try {
		result = await exec(command, args, { cwd, timeout: timeoutMs });
	} catch (error) {
		return { ok: false, report: `${label} could not start: ${errorMessage(error)}` };
	}
	if (result.killed) {
		return { ok: false, report: `${label} was terminated after ${timeoutMs} ms; likely a timeout` };
	}
	if (result.code !== 0) {
		return { ok: false, report: `${label} exited ${result.code}:\n${outputTail(result)}` };
	}
	return { ok: true, report: result.stdout.trim() };
}

/** Remote URL for the install script matching `platform`. */
export function scriptUrl(platform: NodeJS.Platform = process.platform): string {
	return `${SCRIPT_BASE_URL}/${platform === "win32" ? "install.ps1" : "install.sh"}`;
}

/** Command and arguments to run `scriptPath` for `platform`. */
export function installInvocation(scriptPath: string, platform: NodeJS.Platform = process.platform): { command: string; args: string[] } {
	// -ExecutionPolicy Bypass lets the script run under a Restricted system policy (common on Windows) without changing machine policy.
	if (platform === "win32") return { command: "powershell", args: ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath] };
	return { command: "bash", args: [scriptPath] };
}

/** The install script's parameter flags for `params`, matching the platform dialect. */
export function installFlagArgs(params: InstallParams, platform: NodeJS.Platform = process.platform): string[] {
	const isWin = platform === "win32";
	const args: string[] = [];
	if (params.url !== undefined) args.push(isWin ? "-Url" : "--url", params.url);
	if (params.tag !== undefined) args.push(isWin ? "-Tag" : "--tag", params.tag);
	if (params.dir !== undefined) args.push(isWin ? "-Dir" : "--dir", params.dir);
	return args;
}

/** Fetch the install script to a temp file, run it with `params`, and delete the temp dir. Never throws. */
export async function runInstall(
	exec: Exec,
	params: InstallParams,
	cwd: string,
	timeoutMs: number = INSTALL_TIMEOUT_MS,
	fetchText: FetchText = defaultFetchText,
): Promise<SubprocessOutcome> {
	const url = scriptUrl();
	let scriptBody: string;
	try {
		scriptBody = await fetchText(url, AbortSignal.timeout(FETCH_TIMEOUT_MS));
	} catch (error) {
		return { ok: false, report: `pt install could not fetch ${url}: ${errorMessage(error)}` };
	}

	const tmpDir = mkdtempSync(join(tmpdir(), "papertrail-install-"));
	try {
		const scriptName = process.platform === "win32" ? "install.ps1" : "install.sh";
		const scriptPath = join(tmpDir, scriptName);
		writeFileSync(scriptPath, scriptBody, "utf8");
		const { command, args } = installInvocation(scriptPath);
		const outcome = await runSubprocess(exec, command, [...args, ...installFlagArgs(params)], cwd, "pt install", timeoutMs);
		if (outcome.ok) addInstalledDirToProcessPath(outcome.report);
		return outcome;
	} finally {
		rmSync(tmpDir, { recursive: true, force: true });
	}
}

async function defaultFetchText(url: string, signal: AbortSignal): Promise<string> {
	const response = await fetch(url, { signal });
	if (!response.ok) {
		throw new Error(`HTTP ${response.status} ${response.statusText}`.trim());
	}
	return response.text();
}

function addInstalledDirToProcessPath(report: string): void {
	const match = /^installed\s+(.+)$/m.exec(report);
	if (!match?.[1]) return;
	const binDir = dirname(match[1].trim());
	if (!isAbsolute(binDir)) return;
	const current = process.env.PATH ?? "";
	const entries = current.split(delimiter).filter((entry) => entry.length > 0);
	const normalized = process.platform === "win32" ? binDir.toLowerCase() : binDir;
	const exists = entries.some((entry) => (process.platform === "win32" ? entry.toLowerCase() : entry) === normalized);
	if (!exists) {
		process.env.PATH = [binDir, ...entries].join(delimiter);
	}
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

function outputTail(result: ExecResult): string {
	const combined = `${result.stderr.trim()}\n${result.stdout.trim()}`.trim();
	return combined.length === 0 ? "(no output)" : combined.split("\n").slice(-5).join("\n");
}
