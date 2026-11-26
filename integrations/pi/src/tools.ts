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
const TOOL_TIMEOUT_MS = 10_000;

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

export interface CreateParams {
	type: string;
	title: string;
	priority?: number;
	mode?: string;
	dependsOn?: string[];
	requiredBy?: string[];
	relatedTo?: string[];
}

export interface ShowParams {
	id: string;
}

export interface ListParams {
	type?: string;
	status?: string;
}

export interface UpdateParams {
	id: string;
	title?: string;
	type?: string;
	status?: string;
	priority?: number;
	mode?: string;
}

export interface DeleteParams {
	id: string;
	confirm?: boolean;
}

export interface LinkParams {
	id: string;
	dependsOn?: string;
	requiredBy?: string;
	relatedTo?: string;
}

export interface InsertParams {
	type: string;
	title: string;
	after: string;
	before: string;
	priority?: number;
	mode?: string;
}

export interface SectionGetParams {
	id: string;
	heading: string;
}

/** Repeatable link flags for an array of IDs, appended in order. */
function repeatLinkFlags(name: string, ids?: string[]): string[] {
	const args: string[] = [];
	for (const id of ids ?? []) args.push(name, id);
	return args;
}

/** The single --link flag for link/unlink, or an empty argv when none is set (the CLI then rules it a usage error). */
function singleLinkFlags(params: LinkParams): string[] {
	if (params.dependsOn !== undefined) return ["--depends-on", params.dependsOn];
	if (params.requiredBy !== undefined) return ["--required-by", params.requiredBy];
	if (params.relatedTo !== undefined) return ["--related-to", params.relatedTo];
	return [];
}

/** Create a new pt item from type and title, with optional priority, mode, and repeatable link flags. Never throws. */
export async function runCreate(exec: Exec, params: CreateParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	const args = ["create", params.type, params.title];
	if (params.priority !== undefined) args.push("--priority", String(params.priority));
	if (params.mode !== undefined) args.push("--mode", params.mode);
	args.push(...repeatLinkFlags("--depends-on", params.dependsOn));
	args.push(...repeatLinkFlags("--required-by", params.requiredBy));
	args.push(...repeatLinkFlags("--related-to", params.relatedTo));
	args.push("--json");
	return runSubprocess(exec, "pt", args, cwd, "pt create", timeoutMs);
}

/** Show one pt item by ID prefix. Never throws. */
export async function runShow(exec: Exec, params: ShowParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	return runSubprocess(exec, "pt", ["show", params.id, "--json"], cwd, "pt show", timeoutMs);
}

/** List pt items, filtered by type and status when given. Never throws. */
export async function runList(exec: Exec, params: ListParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	const args = ["list"];
	if (params.type !== undefined) args.push("--type", params.type);
	if (params.status !== undefined) args.push("--status", params.status);
	args.push("--json");
	return runSubprocess(exec, "pt", args, cwd, "pt list", timeoutMs);
}

/** Update a pt item's fields by ID prefix. Only provided fields change; --title renames the file. Never throws. */
export async function runUpdate(exec: Exec, params: UpdateParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	const args = ["update", params.id];
	if (params.title !== undefined) args.push("--title", params.title);
	if (params.type !== undefined) args.push("--type", params.type);
	if (params.status !== undefined) args.push("--status", params.status);
	if (params.priority !== undefined) args.push("--priority", String(params.priority));
	if (params.mode !== undefined) args.push("--mode", params.mode);
	args.push("--json");
	return runSubprocess(exec, "pt", args, cwd, "pt update", timeoutMs);
}

/** Delete a pt item by ID prefix: preview without confirm, actually delete with confirm=true (-y). Never throws. */
export async function runDelete(exec: Exec, params: DeleteParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	const args = ["delete", params.id];
	if (params.confirm) args.push("-y");
	args.push("--json");
	return runSubprocess(exec, "pt", args, cwd, "pt delete", timeoutMs);
}

/** Add exactly one link to a pt item by ID prefix. Never throws. */
export async function runLink(exec: Exec, params: LinkParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	return runSubprocess(exec, "pt", ["link", params.id, ...singleLinkFlags(params), "--json"], cwd, "pt link", timeoutMs);
}

/** Remove exactly one link from a pt item by ID prefix. Never throws. */
export async function runUnlink(exec: Exec, params: LinkParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	return runSubprocess(exec, "pt", ["unlink", params.id, ...singleLinkFlags(params), "--json"], cwd, "pt unlink", timeoutMs);
}

/** Insert a new pt item between two items: it depends on after, and before depends on it. Never throws. */
export async function runInsert(exec: Exec, params: InsertParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	const args = ["insert", params.type, params.title, "--after", params.after, "--before", params.before];
	if (params.priority !== undefined) args.push("--priority", String(params.priority));
	if (params.mode !== undefined) args.push("--mode", params.mode);
	args.push("--json");
	return runSubprocess(exec, "pt", args, cwd, "pt insert", timeoutMs);
}

/** Run the pt frontier read: items ready to work on. Never throws. */
export async function runFrontier(exec: Exec, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	return runSubprocess(exec, "pt", ["frontier", "--json"], cwd, "pt frontier", timeoutMs);
}

/** Run the pt sync pass, which repairs links and file names and rebuilds the generated files. Health findings are data, not a failure. Never throws. */
export async function runSyncTool(exec: Exec, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	return runSubprocess(exec, "pt", ["sync", "--json"], cwd, "pt sync", timeoutMs);
}

/** Print one section of a pt item, matched by exact heading. Never throws. */
export async function runSectionGet(exec: Exec, params: SectionGetParams, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	return runSubprocess(exec, "pt", ["section", "get", params.id, params.heading, "--json"], cwd, "pt section get", timeoutMs);
}

/** Run `pt where`: print the resolved pt item root directory. Never throws. */
export async function runWhere(exec: Exec, cwd: string, timeoutMs: number = TOOL_TIMEOUT_MS): Promise<ToolOutcome> {
	return runSubprocess(exec, "pt", ["where", "--json"], cwd, "pt where", timeoutMs);
}