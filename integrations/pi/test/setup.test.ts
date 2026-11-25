import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import type { AutocompleteItem } from "@earendil-works/pi-tui";
import type { Exec, ExecResult } from "../src/session.ts";
import { runInstall, installInvocation, installFlagArgs, scriptUrl, type FetchText } from "../src/install.ts";
import { runInit } from "../src/init.ts";
import papertrailExtension from "../index.ts";

type ExecCall = { command: string; args: string[] };
type InstallOutcome = { ok: boolean; report: string };

function okResult(out: Partial<ExecResult> = {}): ExecResult {
	return { stdout: "", stderr: "", code: 0, killed: false, ...out };
}

const urlFlag = process.platform === "win32" ? "-Url" : "--url";
const fakeFetch: FetchText = async () => "#!/usr/bin/env bash\necho ok\n";

interface CommandCtx {
	cwd: string;
	ui: { notify: (m: string, l: string) => void };
}

interface CapturedCommand {
	getArgumentCompletions?: (prefix: string) => AutocompleteItem[] | null;
	handler: (args: string, ctx: CommandCtx) => Promise<void>;
}

// Drive the extension factory and capture what it registers, mirroring the fake pi in extension.test.ts but for commands and tools.
function installExtension(execImpl?: Exec) {
	const commands = new Map<string, CapturedCommand>();
	const tools: string[] = [];
	const notifications: string[] = [];
	const execCalls: ExecCall[] = [];
	const activeTools: string[] = [];
	const exec: Exec = async (command, args, options) => {
		execCalls.push({ command, args });
		return execImpl ? execImpl(command, args, options) : okResult();
	};
	const notify = (message: string, level: string) => notifications.push(`${level}:${message}`);
	const pi = {
		on: () => {},
		exec,
		ui: { notify },
		registerCommand: (name: string, opts: CapturedCommand) => commands.set(name, opts),
		registerTool: (tool: { name: string }) => tools.push(tool.name),
		getActiveTools: () => [...activeTools],
		setActiveTools: (names: string[]) => {
			activeTools.length = 0;
			activeTools.push(...names);
		},
	};
	papertrailExtension(pi as unknown as Parameters<typeof papertrailExtension>[0], fakeFetch);
	const commandCtx = (cwd = "/cwd"): CommandCtx => ({ cwd, ui: { notify } });
	return { commands, tools, notifications, execCalls, commandCtx, activeTools };
}

test("scriptUrl selects install.ps1 on Windows and install.sh elsewhere", () => {
	assert.equal(scriptUrl("win32"), "https://github.com/grainologic/papertrail/releases/latest/download/install.ps1");
	assert.equal(scriptUrl("linux"), "https://github.com/grainologic/papertrail/releases/latest/download/install.sh");
});

test("installInvocation selects PowerShell on Windows and bash elsewhere for a given script path", () => {
	const win = installInvocation("C:\\tmp\\install.ps1", "win32");
	assert.ok(win.command === "pwsh" || win.command.endsWith("pwsh.exe") || win.command === "powershell");
	assert.equal(win.args[win.args.length - 1], "C:\\tmp\\install.ps1");
	const fallback = installInvocation("C:\\tmp\\install.ps1", "win32", "powershell");
	assert.equal(fallback.command, "powershell");
	assert.equal(fallback.args[fallback.args.length - 1], "C:\\tmp\\install.ps1");
	const unix = installInvocation("/tmp/install.sh", "linux");
	assert.equal(unix.command, "bash");
	assert.deepEqual(unix.args, ["/tmp/install.sh"]);
});

test("installFlagArgs maps params to the platform dialect and omits absent params", () => {
	assert.deepEqual(installFlagArgs({ url: "https://example.com/pt", tag: "v1", dir: "/tmp/x" }, "linux"), ["--url", "https://example.com/pt", "--tag", "v1", "--dir", "/tmp/x"]);
	assert.deepEqual(installFlagArgs({ url: "https://example.com/pt" }, "win32"), ["-Url", "https://example.com/pt"]);
	assert.deepEqual(installFlagArgs({}, "linux"), []);
});

test("runInstall fetches the remote script into a temp file, passes flag args, and cleans up", async () => {
	const calls: ExecCall[] = [];
	const fetchedUrls: string[] = [];
	let scriptDuringExec = "";
	let seenBody = "";
	const fetchText: FetchText = async (url) => {
		fetchedUrls.push(url);
		return "echo fetched-script\n";
	};
	const exec: Exec = async (command, args) => {
		calls.push({ command, args });
		const fileArg = args.find((a) => a.endsWith("install.ps1") || a.endsWith("install.sh"))!;
		scriptDuringExec = fileArg;
		seenBody = readFileSync(fileArg, "utf8");
		return okResult({ stdout: "installed pt v1.0.0" });
	};
	const outcome = await runInstall(exec, { url: "https://github.com/grainologic/papertrail", tag: "v1.0.0" }, "/cwd", undefined, fetchText);
	assert.equal(outcome.ok, true);
	assert.equal(outcome.report, "installed pt v1.0.0");
	assert.deepEqual(fetchedUrls, [scriptUrl()]);
	assert.equal(seenBody, "echo fetched-script\n");
	assert.equal(existsSync(scriptDuringExec), false);
	assert.equal(calls.length, 1);
	const call = calls[0]!;
	assert.equal(call.command, installInvocation(scriptDuringExec).command);
	assert.ok(call.args.includes(urlFlag));
	assert.ok(call.args.includes("https://github.com/grainologic/papertrail"));
	assert.ok(call.args.includes("v1.0.0"));
});

test("runInstall never throws when fetching the install script fails", async () => {
	const failingFetch: FetchText = async () => {
		throw new Error("HTTP 404 Not Found");
	};
	const outcome = await runInstall(async () => okResult(), {}, "/cwd", undefined, failingFetch);
	assert.equal(outcome.ok, false);
	assert.ok(outcome.report.includes("could not fetch"));
	assert.ok(outcome.report.includes("HTTP 404 Not Found"));
});

test("runInstall adds an absolute installed binary directory to process.env.PATH once", async () => {
	const origPath = process.env.PATH;
	const binDir = process.platform === "win32" ? "C:\\Users\\test\\AppData\\Local\\papertrail\\bin" : "/tmp/pt-install-test-bin";
	const binPath = process.platform === "win32" ? `${binDir}\\pt.exe` : `${binDir}/pt`;
	const exec: Exec = async () => okResult({ stdout: `installed ${binPath}\nadded ${binDir} to PATH` });
	try {
		const first = await runInstall(exec, {}, "/cwd", undefined, fakeFetch);
		assert.equal(first.ok, true);
		const entriesAfterFirst = (process.env.PATH ?? "").split(process.platform === "win32" ? ";" : ":").filter((e) => e === binDir);
		assert.equal(entriesAfterFirst.length, 1);

		await runInstall(exec, {}, "/cwd", undefined, fakeFetch);
		const entriesAfterSecond = (process.env.PATH ?? "").split(process.platform === "win32" ? ";" : ":").filter((e) => e === binDir);
		assert.equal(entriesAfterSecond.length, 1);
	} finally {
		if (origPath === undefined) delete process.env.PATH;
		else process.env.PATH = origPath;
	}
});

test("runInstall never throws: a nonzero exit keeps stderr visible even after multi-line stdout", async () => {
	const plan = ["url: https://example.com", "tag: latest", "os/arch: windows/amd64", "asset: pt.zip", "download: https://example.com/pt.zip", "checksum: https://example.com/c.txt", "dest: C:\\pt\\pt.exe"].join("\n");
	const outcome = await runInstall(async () => okResult({ code: 1, stdout: plan, stderr: "Get-FileHash : missing cmdlet" }), {}, "/cwd", undefined, fakeFetch);
	assert.equal(outcome.ok, false);
	assert.ok(outcome.report.includes("exited 1"));
	assert.ok(outcome.report.includes("Get-FileHash : missing cmdlet"));
});

test("runInit runs `pt init` when pt is installed, without installing", async () => {
	const calls: string[] = [];
	const exec: Exec = async (command, args) => {
		calls.push(`${command} ${args.join(" ")}`);
		return command === "pt" && args[0] === "--version" ? okResult() : okResult({ stdout: "initialized work item" });
	};
	const install = async (): Promise<InstallOutcome> => {
		throw new Error("install must not run when pt is present");
	};
	const outcome = await runInit(exec, "/cwd", install);
	assert.equal(outcome.ok, true);
	assert.equal(outcome.installed, false);
	assert.deepEqual(calls, ["pt --version", "pt init"]);
});

test("runInit installs first when pt is missing, then runs `pt init`", async () => {
	for (const mode of ["reject", "nonzero"] as const) {
		const calls: string[] = [];
		let installedCalls = 0;
		const exec: Exec = async (command, args) => {
			calls.push(`${command} ${args.join(" ")}`);
			if (command === "pt" && args[0] === "--version") {
				if (mode === "reject") throw new Error("spawn pt ENOENT");
				return okResult({ code: 1, stderr: "spawn pt ENOENT" });
			}
			return okResult({ stdout: "initialized" });
		};
		const install = async (): Promise<InstallOutcome> => {
			installedCalls++;
			return { ok: true, report: "installed pt" };
		};
		const outcome = await runInit(exec, "/cwd", install);
		assert.equal(outcome.ok, true);
		assert.equal(outcome.installed, true);
		assert.equal(installedCalls, 1);
		assert.deepEqual(calls, ["pt --version", "pt init"]);
	}
});

test("runInit reports a failed install and skips `pt init`", async () => {
	const calls: string[] = [];
	const exec: Exec = async (command, args) => {
		calls.push(`${command} ${args.join(" ")}`);
		throw new Error("spawn pt ENOENT");
	};
	const outcome = await runInit(exec, "/cwd", async () => ({ ok: false, report: "download failed: 403" }));
	assert.equal(outcome.ok, false);
	assert.equal(outcome.installed, false);
	assert.ok(outcome.report.includes("pt was not installed"));
	assert.ok(outcome.report.includes("download failed: 403"));
	assert.deepEqual(calls, ["pt --version"]);
});

test("runInit never throws: a nonzero `pt init` exit becomes a failed outcome", async () => {
	const exec: Exec = async (command, args) =>
		command === "pt" && args[0] === "--version" ? okResult() : okResult({ code: 1, stderr: "pt: not a papertrail workspace" });
	const install = async (): Promise<InstallOutcome> => {
		throw new Error("install must not run");
	};
	const outcome = await runInit(exec, "/cwd", install);
	assert.equal(outcome.ok, false);
	assert.ok(outcome.report.includes("pt init exited 1"));
});

test("extension registers the papertrail command and all four tools", () => {
	const { commands, tools } = installExtension();
	assert.ok(commands.has("papertrail"));
	assert.deepEqual(tools, ["papertrail_install", "papertrail_init", "pt_section_set", "pt_batch"]);
});

test("getArgumentCompletions filters subcommands by prefix and returns null on no match", () => {
	const { commands } = installExtension();
	const completions = commands.get("papertrail")!.getArgumentCompletions!;
	assert.deepEqual(completions("i"), [
		{ value: "init", label: "init" },
		{ value: "install", label: "install" },
	]);
	assert.deepEqual(completions("ins"), [{ value: "install", label: "install" }]);
	assert.deepEqual(completions(""), [
		{ value: "init", label: "init" },
		{ value: "install", label: "install" },
	]);
	assert.equal(completions("z"), null);
});

test("the install subcommand parses flags and notifies the result", async () => {
	const { commands, notifications, execCalls, commandCtx } = installExtension(async (_command, _args, _opts) => okResult({ stdout: "ran" }));
	await commands.get("papertrail")!.handler("install --owner nope --url https://example.com/pt --dir /x", commandCtx());
	const call = execCalls[0]!;
	assert.ok(call.args.includes(urlFlag));
	assert.ok(call.args.includes("https://example.com/pt"));
	assert.equal(notifications.length, 1);
	assert.ok(notifications[0]!.startsWith("info:"));
});

test("an unknown or empty subcommand shows usage", async () => {
	const { commands, notifications, commandCtx } = installExtension();
	await commands.get("papertrail")!.handler("bogus", commandCtx());
	assert.ok(notifications[0]!.includes("usage:"));
	await commands.get("papertrail")!.handler("", commandCtx());
	assert.ok(notifications[1]!.includes("usage:"));
});

test("active-tool gating re-activates the workspace tools after a successful init", async () => {
	const { commands, activeTools, commandCtx } = installExtension(async (command, args) =>
		command === "pt" && args[0] === "--version" ? okResult() : okResult({ stdout: "initialized" })
	);
	await commands.get("papertrail")!.handler("init", commandCtx());
	assert.ok(activeTools.includes("pt_section_set"));
	assert.ok(activeTools.includes("pt_batch"));
});

test("active-tool gating does not activate the workspace tools after a failed init", async () => {
	const { commands, activeTools, commandCtx } = installExtension(async (command, args) =>
		command === "pt" && args[0] === "--version" ? okResult() : okResult({ code: 1, stderr: "boom" })
	);
	await commands.get("papertrail")!.handler("init", commandCtx());
	assert.ok(!activeTools.includes("pt_section_set"));
	assert.ok(!activeTools.includes("pt_batch"));
});