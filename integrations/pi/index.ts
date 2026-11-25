import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { AutocompleteItem } from "@earendil-works/pi-tui";
import { Type } from "typebox";
import { findPapertrailRoot } from "./src/paths.ts";
import { readItemRoot } from "./src/config.ts";
import { readHealth, runSync, type Exec } from "./src/session.ts";
import { buildPromptBlock } from "./src/prompt.ts";
import { runInstall, type FetchText, type InstallParams } from "./src/install.ts";
import { runInit } from "./src/init.ts";
import { runSectionSet, runBatch } from "./src/tools.ts";

const SETUP_SUBCOMMANDS = ["init", "install"] as const;
const WORKSPACE_TOOLS: readonly string[] = ["pt_section_set", "pt_batch"];

/** Adapt pi.exec to the narrow Exec seam the setup commands accept. */
function piExec(pi: ExtensionAPI): Exec {
	return (command, args, options) => pi.exec(command, args, options);
}

/** Parse command-line install flags after the subcommand. */
function parseInstallArgs(input: string): InstallParams {
	const tokens = input.split(/\s+/).filter((t) => t.length > 0);
	const params: InstallParams = {};
	for (let i = 0; i < tokens.length; i++) {
		const flag = tokens[i]!;
		const value = tokens[i + 1];
		if ((flag === "--url" || flag === "-Url") && value !== undefined) {
			params.url = value;
			i++;
		} else if ((flag === "--tag" || flag === "-Tag") && value !== undefined) {
			params.tag = value;
			i++;
		} else if ((flag === "--dir" || flag === "-Dir") && value !== undefined) {
			params.dir = value;
			i++;
		}
	}
	return params;
}

/** Keep the workspace tools active inside a papertrail workspace and out of the loadout elsewhere. Only calls setActiveTools when the active set actually changes. */
function syncActiveTools(pi: ExtensionAPI, hasWorkspace: boolean): void {
	if (typeof pi.getActiveTools !== "function" || typeof pi.setActiveTools !== "function") return;
	const active = pi.getActiveTools();
	if (hasWorkspace) {
		const missing = WORKSPACE_TOOLS.filter((tool) => !active.includes(tool));
		if (missing.length === 0) return;
		pi.setActiveTools([...active, ...missing]);
		return;
	}
	const scrubbed = active.filter((tool) => !WORKSPACE_TOOLS.includes(tool));
	if (scrubbed.length === active.length) return;
	pi.setActiveTools(scrubbed);
}

export default function papertrailExtension(pi: ExtensionAPI, fetchText?: FetchText): void {
	const exec = piExec(pi);

	pi.on("session_start", async (_event, ctx) => {
		const root = findPapertrailRoot(ctx.cwd);
		syncActiveTools(pi, root !== null);
		if (root === null) return;
		const outcome = await runSync(exec, root);
		if (!outcome.ok && ctx.hasUI) ctx.ui.notify(`papertrail: ${outcome.report}`, "warning");
	});

	pi.on("before_agent_start", async (event, ctx) => {
		const root = findPapertrailRoot(ctx.cwd);
		syncActiveTools(pi, root !== null);
		if (root === null) return;
		const itemRoot = readItemRoot(root);
		return { systemPrompt: `${event.systemPrompt}\n\n${buildPromptBlock(itemRoot, readHealth(itemRoot))}` };
	});

	pi.registerCommand("papertrail", {
		description: "Bootstrap papertrail: install the pt binary or initialize a workspace",
		getArgumentCompletions: (prefix: string): AutocompleteItem[] | null => {
			const matches = SETUP_SUBCOMMANDS.filter((sub) => sub.startsWith(prefix));
			return matches.length === 0 ? null : matches.map((value) => ({ value, label: value }));
		},
		handler: async (args, ctx) => {
			const sub = args.trim().split(/\s+/)[0] ?? "";
			if (sub === "init") {
				const outcome = await runInit(exec, ctx.cwd, (e, cwd) => runInstall(e, {}, cwd, undefined, fetchText));
				if (outcome.ok) syncActiveTools(pi, true);
				ctx.ui.notify(`papertrail: ${outcome.report}`, outcome.ok ? "info" : "warning");
				return;
			}
			if (sub === "install") {
				const outcome = await runInstall(exec, parseInstallArgs(args.slice(sub.length)), ctx.cwd, undefined, fetchText);
				ctx.ui.notify(`papertrail: ${outcome.report}`, outcome.ok ? "info" : "warning");
				return;
			}
			ctx.ui.notify("usage: /papertrail [init|install] [--url URL] [--tag TAG] [--dir DIR]", "warning");
		},
	});

	pi.registerTool({
		name: "papertrail_install",
		label: "Install pt",
		description: "Install the pt (papertrail) CLI binary from its release. papertrail_init runs this automatically when pt is missing; call it directly to install without initializing.",
		parameters: Type.Object({
			url: Type.Optional(Type.String()),
			tag: Type.Optional(Type.String()),
			dir: Type.Optional(Type.String()),
		}),
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const outcome = await runInstall(exec, params, ctx.cwd, undefined, fetchText);
			return { content: [{ type: "text", text: outcome.report }], details: { ok: outcome.ok } };
		},
	});

	pi.registerTool({
		name: "papertrail_init",
		label: "Initialize papertrail",
		description: "Initialize a papertrail workspace by running `pt init` in the current directory. Installs pt first when it is missing.",
		parameters: Type.Object({}),
		async execute(_toolCallId, _params, _signal, _onUpdate, ctx) {
			const outcome = await runInit(exec, ctx.cwd, (e, cwd) => runInstall(e, {}, cwd, undefined, fetchText));
			if (outcome.ok) syncActiveTools(pi, true);
			return { content: [{ type: "text", text: outcome.report }], details: { ok: outcome.ok, installed: outcome.installed } };
		},
	});

	pi.registerTool({
		name: "pt_section_set",
		label: "Set item section",
		description: "Replace one section of a pt item, identified by a unique ID prefix, with multiline content. The heading must match exactly and uniquely in the item body; frontmatter and every other section stay untouched. Run `pt help section set` for exact flags rather than guessing.",
		parameters: Type.Object({
			id: Type.String(),
			heading: Type.String(),
			content: Type.String(),
		}),
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const outcome = await runSectionSet(exec, params, ctx.cwd);
			return { content: [{ type: "text", text: outcome.report }], details: { ok: outcome.ok } };
		},
	});

	pi.registerTool({
		name: "pt_batch",
		label: "Batch create",
		description: "Create many pt items from one JSON array in a single transaction. Each entry has type and title plus optional key, priority, mode, dependsOn, requiredBy, and relatedTo (links by key or existing ID). All-or-nothing: any invalid entry prevents every item. Set dryRun to validate and preview the plan without writing.",
		parameters: Type.Object({
			items: Type.Array(
				Type.Object({
					type: Type.String(),
					title: Type.String(),
					key: Type.Optional(Type.String()),
					priority: Type.Optional(Type.Number()),
					mode: Type.Optional(Type.String()),
					dependsOn: Type.Optional(Type.Array(Type.String())),
					requiredBy: Type.Optional(Type.Array(Type.String())),
					relatedTo: Type.Optional(Type.Array(Type.String())),
				}),
			),
			dryRun: Type.Optional(Type.Boolean()),
		}),
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const outcome = await runBatch(exec, params, ctx.cwd);
			return { content: [{ type: "text", text: outcome.report }], details: { ok: outcome.ok } };
		},
	});
}