import { execFile } from "node:child_process";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import assert from "node:assert/strict";
import { findPapertrailRoot } from "../src/paths.ts";
import { runSync, readHealth, SYNC_TIMEOUT_MS, type ExecResult } from "../src/session.ts";
import { buildPromptBlock } from "../src/prompt.ts";
import { readItemRoot } from "../src/config.ts";
import papertrailExtension from "../index.ts";

function tempRoot(withPapertrail: boolean): string {
	const root = mkdtempSync(join(tmpdir(), "pt-ext-"));
	if (withPapertrail) {
		mkdirSync(join(root, ".papertrail"));
		writeFileSync(join(root, ".papertrail", "config.toml"), 'root = ".papertrail/"\n');
	}
	return root;
}

function execResult(overrides: Partial<ExecResult>): ExecResult {
	return { stdout: "", stderr: "", code: 0, killed: false, ...overrides };
}

// A fake pi API that records hook handlers, notifications, and the active-tool loadout.
function installExtension(result: ExecResult = { stdout: "", stderr: "", code: 0, killed: false }) {
	const handlers = new Map<string, Function>();
	const notifications: Array<[string, string]> = [];
	const notify = (message: string, level: string) => {
		notifications.push([message, level]);
	};
	const activeTools: string[] = [];
	const pi = {
		on: (name: string, handler: Function) => {
			handlers.set(name, handler);
		},
		exec: async (): Promise<ExecResult> => result,
		ui: { notify },
		registerCommand: () => {},
		registerTool: () => {},
		getActiveTools: () => [...activeTools],
		setActiveTools: (names: string[]) => {
			activeTools.length = 0;
			activeTools.push(...names);
		},
	};
	papertrailExtension(pi as unknown as Parameters<typeof papertrailExtension>[0]);
	return { handlers, notifications, notify, activeTools };
}

test("findPapertrailRoot walks up to the .papertrail directory", () => {
	const root = tempRoot(true);
	try {
		assert.equal(findPapertrailRoot(join(root, "a", "b")), root);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("findPapertrailRoot returns null without .papertrail", () => {
	const root = tempRoot(false);
	try {
		assert.equal(findPapertrailRoot(root), null);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("runSync reports a healthy sync", async () => {
	const root = tempRoot(true);
	try {
		const outcome = await runSync(async () => execResult({ stdout: "synced 3 items" }), root);
		assert.deepEqual(outcome, { ok: true, report: "synced 3 items" });
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("runSync reports a non-zero exit without throwing", async () => {
	const root = tempRoot(true);
	try {
		const outcome = await runSync(async () => execResult({ code: 1, stderr: "invalid frontmatter: a.md" }), root);
		assert.equal(outcome.ok, false);
		assert.ok(outcome.report.includes("pt sync exited 1"));
		assert.ok(outcome.report.includes("invalid frontmatter: a.md"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("runSync reports a missing pt binary with the spawn error", async () => {
	const root = tempRoot(true);
	try {
		const outcome = await runSync(async () => {
			throw new Error("spawn pt ENOENT");
		}, root);
		assert.equal(outcome.ok, false);
		assert.ok(outcome.report.includes("spawn pt ENOENT"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

// Integration through a real fake pt script executed by node in the temp workspace.
function fakeExec(fakePt: string) {
	return (command: string, args: string[], options: { cwd: string; timeout: number }): Promise<ExecResult> => {
		assert.equal(command, "pt");
		assert.deepEqual(args, ["sync"]);
		assert.ok(options.timeout > 0 && options.timeout <= SYNC_TIMEOUT_MS);
		return new Promise((resolve) => {
			execFile(process.execPath, [fakePt, ...args], { cwd: options.cwd, timeout: options.timeout, windowsHide: true }, (error, stdout, stderr) => {
				const err = error as (NodeJS.ErrnoException & { killed?: boolean }) | null;
				const code = typeof err?.code === "number" ? err.code : 0;
				resolve({ stdout: String(stdout), stderr: String(stderr), code, killed: err?.killed === true });
			});
		});
	};
}

test("runSync executes a fake pt script and reports its stdout", async () => {
	const root = tempRoot(true);
	try {
		const fakePt = join(root, "fake-pt.mjs");
		writeFileSync(fakePt, 'process.stdout.write("synced");\n');
		const outcome = await runSync(fakeExec(fakePt), root);
		assert.deepEqual(outcome, { ok: true, report: "synced" });
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("runSync reports a real timeout from a slowly-returning fake pt", async () => {
	const root = tempRoot(true);
	try {
		const fakePt = join(root, "sleep-pt.mjs");
		writeFileSync(fakePt, "await new Promise((resolve) => setTimeout(resolve, 5000));\n");
		const outcome = await runSync(fakeExec(fakePt), root, 100);
		assert.equal(outcome.ok, false);
		assert.ok(outcome.report.includes("100 ms"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("readItemRoot defaults to .papertrail without config.toml", () => {
	const root = tempRoot(true);
	try {
		rmSync(join(root, ".papertrail", "config.toml"));
		assert.equal(readItemRoot(root), join(root, ".papertrail"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("readItemRoot honors a single-quoted root value", () => {
	const root = tempRoot(true);
	try {
		writeFileSync(join(root, ".papertrail", "config.toml"), "root = 'items/'\n");
		assert.equal(readItemRoot(root), join(root, "items"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("findPapertrailRoot ignores a .papertrail directory without config.toml", () => {
	const root = tempRoot(true);
	try {
		rmSync(join(root, ".papertrail", "config.toml"));
		assert.equal(findPapertrailRoot(root), null);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("readItemRoot honors a custom root key and ignores a blank one", () => {
	const root = tempRoot(true);
	try {
		writeFileSync(join(root, ".papertrail", "config.toml"), 'root = "items/"\n');
		assert.equal(readItemRoot(root), join(root, "items"));
		writeFileSync(join(root, ".papertrail", "config.toml"), 'root = ""\n');
		assert.equal(readItemRoot(root), join(root, ".papertrail"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("readHealth returns file content, or null when missing", () => {
	const root = tempRoot(true);
	try {
		const itemRoot = join(root, ".papertrail");
		assert.equal(readHealth(itemRoot), null);
		writeFileSync(join(itemRoot, "HEALTH.md"), "no findings\n");
		assert.equal(readHealth(itemRoot), "no findings\n");
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("buildPromptBlock points agents at pt help, valid values, the workspace tools, and FRONTIER.md, and carries HEALTH findings", () => {
	const root = tempRoot(true);
	try {
		const itemRoot = join(root, ".papertrail");
		const block = buildPromptBlock(itemRoot, "a.md: bad frontmatter\n");
		assert.match(block, /## Papertrail/);
		assert.ok(block.includes("FRONTIER.md"));
		assert.ok(block.includes("pt --help"));
		assert.ok(block.includes("pt help <command>"));
		assert.ok(block.includes("bug, chore, debate, feature, feedback, milestone, question, research, spike, task"));
		assert.ok(block.includes("open, in_progress, review, blocked, closed"));
		assert.ok(block.includes("Priority: 0-4"));
		assert.ok(block.includes("auto|hitl"));
		assert.ok(block.includes("pt_section_set"));
		assert.ok(block.includes("pt_batch"));
		assert.ok(block.includes("pt sync"));
		assert.ok(block.includes("a.md: bad frontmatter"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("buildPromptBlock states unreadable or empty HEALTH.md instead of inserting nothing", () => {
	const root = tempRoot(true);
	try {
		const itemRoot = join(root, ".papertrail");
		assert.ok(buildPromptBlock(itemRoot, null).includes("HEALTH.md could not be read."));
		assert.ok(buildPromptBlock(itemRoot, "\n").includes("No findings."));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("buildPromptBlock caps HEALTH.md at 50 lines with a truncation notice", () => {
	const root = tempRoot(true);
	try {
		const itemRoot = join(root, ".papertrail");
		const short = buildPromptBlock(itemRoot, Array.from({ length: 3 }, (_, i) => `line ${i}`).join("\n"));
		assert.ok(!short.includes("truncated"));
		const long = buildPromptBlock(itemRoot, Array.from({ length: 51 }, (_, i) => `line ${i}`).join("\n"));
		assert.ok(long.includes("line 0"));
		assert.ok(long.includes("1 more lines truncated"));
		assert.ok(!long.includes("line 50"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("session_start runs pt sync and notifies on failure only when UI is available", async () => {
	const root = tempRoot(true);
	try {
		const good = installExtension(execResult({ stdout: "synced" }));
		await good.handlers.get("session_start")!({}, { cwd: root, hasUI: true, ui: { notify: good.notify } });
		assert.deepEqual(good.notifications, []);

		const failing = installExtension(execResult({ code: 1, stderr: "bad" }));
		await failing.handlers.get("session_start")!({}, { cwd: root, hasUI: true, ui: { notify: failing.notify } });
		assert.equal(failing.notifications.length, 1);
		assert.ok(failing.notifications[0]![0].startsWith("papertrail: pt sync exited 1"));
		assert.equal(failing.notifications[0]![1], "warning");

		const headless = installExtension(execResult({ code: 1, stderr: "bad" }));
		await headless.handlers.get("session_start")!({}, { cwd: root, hasUI: false, ui: { notify: headless.notify } });
		assert.deepEqual(headless.notifications, []);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("session_start is a no-op outside a papertrail workspace", async () => {
	const outside = tempRoot(false);
	try {
		const { handlers, notifications, notify } = installExtension(execResult({ code: 1, stderr: "bad" }));
		await handlers.get("session_start")!({}, { cwd: outside, hasUI: true, ui: { notify } });
		assert.deepEqual(notifications, []);
	} finally {
		rmSync(outside, { recursive: true, force: true });
	}
});

test("active-tool gating activates the workspace tools on session_start inside a workspace", async () => {
	const root = tempRoot(true);
	try {
		const { handlers, activeTools } = installExtension(execResult({ stdout: "synced" }));
		await handlers.get("session_start")!({}, { cwd: root, hasUI: false, ui: { notify: () => {} } });
		assert.ok(activeTools.includes("pt_section_set"));
		assert.ok(activeTools.includes("pt_batch"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("active-tool gating removes the workspace tools outside a workspace and keeps the setup tools", async () => {
	const outside = tempRoot(false);
	try {
		const { handlers, activeTools } = installExtension(execResult({ stdout: "synced" }));
		activeTools.push("pt_section_set", "pt_batch", "papertrail_init");
		await handlers.get("session_start")!({}, { cwd: outside, hasUI: false, ui: { notify: () => {} } });
		assert.ok(!activeTools.includes("pt_section_set"));
		assert.ok(!activeTools.includes("pt_batch"));
		assert.ok(activeTools.includes("papertrail_init"));
	} finally {
		rmSync(outside, { recursive: true, force: true });
	}
});

test("active-tool gating leaves the loadout unset when the workspace tools are already correct", async () => {
	const root = tempRoot(true);
	try {
		const { handlers, activeTools } = installExtension(execResult({ stdout: "synced" }));
		activeTools.push("papertrail_init", "pt_section_set", "pt_batch");
		const before = [...activeTools];
		await handlers.get("session_start")!({}, { cwd: root, hasUI: false, ui: { notify: () => {} } });
		assert.deepEqual(activeTools, before);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("active-tool gating activates the workspace tools on before_agent_start inside a workspace", async () => {
	const root = tempRoot(true);
	try {
		const { handlers, activeTools } = installExtension();
		await handlers.get("before_agent_start")!({ systemPrompt: "base" }, { cwd: root, hasUI: false });
		assert.ok(activeTools.includes("pt_section_set"));
		assert.ok(activeTools.includes("pt_batch"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("before_agent_start appends the prompt block and honors a custom item root", async () => {
	const root = tempRoot(true);
	try {
		mkdirSync(join(root, "items"));
		writeFileSync(join(root, ".papertrail", "config.toml"), 'root = "items/"\n');
		writeFileSync(join(root, "items", "HEALTH.md"), "custom health\n");
		const { handlers } = installExtension();
		const result = (await handlers.get("before_agent_start")!({ systemPrompt: "base" }, { cwd: root, hasUI: false })) as {
			systemPrompt: string;
		};
		assert.ok(result.systemPrompt.startsWith("base"));
		assert.ok(result.systemPrompt.includes("## Papertrail"));
		assert.ok(result.systemPrompt.includes(join(root, "items", "FRONTIER.md")));
		assert.ok(result.systemPrompt.includes(join(root, "items", "HEALTH.md")));
		assert.ok(result.systemPrompt.includes("custom health"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("before_agent_start leaves the prompt untouched outside a papertrail workspace", async () => {
	const outside = tempRoot(false);
	try {
		const { handlers } = installExtension();
		const result = await handlers.get("before_agent_start")!({ systemPrompt: "base" }, { cwd: outside, hasUI: false });
		assert.equal(result, undefined);
	} finally {
		rmSync(outside, { recursive: true, force: true });
	}
});